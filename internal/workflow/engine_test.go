package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

type fakeBrowser struct {
	mu             sync.Mutex
	calls          []string
	active         int
	overrideSource string
	lastActivity   time.Time
}

func (f *fakeBrowser) snapshot() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, "|")
}

func (f *fakeBrowser) OverrideSource() (bool, string) {
	return f.overrideSource != "", f.overrideSource
}

func (f *fakeBrowser) LastActivity() time.Time { return f.lastActivity }

func (f *fakeBrowser) log(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

func (f *fakeBrowser) SetOverride(_ context.Context, page int, d time.Duration, source string) error {
	f.log("override %d %s %s", page, d, source)
	return nil
}
func (f *fakeBrowser) ClearOverride() error         { f.log("resume"); return nil }
func (f *fakeBrowser) Reload(context.Context) error { f.log("reload"); return nil }
func (f *fakeBrowser) Evaluate(_ context.Context, s string) error {
	f.log("script %s", s)
	return errors.New("boom")
}
func (f *fakeBrowser) ShowOverlay(_ context.Context, text string, d time.Duration) error {
	f.log("overlay %s %s", text, d)
	return nil
}
func (f *fakeBrowser) ActivePage() int           { return f.active }
func (f *fakeBrowser) NoteDisplayPower(p string) { f.log("note %s", p) }

type fakeHardware struct{ browser *fakeBrowser }

func (h fakeHardware) SetDisplay(_ context.Context, power string) error {
	h.browser.log("display %s", power)
	return nil
}
func (h fakeHardware) SetBrightness(_ context.Context, value int) error {
	h.browser.log("brightness %d", value)
	return nil
}

func testConfig(rules ...config.WorkflowRule) *config.Config {
	return &config.Config{Kiosk: config.KioskConfig{
		Pages: []config.KioskPage{
			{PageID: "home", Name: "Home", URL: "http://ha/home"},
			{PageID: "off", Name: "Off", URL: "http://ha/off", Disabled: true},
			{PageID: "camera", Name: "Camera", URL: "http://ha/camera"},
		},
		Rules: rules,
	}}
}

func doorbellRule() config.WorkflowRule {
	return config.WorkflowRule{
		ID: "doorbell", Name: "Doorbell",
		Trigger: config.RuleTrigger{Type: "mqtt", Topic: "home/doorbell", Payload: "ON"},
		Actions: []config.RuleAction{
			{Type: "display", Value: "on"},
			{Type: "show_page", PageID: "camera", DurationSeconds: 30},
			{Type: "overlay", Value: "Someone is at the door", DurationSeconds: 5},
		},
		CooldownSeconds: 60,
	}
}

func TestMQTTRuleRunsActionsInOrder(t *testing.T) {
	browser := &fakeBrowser{}
	engine := New(testConfig(doorbellRule()), browser, fakeHardware{browser}, nil, nil)
	if !engine.HandleMQTT(context.Background(), "home/doorbell", "on") {
		t.Fatal("rule topic not claimed")
	}
	want := "display ON|note ON|override 1 30s workflow|overlay Someone is at the door 5s"
	if got := strings.Join(browser.calls, "|"); got != want {
		t.Fatalf("calls = %s\nwant  = %s", got, want)
	}
	if engine.HandleMQTT(context.Background(), "home/other", "ON") {
		t.Fatal("unrelated topic claimed")
	}
}

func TestPayloadMismatchAndCooldown(t *testing.T) {
	browser := &fakeBrowser{}
	engine := New(testConfig(doorbellRule()), browser, fakeHardware{browser}, nil, nil)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return now }
	engine.HandleMQTT(context.Background(), "home/doorbell", "OFF")
	if len(browser.calls) != 0 {
		t.Fatalf("payload mismatch ran actions: %v", browser.calls)
	}
	engine.HandleMQTT(context.Background(), "home/doorbell", "ON")
	engine.HandleMQTT(context.Background(), "home/doorbell", "ON")
	if count := len(browser.calls); count != 4 {
		t.Fatalf("cooldown did not suppress the second run: %v", browser.calls)
	}
	history := engine.History()
	if len(history) != 2 || history[0].Skipped != "cooldown" {
		t.Fatalf("history = %#v", history)
	}
}

