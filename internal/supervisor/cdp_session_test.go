package supervisor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestCDPInterleavedEventAndReversedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		requests := make([]struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}, 2)
		for i := range requests {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if json.Unmarshal(data, &requests[i]) != nil {
				return
			}
		}
		_ = conn.Write(ctx, websocket.MessageText, []byte(`{"method":"Network.webSocketFrameReceived","params":{"response":{"payloadData":"{\"type\":\"auth_invalid\"}"}}}`))
		for i := len(requests) - 1; i >= 0; i-- {
			data, _ := json.Marshal(map[string]any{"id": requests[i].ID, "result": map[string]string{"method": requests[i].Method}})
			_ = conn.Write(ctx, websocket.MessageText, data)
		}
		_, _, _ = conn.Read(ctx)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	session := newCDPSession(conn)
	defer session.close()
	var group sync.WaitGroup
	for _, method := range []string{"Page.enable", "Network.enable"} {
		group.Go(func() {
			var result struct {
				Method string `json:"method"`
			}
			if err := session.command(ctx, method, nil, &result); err != nil {
				t.Error(err)
			} else if result.Method != method {
				t.Errorf("response crossed: %s != %s", result.Method, method)
			}
		})
	}
	group.Wait()
	select {
	case event := <-session.events:
		if event.Method != "Network.webSocketFrameReceived" {
			t.Fatal(event.Method)
		}
	case <-ctx.Done():
		t.Fatal("auth event was lost while commands were pending")
	}
}

func TestHAClassificationIgnoresResourceFailures(t *testing.T) {
	for _, resource := range []string{"Image", "Fetch", "XHR", "Media", "Font"} {
		if result := classifyHAResponse(403, resource, "http://ha.local:8123/api/camera_proxy/camera.front?token=bad"); result.Kind != "" {
			t.Fatalf("media failure tripped guard: %+v", result)
		}
	}
	if result := classifyHAResponse(400, "Fetch", "https://ha.example/auth/token"); result.Kind != "token_response" {
		t.Fatal(result)
	}
	if parseAuthFailure(`{"data":{"type":"auth_invalid"}}`) {
		t.Fatal("nested text is not an authentication failure")
	}
	if !parseAuthFailure("{\"type\":\t\"auth_invalid\"}") {
		t.Fatal("valid JSON whitespace was rejected")
	}
}

func TestCDPOnlyQueuesLoadingFinishedForTokenResponse(t *testing.T) {
	session := &cdpSession{bodyIDs: make(map[string]struct{})}
	ordinary := cdpMessage{Method: "Network.loadingFinished", Params: json.RawMessage(`{"requestId":"asset"}`)}
	if session.interestingEvent(ordinary) {
		t.Fatal("ordinary loadingFinished event should be discarded")
	}
	token := cdpMessage{Method: "Network.responseReceived", Params: json.RawMessage(`{"requestId":"token","response":{"status":400,"url":"http://ha.local:8123/auth/token"}}`)}
	if !session.interestingEvent(token) {
		t.Fatal("token response was discarded")
	}
	finished := cdpMessage{Method: "Network.loadingFinished", Params: json.RawMessage(`{"requestId":"token"}`)}
	if !session.interestingEvent(finished) {
		t.Fatal("token loadingFinished event was discarded")
	}
	if session.interestingEvent(finished) {
		t.Fatal("completed token request remained registered")
	}
}

func TestCDPQueuesNavigationLifecycleEvents(t *testing.T) {
	session := &cdpSession{bodyIDs: make(map[string]struct{})}
	for _, method := range []string{"Page.loadEventFired", "Page.lifecycleEvent"} {
		if !session.interestingEvent(cdpMessage{Method: method}) {
			t.Fatalf("%s event was discarded", method)
		}
	}
}
