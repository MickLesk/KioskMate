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
	mu     sync.Mutex
	calls  []string
	active int
}

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