func TestConditionsGateRules(t *testing.T) {
	rule := doorbellRule()
	rule.CooldownSeconds = 0
	rule.Conditions = config.RuleConditions{Start: "22:00", End: "06:00", ActivePageID: "home"}
	browser := &fakeBrowser{active: 0}
	engine := New(testConfig(rule), browser, fakeHardware{browser}, nil, nil)
	engine.now = func() time.Time { return time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC) }
	engine.HandleMQTT(context.Background(), "home/doorbell", "ON")
	if len(browser.calls) != 0 || engine.History()[0].Skipped != "time_window" {
		t.Fatalf("time window not enforced: %v", engine.History())
	}
	engine.now = func() time.Time { return time.Date(2026, 10, 12, 23, 30, 0, 0, time.UTC) }
	browser.active = 1
	engine.HandleMQTT(context.Background(), "home/doorbell", "ON")
	if engine.History()[0].Skipped != "active_page" {
		t.Fatalf("active page condition not enforced: %v", engine.History())
	}
	browser.active = 0
	engine.HandleMQTT(context.Background(), "home/doorbell", "ON")
	if len(browser.calls) == 0 {
		t.Fatal("rule did not run inside the window on the right page")
	}
}

func TestTimeTriggerFiresOncePerMinute(t *testing.T) {
	rule := config.WorkflowRule{ID: "night", Name: "Night", Trigger: config.RuleTrigger{Type: "time", At: "22:30"}, Actions: []config.RuleAction{{Type: "display", Value: "OFF"}}}
	browser := &fakeBrowser{}
	engine := New(testConfig(rule), browser, fakeHardware{browser}, nil, nil)
	engine.now = func() time.Time { return time.Date(2026, 10, 12, 22, 30, 5, 0, time.UTC) }
	engine.checkTime(context.Background())
	engine.checkTime(context.Background())
	if got := strings.Join(browser.calls, "|"); got != "display OFF|note OFF" {
		t.Fatalf("calls = %s", got)
	}
}

func TestWebhookRequiresToken(t *testing.T) {
	rule := config.WorkflowRule{ID: "hook", Name: "Hook", Trigger: config.RuleTrigger{Type: "webhook", Token: "0123456789abcdef"}, Actions: []config.RuleAction{{Type: "reload"}}}
	browser := &fakeBrowser{}
	engine := New(testConfig(rule), browser, fakeHardware{browser}, nil, nil)
	if err := engine.HandleWebhook(context.Background(), "hook", "wrong"); !errors.Is(err, ErrUnknownRule) {
		t.Fatalf("wrong token err = %v", err)
	}
	if err := engine.HandleWebhook(context.Background(), "hook", "0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	if len(browser.calls) != 1 || browser.calls[0] != "reload" {
		t.Fatalf("calls = %v", browser.calls)
	}
}

func TestFailedActionIsReportedButLaterActionsRun(t *testing.T) {
	rule := config.WorkflowRule{ID: "s", Name: "Script", Trigger: config.RuleTrigger{Type: "webhook", Token: "0123456789abcdef"}, Actions: []config.RuleAction{{Type: "script", Value: "x()"}, {Type: "reload"}}}
	browser := &fakeBrowser{}
	engine := New(testConfig(rule), browser, fakeHardware{browser}, nil, nil)
	if err := engine.Test(context.Background(), "s"); err == nil || !strings.Contains(err.Error(), "1 script: boom") {
		t.Fatalf("err = %v", err)
	}
	if browser.calls[len(browser.calls)-1] != "reload" {
		t.Fatalf("later action skipped: %v", browser.calls)
	}
}

