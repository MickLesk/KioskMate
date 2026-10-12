// Package workflow runs event-driven rules on top of the kiosk playlist:
// an MQTT message, a time of day or a webhook triggers actions such as
// showing a page for a while, switching the display or showing an overlay.
package workflow

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/events"
)

// Browser is the part of the supervisor the actions drive.
type Browser interface {
	SetOverride(ctx context.Context, page int, duration time.Duration, source string) error
	ClearOverride() error
	Reload(ctx context.Context) error
	Evaluate(ctx context.Context, script string) error
	ShowOverlay(ctx context.Context, text string, duration time.Duration) error
	ActivePage() int
	NoteDisplayPower(power string)
	// OverrideSource reports an active page override and who set it.
	OverrideSource() (bool, string)
	// LastActivity is the last user input on the kiosk page, or zero.
	LastActivity() time.Time
}

// Hardware controls the physical display.
type Hardware interface {
	SetDisplay(ctx context.Context, power string) error
	SetBrightness(ctx context.Context, percent int) error
}

// Run is one rule execution, kept for the Admin UI and diagnostics.
type Run struct {
	RuleID  string    `json:"rule_id"`
	Rule    string    `json:"rule"`
	Source  string    `json:"source"`
	At      time.Time `json:"at"`
	Result  string    `json:"result"`
	Error   string    `json:"error,omitempty"`
	Skipped string    `json:"skipped,omitempty"`
}

type Engine struct {
	cfg      *config.Config
	browser  Browser
	hardware Hardware
	logger   *slog.Logger
	journal  *events.Journal
	now      func() time.Time

	mu           sync.Mutex
	lastFired    map[string]time.Time
	timeFired    map[string]string
	idleFired    map[string]bool
	history      []Run
	started      time.Time
	displayTimer *time.Timer
	running   sync.Mutex
}

func New(cfg *config.Config, browser Browser, hardware Hardware, logger *slog.Logger, journal *events.Journal) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		cfg: cfg, browser: browser, hardware: hardware, logger: logger, journal: journal, now: time.Now,
		lastFired: map[string]time.Time{}, timeFired: map[string]string{}, idleFired: map[string]bool{},
		started: time.Now(),
	}
}

// Run evaluates time and idle triggers every 5 seconds until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		e.checkTime(ctx)
		e.checkIdle(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (e *Engine) checkTime(ctx context.Context) {
	cfg := e.cfg.Snapshot()
	now := localNow(e.now(), cfg.Time.Timezone)
	clock := now.Format("15:04")
	day := now.Format("2006-01-02")
	for _, rule := range cfg.Kiosk.Rules {
		if rule.Disabled || rule.Trigger.Type != "time" || rule.Trigger.At != clock || !dayMatches(rule.Trigger.Days, now.Weekday()) {
			continue
		}
		e.mu.Lock()
		already := e.timeFired[rule.ID] == day+" "+clock
		if !already {
			e.timeFired[rule.ID] = day + " " + clock
		}
		e.mu.Unlock()
		if !already {
			e.fire(ctx, cfg, rule, "time")
		}
	}
}

// checkIdle fires idle rules once the kiosk page has seen no input for the
// rule's idle time. A rule fires once per idle period; input re-arms it.
func (e *Engine) checkIdle(ctx context.Context) {
	cfg := e.cfg.Snapshot()
	last := e.browser.LastActivity()
	e.mu.Lock()
	if last.IsZero() || last.Before(e.started) {
		last = e.started
	}
	e.mu.Unlock()
	idle := e.now().Sub(last)
	for _, rule := range cfg.Kiosk.Rules {
		if rule.Disabled || rule.Trigger.Type != "idle" || idle < time.Duration(rule.Trigger.IdleSeconds)*time.Second {
			continue
		}
		e.mu.Lock()
		already := e.idleFired[rule.ID]
		e.idleFired[rule.ID] = true
		e.mu.Unlock()
		if !already {
			e.fire(ctx, cfg, rule, "idle")
		}
	}
}

// HandleActivity is called on user input on the kiosk page: it fires touch
// rules and re-arms idle rules.
func (e *Engine) HandleActivity(ctx context.Context) {
	e.mu.Lock()
	clear(e.idleFired)
	e.mu.Unlock()
	cfg := e.cfg.Snapshot()
	for _, rule := range cfg.Kiosk.Rules {
		if !rule.Disabled && rule.Trigger.Type == "touch" {
			e.fire(ctx, cfg, rule, "touch")
		}
	}
}

// Topics returns the MQTT topics rules listen on.
func (e *Engine) Topics() []string {
	var topics []string
	seen := map[string]bool{}
	for _, rule := range e.cfg.Snapshot().Kiosk.Rules {
		topic := strings.TrimSpace(rule.Trigger.Topic)
		if rule.Disabled || rule.Trigger.Type != "mqtt" || topic == "" || seen[topic] {
			continue
		}
		seen[topic] = true
		topics = append(topics, topic)
	}
	return topics
}

// HandleMQTT fires every enabled rule whose topic and payload match. It
// reports whether any rule listens on the topic, so the caller does not
// interpret the message as a KioskMate command.
func (e *Engine) HandleMQTT(ctx context.Context, topic, payload string) bool {
	cfg := e.cfg.Snapshot()
	listened := false
	for _, rule := range cfg.Kiosk.Rules {
		if rule.Disabled || rule.Trigger.Type != "mqtt" || strings.TrimSpace(rule.Trigger.Topic) != topic {
			continue
		}
		listened = true
		if expected := strings.TrimSpace(rule.Trigger.Payload); expected != "" && !strings.EqualFold(expected, strings.TrimSpace(payload)) {
			continue
		}
		e.fire(ctx, cfg, rule, "mqtt")
	}
	return listened
}

// ErrUnknownRule is returned for webhook and manual calls to a missing rule.
var ErrUnknownRule = errors.New("workflow rule not found")

// HandleWebhook fires a webhook rule when the token matches.
func (e *Engine) HandleWebhook(ctx context.Context, id, token string) error {
	cfg := e.cfg.Snapshot()
	for _, rule := range cfg.Kiosk.Rules {
		if rule.ID != id || rule.Trigger.Type != "webhook" || rule.Disabled {
			continue
		}
		expected := strings.TrimSpace(rule.Trigger.Token)
		if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(token)) != 1 {
			return ErrUnknownRule
		}
		return e.fire(ctx, cfg, rule, "webhook")
	}
	return ErrUnknownRule
}

