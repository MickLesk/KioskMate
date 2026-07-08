package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/system"
)

type Browser struct {
	cfg    *config.Config
	logger *slog.Logger

	mu            sync.Mutex
	cmd           *exec.Cmd
	stopping      bool
	started       time.Time
	active        int
	lastStat      system.ProcessTreeStats
	hotSince      time.Time
	lastError     string
	lastExit      time.Time
	scheduler     SchedulerStatus
	rotationIndex int
	rotationUntil time.Time
}

type Status struct {
	Running   bool                    `json:"running"`
	PID       int                     `json:"pid,omitempty"`
	Started   *time.Time              `json:"started,omitempty"`
	Command   string                  `json:"command"`
	Args      []string                `json:"args"`
	Stats     system.ProcessTreeStats `json:"stats"`
	Active    int                     `json:"active"`
	PageName  string                  `json:"page_name"`
	URL       string                  `json:"url"`
	Scheduler SchedulerStatus         `json:"scheduler"`
	LastError string                  `json:"last_error,omitempty"`
	LastExit  *time.Time              `json:"last_exit,omitempty"`
}

type SchedulerStatus struct {
	Enabled       bool       `json:"enabled"`
	Mode          string     `json:"mode"`
	Reason        string     `json:"reason"`
	ActiveRule    string     `json:"active_rule,omitempty"`
	NextSwitch    *time.Time `json:"next_switch,omitempty"`
	RotationIndex int        `json:"rotation_index,omitempty"`
}

func NewBrowser(cfg *config.Config, logger *slog.Logger) *Browser {
	return &Browser{cfg: cfg, logger: logger}
}

func (b *Browser) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd != nil && b.cmd.Process != nil {
		return nil
	}
	b.stopping = false
	command, args, err := b.command()
	if err != nil {
		b.lastError = err.Error()
		return err
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = processGroupAttr()
	if err := cmd.Start(); err != nil {
		b.lastError = err.Error()
		return err
	}
	b.cmd = cmd
	b.started = time.Now()
	b.lastStat = system.ProcessTreeStats{}
	b.hotSince = time.Time{}
	b.lastError = ""
	b.logger.Info("browser started", "pid", cmd.Process.Pid, "command", command)

	go b.wait(cmd)
	go b.watch(ctx, cmd.Process.Pid)
	return nil
}

func (b *Browser) Stop(ctx context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	if cmd != nil {
		b.stopping = true
	}
	b.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- terminateProcessTree(cmd.Process.Pid)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Browser) Restart(ctx context.Context) error {
	if err := b.Stop(ctx); err != nil {
		return err
	}
	time.Sleep(750 * time.Millisecond)
	return b.Start(ctx)
}

func (b *Browser) Reload(ctx context.Context) error {
	return b.Restart(ctx)
}

func (b *Browser) Next(ctx context.Context) error {
	b.mu.Lock()
	if b.cfg.Kiosk.PageCount() > 0 {
		b.active = (b.active + 1) % b.cfg.Kiosk.PageCount()
	}
	b.mu.Unlock()
	return b.Restart(ctx)
}

func (b *Browser) Previous(ctx context.Context) error {
	b.mu.Lock()
	if b.cfg.Kiosk.PageCount() > 0 {
		b.active--
		if b.active < 0 {
			b.active = b.cfg.Kiosk.PageCount() - 1
		}
	}
	b.mu.Unlock()
	return b.Restart(ctx)
}

func (b *Browser) SetActive(ctx context.Context, index int) error {
	b.mu.Lock()
	if index < 0 || index >= b.cfg.Kiosk.PageCount() {
		b.mu.Unlock()
		return fmt.Errorf("page index out of range")
	}
	b.active = index
	b.mu.Unlock()
	return b.Restart(ctx)
}

func (b *Browser) ResetSession(ctx context.Context) error {
	if err := b.Stop(ctx); err != nil {
		return err
	}
	dir := b.cfg.Kiosk.UserDataDir
	if dir == "" {
		return nil
	}
	for _, name := range []string{"Default/Cookies", "Default/Local Storage", "Default/IndexedDB", "Default/Service Worker", "Default/Session Storage", "Default/Cache", "Default/Code Cache"} {
		_ = os.RemoveAll(filepath.Join(dir, filepath.FromSlash(name)))
	}
	return b.Start(ctx)
}

func (b *Browser) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	command, args, err := b.command()
	status := Status{Command: command, Args: args, Stats: b.lastStat, Active: b.active, PageName: b.cfg.Kiosk.PageName(b.active), URL: b.activeURL(), Scheduler: b.scheduler, LastError: b.lastError}
	if !b.lastExit.IsZero() {
		lastExit := b.lastExit
		status.LastExit = &lastExit
	}
	if err != nil {
		status.LastError = err.Error()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		status.Running = true
		status.PID = b.cmd.Process.Pid
		started := b.started
		status.Started = &started
	}
	return status
}

