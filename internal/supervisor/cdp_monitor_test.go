package supervisor

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestPeekEventMethodSkipsLargeFrames(t *testing.T) {
	method, ok := peekEventMethod([]byte(`{"method":"Network.dataReceived","params":{}}`))
	if !ok || method != "Network.dataReceived" {
		t.Fatalf("peekEventMethod = %q, %v", method, ok)
	}
	if _, ok := peekEventMethod([]byte(`{"id":4,"result":{}}`)); ok {
		t.Fatal("command responses must not be treated as events")
	}
	if cdpEventWanted("Network.dataReceived", 10) {
		t.Fatal("dataReceived should be dropped before decoding")
	}
	if cdpEventWanted("Network.webSocketFrameReceived", maxAuthFrameEvent+1) {
		t.Fatal("large WebSocket frames cannot carry auth_invalid and should be dropped")
	}
	if !cdpEventWanted("Network.webSocketFrameReceived", 120) {
		t.Fatal("small WebSocket frames must still be inspected")
	}
}

func TestMainFrameURLIgnoresIframes(t *testing.T) {
	if _, ok := mainFrameURL(json.RawMessage(`{"frame":{"parentId":"x","url":"http://cam"}}`)); ok {
		t.Fatal("iframe navigation reported as main frame")
	}
	if url, ok := mainFrameURL(json.RawMessage(`{"frame":{"url":"http://ha:8123/auth/authorize"}}`)); !ok || url == "" {
		t.Fatal("main frame navigation not detected")
	}
}

// fakeCDP answers every command with an empty result and records the
// method names in order.
type fakeCDP struct {
	mu      sync.Mutex
	methods []string
	conn    *websocket.Conn
	ready   chan struct{}
}

func (f *fakeCDP) handler(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()
	close(f.ready)
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var request struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(data, &request) != nil {
			continue
		}
		f.mu.Lock()
		f.methods = append(f.methods, request.Method)
		f.mu.Unlock()
		reply, _ := json.Marshal(map[string]any{"id": request.ID, "result": map[string]any{}})
		_ = conn.Write(r.Context(), websocket.MessageText, reply)
	}
}

func (f *fakeCDP) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.methods {
		if m == method {
			n++
		}
	}
	return n
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestNetworkMonitorWindowDisablesAndRearmsOnNavigation(t *testing.T) {
	previous := networkMonitorWindow
	networkMonitorWindow = 80 * time.Millisecond
	defer func() { networkMonitorWindow = previous }()

	fake := &fakeCDP{ready: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer server.Close()

	cfg := schedulerTestConfig()
	cfg.Path = filepath.Join(t.TempDir(), "config.json")
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	done := make(chan struct{})
	browser.done = done

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	session := newCDPSession(conn)
	defer session.close()
	navigated := true
	result := make(chan error, 1)
	go func() { result <- browser.runDevTools(ctx, session, "", done, &navigated) }()

	<-fake.ready
	waitFor(t, "Network.disable after the window", func() bool { return fake.count("Network.disable") == 1 })
	fake.mu.Lock()
	peer := fake.conn
	fake.mu.Unlock()
	_ = peer.Write(ctx, websocket.MessageText, []byte(`{"method":"Page.frameNavigated","params":{"frame":{"id":"main","url":"http://ha.local:8123/auth/authorize"}}}`))
	waitFor(t, "Network.enable after navigation", func() bool { return fake.count("Network.enable") == 2 })
	waitFor(t, "second Network.disable", func() bool { return fake.count("Network.disable") == 2 })
	cancel()
	<-result
}