// Test runs a rule's actions now, ignoring trigger, conditions and cooldown.
func (e *Engine) Test(ctx context.Context, id string) error {
	cfg := e.cfg.Snapshot()
	for _, rule := range cfg.Kiosk.Rules {
		if rule.ID == id {
			return e.execute(ctx, cfg, rule, "test")
		}
	}
	return ErrUnknownRule
}

// History returns the most recent rule runs, newest first.
func (e *Engine) History() []Run {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Run, len(e.history))
	for i, run := range e.history {
		out[len(e.history)-1-i] = run
	}
	return out
}

func (e *Engine) fire(ctx context.Context, cfg *config.Config, rule config.WorkflowRule, source string) error {
	now := e.now()
	if reason := e.blocked(cfg, rule, now); reason != "" {
		e.record(Run{RuleID: rule.ID, Rule: rule.Name, Source: source, At: now, Result: "skipped", Skipped: reason})
		return nil
	}
	e.mu.Lock()
	e.lastFired[rule.ID] = now
	e.mu.Unlock()
	return e.execute(ctx, cfg, rule, source)
}

// blocked returns why conditions or the cooldown prevent a rule, or "".
func (e *Engine) blocked(cfg *config.Config, rule config.WorkflowRule, now time.Time) string {
	if rule.CooldownSeconds > 0 {
		e.mu.Lock()
		last := e.lastFired[rule.ID]
		e.mu.Unlock()
		if !last.IsZero() && now.Sub(last) < time.Duration(rule.CooldownSeconds)*time.Second {
			return "cooldown"
		}
	}
	local := localNow(now, cfg.Time.Timezone)
	conditions := rule.Conditions
	if !dayMatches(conditions.Days, local.Weekday()) {
		return "day"
	}
	if conditions.Start != "" && !inWindow(local, conditions.Start, conditions.End) {
		return "time_window"
	}
	if id := conditions.ActivePageID; id != "" && cfg.Kiosk.EnabledPageIndex(id) != e.browser.ActivePage() {
		return "active_page"
	}
	return ""
}

func (e *Engine) execute(ctx context.Context, cfg *config.Config, rule config.WorkflowRule, source string) error {
	// One rule at a time: actions of two rules must not interleave.
	e.running.Lock()
	defer e.running.Unlock()
	run := Run{RuleID: rule.ID, Rule: rule.Name, Source: source, At: e.now(), Result: "ok"}
	var failures []string
	for index, action := range rule.Actions {
		actionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := e.action(actionCtx, cfg, action)
		cancel()
		if errors.Is(err, errManualOverride) {
			run.Skipped = "manual_override"
			continue
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%d %s: %v", index+1, action.Type, err))
		}
	}
	if len(failures) > 0 {
		run.Result = "failed"
		run.Error = strings.Join(failures, "; ")
		e.logger.Warn("workflow rule failed", "rule", rule.Name, "source", source, "error", run.Error)
	} else {
		e.logger.Info("workflow rule ran", "rule", rule.Name, "source", source)
	}
	e.record(run)
	if run.Error != "" {
		return errors.New(run.Error)
	}
	return nil
}