func (b *Browser) RunScheduler(ctx context.Context) {
	tick := b.cfg.Kiosk.Scheduler.TickInterval
	if tick <= 0 {
		tick = 15 * time.Second
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			target, status := b.schedulerTarget(now)
			b.mu.Lock()
			current := b.active
			b.scheduler = status
			b.mu.Unlock()
			if target >= 0 && target != current {
				b.logger.Info("scheduler page switch", "page", target, "reason", status.Reason)
				switchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				_ = b.SetActive(switchCtx, target)
				cancel()
			}
		}
	}
}

func (b *Browser) wait(cmd *exec.Cmd) {
	err := cmd.Wait()
	b.mu.Lock()
	expected := b.stopping
	if b.cmd == cmd {
		b.cmd = nil
	}
	b.lastExit = time.Now()
	if expected {
		b.lastError = ""
		b.stopping = false
	} else if err != nil && !errors.Is(err, os.ErrProcessDone) {
		b.lastError = err.Error()
	}
	b.mu.Unlock()
	if err != nil && !expected && !errors.Is(err, os.ErrProcessDone) {
		b.logger.Warn("browser exited", "error", err)
		return
	}
	b.logger.Info("browser exited")
}

func (b *Browser) watch(ctx context.Context, pid int) {
	if !b.cfg.Watchdog.Enabled {
		return
	}
	ticker := time.NewTicker(b.cfg.Watchdog.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats, err := system.ReadProcessTreeStats(pid, b.lastStat)
			if err != nil {
				return
			}
			b.mu.Lock()
			b.lastStat = stats
			restart := b.shouldRestart(stats)
			b.mu.Unlock()
			if restart {
				b.logger.Warn("browser watchdog restart", "rss_mb", stats.RSSMB, "cpu", stats.CPUPercent)
				_ = b.Restart(ctx)
				return
			}
		}
	}
}

func (b *Browser) shouldRestart(stats system.ProcessTreeStats) bool {
	overRSS := b.cfg.Watchdog.MaxRSSMB > 0 && stats.RSSMB > b.cfg.Watchdog.MaxRSSMB
	overCPU := b.cfg.Watchdog.MaxCPUPercent > 0 && stats.CPUPercent > b.cfg.Watchdog.MaxCPUPercent
	if !overRSS && !overCPU {
		b.hotSince = time.Time{}
		return false
	}
	if b.hotSince.IsZero() {
		b.hotSince = time.Now()
		return false
	}
	return time.Since(b.hotSince) >= b.cfg.Watchdog.CPUGrace
}

func (b *Browser) command() (string, []string, error) {
	preset := browserPreset(b.cfg.Kiosk.BrowserPreset)
	command := b.cfg.Kiosk.BrowserCommand
	if command == "" || preset != "custom" {
		command = findBrowser(preset)
	}
	if command == "" {
		return "", nil, fmt.Errorf("no browser found for preset %q, install a supported browser or use custom command", preset)
	}
	args := browserArgs(b.cfg, preset, b.activeURL(), b.cfg.Kiosk.ExtraArgs)
	return command, args, nil
}

func (b *Browser) activeURL() string {
	urls := b.cfg.Kiosk.PageURLs()
	if len(urls) == 0 {
		return "about:blank"
	}
	if b.active < 0 || b.active >= len(urls) {
		b.active = 0
	}
	return urls[b.active]
}

func (b *Browser) schedulerTarget(now time.Time) (int, SchedulerStatus) {
	cfg := b.cfg.Kiosk
	status := SchedulerStatus{Enabled: cfg.Scheduler.Enabled, Mode: cfg.Scheduler.Mode}
	if !cfg.Scheduler.Enabled || cfg.PageCount() == 0 {
		status.Reason = "disabled"
		return -1, status
	}
	mode := cfg.Scheduler.Mode
	if mode == "" {
		mode = "rotation"
		status.Mode = mode
	}
	if mode == "time" || mode == "hybrid" {
		if page, rule, next, ok := timeRuleTarget(cfg, now); ok {
			status.Reason = "time"
			status.ActiveRule = rule
			status.NextSwitch = &next
			return page, status
		}
		if mode == "time" {
			status.Reason = "no active time rule"
			return -1, status
		}
	}
	if mode == "rotation" || mode == "hybrid" {
		return b.rotationTarget(now, cfg, status)
	}
	status.Reason = "unsupported mode"
	return -1, status
}

func (b *Browser) rotationTarget(now time.Time, cfg config.KioskConfig, status SchedulerStatus) (int, SchedulerStatus) {
	if len(cfg.Rotation) == 0 {
		status.Reason = "no rotation items"
		return -1, status
	}
	if b.rotationUntil.IsZero() {
		b.rotationIndex = 0
	} else if !now.Before(b.rotationUntil) {
		b.rotationIndex = (b.rotationIndex + 1) % len(cfg.Rotation)
	}
	if b.rotationUntil.IsZero() || !now.Before(b.rotationUntil) {
		item := cfg.Rotation[b.rotationIndex]
		duration := time.Duration(item.DurationSeconds) * time.Second
		if duration <= 0 {
			duration = time.Hour
		}
		b.rotationUntil = now.Add(duration)
	}
	item := cfg.Rotation[b.rotationIndex]
	page := clampPage(item.Page, cfg.PageCount())
	status.Reason = "rotation"
	status.RotationIndex = b.rotationIndex
	next := b.rotationUntil
	status.NextSwitch = &next
	return page, status
}

