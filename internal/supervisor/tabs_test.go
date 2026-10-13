package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/coder/websocket"
)

type fakeBrowserTarget struct {
	mu      sync.Mutex
	calls   []string
	created int
}

func (f *fakeBrowserTarget) handler(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var request struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(data, &request) != nil {
			continue
		}
		result := map[string]any{}
		f.mu.Lock()
		f.calls = append(f.calls, fmt.Sprintf("%s %v", request.Method, request.Params["targetId"]))
		if request.Method == "Target.createTarget" {
			f.created++
			result["targetId"] = fmt.Sprintf("tab-%d", f.created)
		}
		f.mu.Unlock()
		reply, _ := json.Marshal(map[string]any{"id": request.ID, "result": result})
		_ = conn.Write(r.Context(), websocket.MessageText, reply)
	}
}

func (f *fakeBrowserTarget) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func tabTestBrowser(t *testing.T, fake *fakeBrowserTarget) *Browser {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(server.Close)
	parsed, _ := url.Parse(server.URL)
	profile := t.TempDir()
	if err := os.WriteFile(filepath.Join(profile, "DevToolsActivePort"), []byte(parsed.Port()+"\n/devtools/browser/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := schedulerTestConfig()
	cfg.Path = filepath.Join(t.TempDir(), "config.json")
	cfg.Kiosk.UserDataDir = profile
	cfg.Kiosk.PageSwitch = "tabs"
	cfg.Kiosk.MaxTabs = 2
	cfg.Kiosk.Pages = append(cfg.Kiosk.Pages, cfg.Kiosk.Pages[0])
	cfg.Kiosk.Pages[2].URL = "http://ha.local/third"
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start helper process")
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	browser.cmd = cmd
	browser.resetTabsLocked()
	if browser.bindControlTarget("initial") {
		t.Fatal("initial launch target must not be treated as a new blank tab")
	}
	return browser
}

func TestTabModeCreatesActivatesAndEvicts(t *testing.T) {
	fake := &fakeBrowserTarget{}
	browser := tabTestBrowser(t, fake)
	cfg := browser.cfg
	ctx := context.Background()

	if err := browser.switchTab(ctx, cfg, 1); err != nil {
		t.Fatal(err)
	}
	if browser.activeTarget != "tab-1" || !browser.consumeRetarget() {
		t.Fatalf("active target = %q", browser.activeTarget)
	}
	if !browser.bindControlTarget("tab-1") {
		t.Fatal("new tab should be navigated after attach")
	}

	if err := browser.switchTab(ctx, cfg, 0); err != nil {
		t.Fatal(err)
	}
	if browser.activeTarget != "initial" || browser.bindControlTarget("initial") {
		t.Fatal("switching back must reuse the initial tab without navigation")
	}

	// Third page exceeds max_tabs=2 and evicts the least recently used tab (page 1).
	if err := browser.switchTab(ctx, cfg, 2); err != nil {
		t.Fatal(err)
	}
	calls := fake.callList()
	want := []string{"Target.createTarget <nil>", "Target.activateTarget initial", "Target.createTarget <nil>", "Target.closeTarget tab-1"}
	if fmt.Sprint(calls) != fmt.Sprint(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if _, ok := browser.tabs[1]; ok {
		t.Fatal("evicted tab still tracked")
	}
}

func TestTabModeRenavigatesEditedPage(t *testing.T) {
	fake := &fakeBrowserTarget{}
	browser := tabTestBrowser(t, fake)
	ctx := context.Background()
	if err := browser.switchTab(ctx, browser.cfg, 1); err != nil {
		t.Fatal(err)
	}
	browser.bindControlTarget("tab-1")
	browser.cfg.Kiosk.Pages[0].URL = "http://ha.local/edited"
	if err := browser.switchTab(ctx, browser.cfg, 0); err != nil {
		t.Fatal(err)
	}
	if !browser.bindControlTarget("initial") {
		t.Fatal("tab with an edited URL should be navigated again")
	}
}

func TestPickDevToolsTargetPrefersActiveTab(t *testing.T) {
	targets := []devToolsTarget{
		{ID: "a", Type: "page", WebSocketDebuggerURL: "ws://a"},
		{ID: "b", Type: "page", WebSocketDebuggerURL: "ws://b"},
	}
	if target, _ := pickDevToolsTarget(targets, "b"); target.ID != "b" {
		t.Fatalf("picked %q", target.ID)
	}
	if target, _ := pickDevToolsTarget(targets, "gone"); target.ID != "a" {
		t.Fatalf("fallback picked %q", target.ID)
	}
}