func (e *Engine) action(ctx context.Context, cfg *config.Config, action config.RuleAction) error {
	duration := time.Duration(action.DurationSeconds) * time.Second
	switch action.Type {
	case "show_page":
		index := cfg.Kiosk.EnabledPageIndex(action.PageID)
		if index < 0 {
			return errors.New("page is disabled or missing")
		}
		if e.manualOverride() {
			return errManualOverride
		}
		if action.UntilResume {
			duration = 24 * time.Hour
		}
		return e.browser.SetOverride(ctx, index, duration, workflowSource)
	case "resume_playlist":
		active, source := e.browser.OverrideSource()
		if !active {
			return nil
		}
		if source != workflowSource {
			return errManualOverride
		}
		return e.browser.ClearOverride()
	case "click":
		return e.browser.Evaluate(ctx, clickScript(action.Value))
	case "scroll":
		return e.browser.Evaluate(ctx, scrollScript(action.Value))
	case "sound":
		return e.browser.Evaluate(ctx, soundScript(action.Value))
	case "reload":
		return e.browser.Reload(ctx)
	case "overlay":
		return e.browser.ShowOverlay(ctx, action.Value, duration)
	case "script":
		return e.browser.Evaluate(ctx, action.Value)
	case "display":
		if e.hardware == nil {
			return errors.New("display control unavailable")
		}
		power := strings.ToUpper(strings.TrimSpace(action.Value))
		if err := e.setDisplay(ctx, power); err != nil {
			return err
		}
		e.scheduleDisplayRevert(power, duration)
		return nil
	case "brightness":
		if e.hardware == nil {
			return errors.New("brightness control unavailable")
		}
		value, err := strconv.Atoi(strings.TrimSpace(action.Value))
		if err != nil {
			return err
		}
		return e.hardware.SetBrightness(ctx, value)
	default:
		return fmt.Errorf("unsupported action %q", action.Type)
	}
}

// workflowSource marks page overrides set by rules. Priority is fixed:
// a manual override (Admin UI, Home Assistant) beats rules, rules beat the
// schedule and the rotation.
const workflowSource = "workflow"

var errManualOverride = errors.New("manual override active")

func (e *Engine) manualOverride() bool {
	active, source := e.browser.OverrideSource()
	return active && source != workflowSource
}

func (e *Engine) setDisplay(ctx context.Context, power string) error {
	if err := e.hardware.SetDisplay(ctx, power); err != nil {
		return err
	}
	e.browser.NoteDisplayPower(power)
	return nil
}

// scheduleDisplayRevert switches the display back after duration, e.g.
// "on motion: display on for 2 minutes". A newer display action replaces a
// pending revert.
func (e *Engine) scheduleDisplayRevert(power string, duration time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.displayTimer != nil {
		e.displayTimer.Stop()
		e.displayTimer = nil
	}
	if duration <= 0 {
		return
	}
	revert := "OFF"
	if power == "OFF" {
		revert = "ON"
	}
	e.displayTimer = time.AfterFunc(duration, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := e.setDisplay(ctx, revert); err != nil {
			e.logger.Warn("workflow display revert failed", "power", revert, "error", err)
		}
	})
}

func (e *Engine) record(run Run) {
	e.mu.Lock()
	e.history = append(e.history, run)
	if len(e.history) > 50 {
		e.history = e.history[len(e.history)-50:]
	}
	journal := e.journal
	e.mu.Unlock()
	if journal != nil {
		status := run.Result
		message := run.Rule
		if run.Skipped != "" {
			message += " (" + run.Skipped + ")"
		}
		if run.Error != "" {
			message += ": " + run.Error
		}
		journal.Record("workflow", "rule", status, message, map[string]string{"rule_id": run.RuleID, "source": run.Source})
	}
}

func localNow(now time.Time, timezone string) time.Time {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" || timezone == "Local" {
		return now
	}
	if location, err := time.LoadLocation(timezone); err == nil {
		return now.In(location)
	}
	return now
}

func dayMatches(days []string, weekday time.Weekday) bool {
	if len(days) == 0 {
		return true
	}
	short := strings.ToLower(weekday.String()[:3])
	for _, day := range days {
		day = strings.ToLower(strings.TrimSpace(day))
		if day == short || day == strings.ToLower(weekday.String()) {
			return true
		}
	}
	return false
}

func clockMinutes(value string) (int, bool) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}

// inWindow reports whether now lies in [start, end), across midnight when
// end is before start.
func inWindow(now time.Time, start, end string) bool {
	from, ok1 := clockMinutes(start)
	to, ok2 := clockMinutes(end)
	if !ok1 || !ok2 {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if from <= to {
		return current >= from && current < to
	}
	return current >= from || current < to
}