func TestManualOverrideWinsOverRules(t *testing.T) {
	rule := config.WorkflowRule{ID: "r", Name: "R", Trigger: config.RuleTrigger{Type: "webhook", Token: "0123456789abcdef"}, Actions: []config.RuleAction{{Type: "show_page", PageID: "camera", UntilResume: true}, {Type: "resume_playlist"}, {Type: "reload"}}}
	browser := &fakeBrowser{overrideSource: "admin"}
	engine := New(testConfig(rule), browser, fakeHardware{browser}, nil, nil)
	if err := engine.Test(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	if got := browser.snapshot(); got != "reload" {
		t.Fatalf("calls = %s, want only reload", got)
	}
	if engine.History()[0].Skipped != "manual_override" {
		t.Fatalf("history = %#v", engine.History())
	}
	browser.calls, browser.overrideSource = nil, "workflow"
	if err := engine.Test(context.Background(), "r"); err != nil {
		t.Fatal(err)
	}
	if got := browser.snapshot(); got != "override 1 24h0m0s workflow|resume|reload" {
		t.Fatalf("calls = %s", got)
	}
}

func TestIdleAndTouchTriggers(t *testing.T) {
	idle := config.WorkflowRule{ID: "idle", Name: "Idle", Trigger: config.RuleTrigger{Type: "idle", IdleSeconds: 60}, Actions: []config.RuleAction{{Type: "display", Value: "OFF"}}}
	touch := config.WorkflowRule{ID: "touch", Name: "Touch", Trigger: config.RuleTrigger{Type: "touch"}, Actions: []config.RuleAction{{Type: "display", Value: "ON"}}}
	browser := &fakeBrowser{}
	engine := New(testConfig(idle, touch), browser, fakeHardware{browser}, nil, nil)
	start := engine.started
	engine.now = func() time.Time { return start.Add(30 * time.Second) }
	engine.checkIdle(context.Background())
	if browser.snapshot() != "" {
		t.Fatalf("idle fired early: %s", browser.snapshot())
	}
	engine.now = func() time.Time { return start.Add(61 * time.Second) }
	engine.checkIdle(context.Background())
	engine.checkIdle(context.Background())
	if got := browser.snapshot(); got != "display OFF|note OFF" {
		t.Fatalf("idle calls = %s", got)
	}
	browser.lastActivity = start.Add(70 * time.Second)
	engine.HandleActivity(context.Background())
	if got := browser.snapshot(); got != "display OFF|note OFF|display ON|note ON" {
		t.Fatalf("touch calls = %s", got)
	}
	engine.now = func() time.Time { return start.Add(131 * time.Second) }
	engine.checkIdle(context.Background())
	if got := browser.snapshot(); !strings.HasSuffix(got, "display ON|note ON|display OFF|note OFF") {
		t.Fatalf("idle not re-armed: %s", got)
	}
}

func TestDisplayActionRevertsAfterDuration(t *testing.T) {
	rule := config.WorkflowRule{ID: "m", Name: "Motion", Trigger: config.RuleTrigger{Type: "touch"}, Actions: []config.RuleAction{{Type: "display", Value: "ON", DurationSeconds: 1}}}
	browser := &fakeBrowser{}
	engine := New(testConfig(rule), browser, fakeHardware{browser}, nil, nil)
	if err := engine.Test(context.Background(), "m"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(browser.snapshot(), "display OFF") {
		time.Sleep(20 * time.Millisecond)
	}
	if got := browser.snapshot(); got != "display ON|note ON|display OFF|note OFF" {
		t.Fatalf("calls = %s", got)
	}
}

func TestPageScriptsEncodeValues(t *testing.T) {
	if script := clickScript(`button[title="Go"]`); !strings.Contains(script, `"button[title=\"Go\"]"`) || !strings.Contains(script, "shadowRoot") {
		t.Fatalf("click script = %s", script)
	}
	if script := scrollScript("-400"); !strings.Contains(script, "top: -400") {
		t.Fatalf("scroll script = %s", script)
	}
	if script := soundScript(""); !strings.Contains(script, "AudioContext") {
		t.Fatalf("chime script = %s", script)
	}
	if script := soundScript("https://x/a.mp3"); script != `new Audio("https://x/a.mp3").play()` {
		t.Fatalf("sound script = %s", script)
	}
}
