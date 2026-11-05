package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// revealCommander answers readiness probes with "loading" until ready calls
// have passed and records every method.
type revealCommander struct {
	mu      sync.Mutex
	methods []string
	probes  int
	ready   int
}

func (r *revealCommander) command(_ context.Context, method string, _ any, result any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.methods = append(r.methods, method)
	if method == "Runtime.evaluate" {
		r.probes++
		value := r.probes > r.ready
		data, _ := json.Marshal(map[string]any{"result": map[string]any{"value": value}})
		return json.Unmarshal(data, result)
	}
	return nil
}

func shortReveal(t *testing.T) {
	t.Helper()
	oldTimeout, oldSettle, oldPoll := revealLoadTimeout, revealSettle, revealPoll
	revealLoadTimeout, revealSettle, revealPoll = 500*time.Millisecond, time.Millisecond, time.Millisecond
	t.Cleanup(func() { revealLoadTimeout, revealSettle, revealPoll = oldTimeout, oldSettle, oldPoll })
}

func TestNewTabOpensInBackgroundAndIsRevealedAfterLoad(t *testing.T) {
	shortReveal(t)
	fake := &fakeBrowserTarget{}
	browser := tabTestBrowser(t, fake)
	if err := browser.switchTab(context.Background(), browser.cfg, 1); err != nil {
		t.Fatal(err)
	}
	if !browser.pendingReveal("tab-1") {
		t.Fatal("a new tab must wait in the background until its page loaded")
	}
	session := &revealCommander{ready: 3}
	browser.revealWhenLoaded(context.Background(), session, "tab-1")
	calls := strings.Join(session.methods, ",")
	if !strings.HasSuffix(calls, "Page.bringToFront") || strings.Count(calls, "Runtime.evaluate") != 4 {
		t.Fatalf("calls = %s", calls)
	}
	if browser.pendingReveal("tab-1") {
		t.Fatal("reveal must be cleared after the tab came to the front")
	}

	// Switching back to a kept tab activates it directly; nothing waits.
	if err := browser.switchTab(context.Background(), browser.cfg, 0); err != nil {
		t.Fatal(err)
	}
	if browser.pendingReveal("initial") || browser.pendingReveal("tab-1") {
		t.Fatal("kept tabs must not wait for a reveal")
	}
}

func TestRevealShowsSlowPageAfterTimeoutAndSkipsSupersededTab(t *testing.T) {
	shortReveal(t)
	browser := tabTestBrowser(t, &fakeBrowserTarget{})
	browser.revealTarget = "slow"
	slow := &revealCommander{ready: 1 << 30}
	browser.revealWhenLoaded(context.Background(), slow, "slow")
	if last := slow.methods[len(slow.methods)-1]; last != "Page.bringToFront" {
		t.Fatalf("slow page must still be shown after the timeout, last call %s", last)
	}

	browser.revealTarget = "newer"
	stale := &revealCommander{}
	browser.revealWhenLoaded(context.Background(), stale, "older")
	if fmt.Sprint(stale.methods) != "[]" {
		t.Fatalf("superseded tab must not be brought to front: %v", stale.methods)
	}
}

func TestDarkThemeSetsDarkDefaultBackground(t *testing.T) {
	var params map[string]any
	commander := &paramsCommander{fn: func(method string, p any) {
		if method == "Emulation.setDefaultBackgroundColorOverride" {
			params, _ = p.(map[string]any)
		}
	}}
	if err := configurePageBackground(context.Background(), commander, "light"); err != nil || params != nil {
		t.Fatal("light theme must keep Chromium's default background")
	}
	if err := configurePageBackground(context.Background(), commander, "dark"); err != nil || params == nil {
		t.Fatal("dark theme must set a dark default background")
	}
	color := params["color"].(map[string]any)
	if color["r"] != 0 || color["a"] != 1 {
		t.Fatalf("color = %v", color)
	}
}

type paramsCommander struct{ fn func(string, any) }

func (p *paramsCommander) command(_ context.Context, method string, params any, _ any) error {
	p.fn(method, params)
	return nil
}