func timeRuleTarget(cfg config.KioskConfig, now time.Time) (int, string, time.Time, bool) {
	for _, rule := range cfg.TimeRules {
		if rule.Disabled || !dayMatches(rule.Days, now.Weekday()) {
			continue
		}
		start, ok1 := parseClock(rule.Start)
		end, ok2 := parseClock(rule.End)
		if !ok1 || !ok2 {
			continue
		}
		current := now.Hour()*60 + now.Minute()
		active := false
		if start <= end {
			active = current >= start && current < end
		} else {
			active = current >= start || current < end
		}
		if active {
			next := time.Date(now.Year(), now.Month(), now.Day(), end/60, end%60, 0, 0, now.Location())
			if start > end && current >= start {
				next = next.Add(24 * time.Hour)
			}
			name := rule.Name
			if name == "" {
				name = fmt.Sprintf("%s-%s", rule.Start, rule.End)
			}
			return clampPage(rule.Page, cfg.PageCount()), name, next, true
		}
	}
	return -1, "", time.Time{}, false
}

func parseClock(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func dayMatches(days []string, weekday time.Weekday) bool {
	if len(days) == 0 {
		return true
	}
	want := strings.ToLower(weekday.String()[:3])
	for _, day := range days {
		day = strings.ToLower(strings.TrimSpace(day))
		if day == want || day == strings.ToLower(weekday.String()) {
			return true
		}
	}
	return false
}

func clampPage(page int, count int) int {
	if count <= 0 {
		return -1
	}
	if page < 0 {
		return 0
	}
	if page >= count {
		return count - 1
	}
	return page
}

func browserPreset(preset string) string {
	switch strings.TrimSpace(strings.ToLower(preset)) {
	case "chromium", "chromium-lite", "firefox", "webkit-cog", "epiphany", "midori", "custom":
		return strings.TrimSpace(strings.ToLower(preset))
	default:
		return "chromium"
	}
}

func findBrowser(preset string) string {
	candidates := browserCandidates(preset)
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func browserCandidates(preset string) []string {
	switch preset {
	case "firefox":
		return []string{"firefox-esr", "firefox"}
	case "webkit-cog":
		return []string{"cog"}
	case "epiphany":
		return []string{"epiphany-browser", "epiphany"}
	case "midori":
		return []string{"midori"}
	default:
		return []string{"chromium-browser", "chromium", "google-chrome-stable", "google-chrome", "microsoft-edge"}
	}
}

func browserArgs(cfg *config.Config, preset string, url string, extra []string) []string {
	switch preset {
	case "firefox":
		return append(firefoxArgs(cfg), append(extra, url)...)
	case "webkit-cog":
		return append(extra, url)
	case "epiphany":
		return append([]string{"--application-mode"}, append(extra, url)...)
	case "midori":
		return append([]string{"-e", "Fullscreen"}, append(extra, url)...)
	case "custom":
		return append(extra, url)
	case "chromium-lite":
		args := append(chromiumArgs(cfg), chromiumLiteArgs()...)
		return append(args, append(extra, url)...)
	default:
		return append(chromiumArgs(cfg), append(extra, url)...)
	}
}

func chromiumArgs(cfg *config.Config) []string {
	args := []string{
		"--kiosk",
		"--user-data-dir=" + cfg.Kiosk.UserDataDir,
		"--no-first-run",
		"--disable-translate",
		"--disable-session-crashed-bubble",
		"--disable-infobars",
		"--autoplay-policy=user-gesture-required",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-features=TranslateUI,MediaRouter,OptimizationHints",
	}
	if cfg.Performance.Profile == "raspberry" || cfg.Performance.Profile == "minimal" || cfg.Performance.ReduceMotion {
		args = append(args, "--disable-smooth-scrolling")
	}
	if cfg.Kiosk.ZoomPercent > 0 && cfg.Kiosk.ZoomPercent != 100 {
		args = append(args, fmt.Sprintf("--force-device-scale-factor=%.2f", float64(cfg.Kiosk.ZoomPercent)/100.0))
	}
	if cfg.Performance.GPUMode == "software" {
		args = append(args, "--disable-gpu", "--disable-gpu-compositing")
	}
	return args
}

func chromiumLiteArgs() []string {
	return []string{
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--disable-sync",
		"--disable-print-preview",
		"--disable-speech-api",
		"--disable-notifications",
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
		"--renderer-process-limit=2",
		"--process-per-site",
	}
}

func firefoxArgs(cfg *config.Config) []string {
	args := []string{"--kiosk"}
	if cfg.Kiosk.UserDataDir != "" {
		args = append(args, "--profile", cfg.Kiosk.UserDataDir)
	}
	return args
}
