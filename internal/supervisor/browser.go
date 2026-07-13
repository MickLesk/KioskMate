package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/logutil"
	"github.com/MickLesk/KioskMate/internal/system"
)

type Browser struct {
	cfg    *config.Config
	logger *slog.Logger

	mu            sync.Mutex
	cmd           *exec.Cmd
	done          chan struct{}
	stopping      bool
	started       time.Time
	startCount    int
	restartCount  int
	active        int
	lastStat      system.ProcessTreeStats
	hotSince      time.Time
	lastError     string
	lastExit      time.Time
	watchdog      WatchdogStatus
	watchdogRuns  []time.Time
	scheduler     SchedulerStatus
	rotationIndex int
	rotationUntil time.Time
	devTools      bool
	themeStatus   ThemeStatus
	authGuard     AuthGuardStatus
}

type Status struct {
	Running    bool                    `json:"running"`
	PID        int                     `json:"pid,omitempty"`
	Started    *time.Time              `json:"started,omitempty"`
	StartCount int                     `json:"start_count"`
	Restarts   int                     `json:"restart_count"`
	Command    string                  `json:"command"`
	Args       []string                `json:"args"`
	Stats      system.ProcessTreeStats `json:"stats"`
	Active     int                     `json:"active"`
	PageName   string                  `json:"page_name"`
	URL        string                  `json:"url"`
	Scheduler  SchedulerStatus         `json:"scheduler"`
	Watchdog   WatchdogStatus          `json:"watchdog"`
	LastError  string                  `json:"last_error,omitempty"`
	LastExit   *time.Time              `json:"last_exit,omitempty"`
	DevTools   bool                    `json:"devtools"`
	Theme      ThemeStatus             `json:"theme_status"`
	AuthGuard  AuthGuardStatus         `json:"auth_guard"`
}

type ThemeStatus struct {
	State          string     `json:"state"`
	Configured     string     `json:"configured"`
	RequestedTheme string     `json:"requested_theme,omitempty"`
	RequestedDark  *bool      `json:"requested_dark,omitempty"`
	SelectedTheme  string     `json:"selected_theme,omitempty"`
	SelectedDark   *bool      `json:"selected_dark,omitempty"`
	AppliedDark    *bool      `json:"applied_dark,omitempty"`
	Error          string     `json:"error,omitempty"`
	Updated        *time.Time `json:"updated,omitempty"`
}

type AuthGuardStatus struct {
	Tripped bool       `json:"tripped"`
	Reason  string     `json:"reason,omitempty"`
	At      *time.Time `json:"at,omitempty"`
}

type WatchdogStatus struct {
	Enabled            bool       `json:"enabled"`
	MaxRSSMB           uint64     `json:"max_rss_mb"`
	MaxCPUPercent      float64    `json:"max_cpu_percent"`
	CheckInterval      int        `json:"check_interval_seconds"`
	CPUGrace           int        `json:"cpu_grace_seconds"`
	HotSince           *time.Time `json:"hot_since,omitempty"`
	LastRestart        *time.Time `json:"last_restart,omitempty"`
	LastReason         string     `json:"last_reason,omitempty"`
	LastAction         string     `json:"last_action,omitempty"`
	SuppressedUntil    *time.Time `json:"suppressed_until,omitempty"`
	RestartWindowCount int        `json:"restart_window_count"`
	Pressure           string     `json:"pressure"`
}

const (
	watchdogRestartWindow       = 30 * time.Minute
	watchdogSuppressDuration    = 30 * time.Minute
	watchdogMaxRestartsInWindow = 3
	watchdogMinCPUOnlyGrace     = 10 * time.Minute
)

type SchedulerStatus struct {
	Enabled       bool       `json:"enabled"`
	Mode          string     `json:"mode"`
	Reason        string     `json:"reason"`
	ActiveRule    string     `json:"active_rule,omitempty"`
	NextSwitch    *time.Time `json:"next_switch,omitempty"`
	RotationIndex int        `json:"rotation_index,omitempty"`
}

func NewBrowser(cfg *config.Config, logger *slog.Logger) *Browser {
	browser := &Browser{cfg: cfg, logger: logger}
	browser.loadAuthGuard()
	return browser
}

func (b *Browser) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.cmd != nil && b.cmd.Process != nil {
		return nil
	}
	if b.authGuard.Tripped {
		return fmt.Errorf("Home Assistant authentication guard is active: %s; reset the browser session before starting", b.authGuard.Reason)
	}
	b.stopping = false
	command, args, err := b.command()
	if err != nil {
		b.lastError = err.Error()
		return err
	}
	// The browser lifetime belongs to the supervisor, not to the HTTP/MQTT
	// request that asked for it to start.
	cmd := exec.Command(command, args...)
	logFile := b.openBrowserLog()
	if logFile != nil {
		writer := io.MultiWriter(os.Stdout, logFile)
		cmd.Stdout = writer
		cmd.Stderr = writer
		writeBrowserLaunchLog(logFile, command, args)
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.SysProcAttr = processGroupAttr()
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_, _ = fmt.Fprintf(logFile, "start error: %v\n", err)
		}
		if logFile != nil {
			_ = logFile.Close()
		}
		b.lastError = err.Error()
		return err
	}
	b.cmd = cmd
	b.done = make(chan struct{})
	b.started = time.Now()
	b.startCount++
	b.lastStat = system.ProcessTreeStats{}
	b.hotSince = time.Time{}
	b.lastError = ""
	b.devTools = false
	configuredTheme := b.cfg.Snapshot().Kiosk.Theme
	b.themeStatus = ThemeStatus{State: "pending", Configured: configuredTheme}
	b.logger.Info("browser started", "pid", cmd.Process.Pid, "command", command, "args", args)
	if logFile != nil {
		_, _ = fmt.Fprintf(logFile, "started pid: %d\n", cmd.Process.Pid)
	}

	go b.wait(cmd, logFile)
	go b.watch(cmd.Process.Pid, b.done)
	cfg := b.cfg.Snapshot()
	if supportsCDP(browserPreset(cfg.Kiosk.BrowserPreset)) {
		go b.monitorDevTools(browserUserDataDir(cfg, b.active), cfg.Kiosk.Theme, b.done)
	}
	return nil
}

func writeBrowserLaunchLog(file *os.File, command string, args []string) {
	_, _ = fmt.Fprintf(file, "command: %s\n", command)
	_, _ = fmt.Fprintf(file, "args: %s\n", strings.Join(args, " "))
	_, _ = fmt.Fprintf(file, "env DISPLAY=%s WAYLAND_DISPLAY=%s XDG_RUNTIME_DIR=%s XDG_SESSION_TYPE=%s\n",
		os.Getenv("DISPLAY"),
		os.Getenv("WAYLAND_DISPLAY"),
		os.Getenv("XDG_RUNTIME_DIR"),
		os.Getenv("XDG_SESSION_TYPE"),
	)
}

func (b *Browser) openBrowserLog() *os.File {
	path := config.BrowserLogFilePath(b.cfg.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		b.logger.Warn("browser log directory failed", "path", path, "error", err)
		return nil
	}
	_ = logutil.Rotate(path, 5<<20, 3)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		b.logger.Warn("browser log open failed", "path", path, "error", err)
		return nil
	}
	_, _ = fmt.Fprintf(file, "\n--- browser start %s ---\n", time.Now().Format(time.RFC3339))
	return file
}

func (b *Browser) Stop(ctx context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	done := b.done
	if cmd != nil {
		b.stopping = true
	}
	b.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	terminated := make(chan error, 1)
	go func() {
		terminated <- terminateProcessTree(cmd.Process.Pid)
	}()
	select {
	case err := <-terminated:
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Browser) Restart(ctx context.Context) error {
	b.mu.Lock()
	b.restartCount++
	b.mu.Unlock()
	if err := b.Stop(ctx); err != nil {
		return err
	}
	return b.Start(ctx)
}

func (b *Browser) Reload(ctx context.Context) error {
	if err := b.reloadDevTools(ctx); err == nil {
		return nil
	}
	return b.Restart(ctx)
}

func (b *Browser) Next(ctx context.Context) error {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	active := b.active
	if cfg.Kiosk.PageCount() > 0 {
		active = (active + 1) % cfg.Kiosk.PageCount()
	}
	b.mu.Unlock()
	return b.SetActive(ctx, active)
}

func (b *Browser) Previous(ctx context.Context) error {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	active := b.active - 1
	if active < 0 {
		active = cfg.Kiosk.PageCount() - 1
	}
	b.mu.Unlock()
	return b.SetActive(ctx, active)
}

func (b *Browser) SetActive(ctx context.Context, index int) error {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	if index < 0 || index >= cfg.Kiosk.PageCount() {
		b.mu.Unlock()
		return fmt.Errorf("page index out of range")
	}
	previous := b.active
	b.active = index
	target := activeURL(cfg, b.active)
	running := b.cmd != nil && b.cmd.Process != nil
	isolate := cfg.Kiosk.IsolateSessions
	b.mu.Unlock()
	if !running {
		return b.Start(ctx)
	}
	if !isolate || previous == index {
		if err := b.navigateDevTools(ctx, target); err == nil {
			return nil
		}
	}
	return b.Restart(ctx)
}

func (b *Browser) ResetSession(ctx context.Context) error {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	active := b.active
	b.mu.Unlock()
	if err := b.Stop(ctx); err != nil {
		return err
	}
	dir := browserUserDataDir(cfg, active)
	if dir == "" {
		return nil
	}
	if err := backupAndResetSession(dir); err != nil {
		return err
	}
	b.clearAuthGuard()
	return b.Start(ctx)
}

func (b *Browser) CaptureScreenshot(ctx context.Context) ([]byte, error) {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	if b.cmd == nil || b.cmd.Process == nil {
		b.mu.Unlock()
		return nil, errors.New("browser is not running")
	}
	profile := browserUserDataDir(cfg, b.active)
	b.mu.Unlock()
	return captureDevToolsScreenshot(ctx, profile)
}

func (b *Browser) TripAuthGuard(reason string) {
	b.tripAuthGuard(reason)
}

func (b *Browser) Status() Status {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	defer b.mu.Unlock()
	command, args, err := b.command()
	status := Status{Command: command, Args: args, Stats: b.lastStat, Active: b.active, PageName: cfg.Kiosk.PageName(b.active), URL: activeURL(cfg, b.active), Scheduler: b.scheduler, Watchdog: b.watchdogStatusLocked(), StartCount: b.startCount, Restarts: b.restartCount, LastError: b.lastError, DevTools: b.devTools, Theme: b.themeStatus, AuthGuard: b.authGuard}
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
	timer := time.NewTimer(b.schedulerInterval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
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
			timer.Reset(b.schedulerInterval())
		}
	}
}

func (b *Browser) schedulerInterval() time.Duration {
	tick := b.cfg.Snapshot().Kiosk.Scheduler.TickInterval
	if tick <= 0 {
		return 15 * time.Second
	}
	return tick
}

func (b *Browser) wait(cmd *exec.Cmd, logFile *os.File) {
	if logFile != nil {
		defer logFile.Close()
	}
	err := cmd.Wait()
	b.mu.Lock()
	expected := b.stopping
	runtime := time.Duration(0)
	if b.cmd == cmd && !b.started.IsZero() {
		runtime = time.Since(b.started)
	}
	if b.cmd == cmd {
		b.cmd = nil
	}
	done := b.done
	if b.cmd == nil {
		b.done = nil
		b.devTools = false
	}
	b.lastExit = time.Now()
	if expected {
		b.lastError = ""
		b.stopping = false
	} else if err != nil && !errors.Is(err, os.ErrProcessDone) {
		b.lastError = err.Error()
		if runtime > 0 && runtime < 5*time.Second {
			b.lastError = fmt.Sprintf("browser exited after %s: %v", runtime.Round(time.Millisecond), err)
		}
	} else if runtime > 0 && runtime < 5*time.Second {
		b.lastError = fmt.Sprintf("browser exited after %s without error", runtime.Round(time.Millisecond))
	}
	lastError := b.lastError
	b.mu.Unlock()
	if done != nil {
		close(done)
	}
	if logFile != nil {
		if err != nil {
			_, _ = fmt.Fprintf(logFile, "exit: %v runtime=%s\n", err, runtime)
		} else {
			_, _ = fmt.Fprintf(logFile, "exit: ok runtime=%s\n", runtime)
		}
		if !expected && lastError != "" {
			_, _ = fmt.Fprintf(logFile, "last error: %s\n", lastError)
		}
	}
	if !expected && lastError != "" {
		b.writeCrashDiagnostic(cmd, runtime, lastError)
	}
	if err != nil && !expected && !errors.Is(err, os.ErrProcessDone) {
		b.logger.Warn("browser exited", "error", err, "runtime", runtime)
		return
	}
	if !expected && lastError != "" {
		b.logger.Warn("browser exited unexpectedly", "error", lastError, "runtime", runtime)
		return
	}
	b.logger.Info("browser exited", "runtime", runtime)
}

func (b *Browser) writeCrashDiagnostic(cmd *exec.Cmd, runtime time.Duration, lastError string) {
	cfg := b.cfg.Snapshot()
	dir := filepath.Join(config.ConfigDir(cfg.Path), "diagnostics", "browser-crashes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		b.logger.Warn("browser crash diagnostic directory failed", "error", err)
		return
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+".txt")
	var out strings.Builder
	out.WriteString("KioskMate browser crash diagnostic\n")
	out.WriteString("time: " + time.Now().Format(time.RFC3339) + "\n")
	out.WriteString("runtime: " + runtime.String() + "\n")
	out.WriteString("last_error: " + lastError + "\n")
	if cmd != nil {
		out.WriteString("path: " + cmd.Path + "\n")
		out.WriteString("args: " + strings.Join(cmd.Args, " ") + "\n")
	}
	out.WriteString("active_page: " + cfg.Kiosk.PageName(b.active) + "\n")
	out.WriteString("active_url: " + b.activeURL() + "\n")
	out.WriteString("profile: " + browserUserDataDir(cfg, b.active) + "\n")
	out.WriteString("performance_profile: " + cfg.Performance.Profile + "\n")
	out.WriteString("gpu_mode: " + cfg.Performance.GPUMode + "\n")
	out.WriteString("DISPLAY: " + os.Getenv("DISPLAY") + "\n")
	out.WriteString("WAYLAND_DISPLAY: " + os.Getenv("WAYLAND_DISPLAY") + "\n")
	out.WriteString("XDG_RUNTIME_DIR: " + os.Getenv("XDG_RUNTIME_DIR") + "\n")
	out.WriteString("XDG_SESSION_TYPE: " + os.Getenv("XDG_SESSION_TYPE") + "\n")
	out.WriteString("browser_log: " + config.BrowserLogFilePath(cfg.Path) + "\n")
	out.WriteString("core_log: " + config.LogFilePath(cfg.Path) + "\n")
	if err := os.WriteFile(path, []byte(out.String()), 0o600); err != nil {
		b.logger.Warn("browser crash diagnostic write failed", "error", err)
		return
	}
	b.logger.Warn("browser crash diagnostic written", "path", path)
	_ = logutil.PruneFiles(dir, 10)
}

func (b *Browser) watch(pid int, done <-chan struct{}) {
	cfg := b.cfg.Snapshot()
	if !cfg.Watchdog.Enabled {
		return
	}
	ticker := time.NewTicker(cfg.Watchdog.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			stats, err := system.ReadProcessTreeStats(pid, b.lastStat)
			if err != nil {
				return
			}
			b.mu.Lock()
			b.lastStat = stats
			restart, reason := b.shouldRestart(stats)
			if reason != "" {
				b.watchdog.LastReason = reason
			}
			if restart {
				now := time.Now()
				if !b.watchdogRestartAllowedLocked(now) {
					until := b.watchdog.SuppressedUntil
					b.mu.Unlock()
					b.logger.Warn("browser watchdog restart suppressed", "reason", reason, "suppressed_until", until)
					continue
				}
				b.watchdog.LastRestart = &now
				b.watchdog.LastReason = reason
				b.watchdog.LastAction = "restart"
			}
			b.mu.Unlock()
			if restart {
				current := b.cfg.Snapshot()
				b.logger.Warn("browser watchdog restart", "reason", reason, "rss_mb", stats.RSSMB, "rss_limit_mb", current.Watchdog.MaxRSSMB, "cpu", stats.CPUPercent, "cpu_limit", current.Watchdog.MaxCPUPercent)
				restartCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				_ = b.Restart(restartCtx)
				cancel()
				return
			}
		}
	}
}

func (b *Browser) shouldRestart(stats system.ProcessTreeStats) (bool, string) {
	cfg := b.cfg.Snapshot()
	overRSS := cfg.Watchdog.MaxRSSMB > 0 && stats.RSSMB > cfg.Watchdog.MaxRSSMB
	overCPU := cfg.Watchdog.MaxCPUPercent > 0 && stats.CPUPercent > cfg.Watchdog.MaxCPUPercent
	if !overRSS && !overCPU {
		b.hotSince = time.Time{}
		b.watchdog.Pressure = "normal"
		return false, ""
	}
	reason := b.watchdogReason(stats, overRSS, overCPU)
	b.watchdog.Pressure = reason
	if b.hotSince.IsZero() {
		b.hotSince = time.Now()
		return false, reason
	}
	grace := cfg.Watchdog.CPUGrace
	if overCPU && !overRSS && grace < watchdogMinCPUOnlyGrace {
		grace = watchdogMinCPUOnlyGrace
	}
	return time.Since(b.hotSince) >= grace, reason
}

func (b *Browser) watchdogRestartAllowedLocked(now time.Time) bool {
	if b.watchdog.SuppressedUntil != nil && now.Before(*b.watchdog.SuppressedUntil) {
		b.watchdog.LastAction = "restart suppressed"
		return false
	}
	cutoff := now.Add(-watchdogRestartWindow)
	var recent []time.Time
	for _, item := range b.watchdogRuns {
		if item.After(cutoff) {
			recent = append(recent, item)
		}
	}
	b.watchdogRuns = recent
	b.watchdog.RestartWindowCount = len(recent)
	if len(recent) >= watchdogMaxRestartsInWindow {
		until := now.Add(watchdogSuppressDuration)
		b.watchdog.SuppressedUntil = &until
		b.watchdog.LastAction = "restart suppressed"
		return false
	}
	b.watchdogRuns = append(b.watchdogRuns, now)
	b.watchdog.RestartWindowCount = len(b.watchdogRuns)
	b.watchdog.SuppressedUntil = nil
	return true
}

func (b *Browser) watchdogReason(stats system.ProcessTreeStats, overRSS bool, overCPU bool) string {
	cfg := b.cfg.Snapshot()
	var reasons []string
	if overRSS {
		reasons = append(reasons, fmt.Sprintf("rss %dMB > %dMB", stats.RSSMB, cfg.Watchdog.MaxRSSMB))
	}
	if overCPU {
		reasons = append(reasons, fmt.Sprintf("cpu %.1f%% > %.1f%%", stats.CPUPercent, cfg.Watchdog.MaxCPUPercent))
	}
	return strings.Join(reasons, ", ")
}

func (b *Browser) watchdogStatusLocked() WatchdogStatus {
	cfg := b.cfg.Snapshot()
	status := b.watchdog
	status.Enabled = cfg.Watchdog.Enabled
	status.MaxRSSMB = cfg.Watchdog.MaxRSSMB
	status.MaxCPUPercent = cfg.Watchdog.MaxCPUPercent
	status.CheckInterval = int(cfg.Watchdog.CheckInterval / time.Second)
	status.CPUGrace = int(cfg.Watchdog.CPUGrace / time.Second)
	if !b.hotSince.IsZero() {
		hotSince := b.hotSince
		status.HotSince = &hotSince
	}
	status.RestartWindowCount = len(b.watchdogRuns)
	if status.Pressure == "" {
		status.Pressure = "normal"
	}
	return status
}

func (b *Browser) command() (string, []string, error) {
	cfg := b.cfg.Snapshot()
	preset := browserPreset(cfg.Kiosk.BrowserPreset)
	command := cfg.Kiosk.BrowserCommand
	if command == "" || preset != "custom" {
		command = findBrowser(preset)
	}
	if command == "" {
		return "", nil, fmt.Errorf("no browser found for preset %q, install a supported browser or use custom command", preset)
	}
	args := browserArgs(cfg, preset, activeURL(cfg, b.active), cfg.Kiosk.ExtraArgs, b.active)
	return command, args, nil
}

func (b *Browser) activeURL() string {
	return activeURL(b.cfg.Snapshot(), b.active)
}

func activeURL(cfg *config.Config, active int) string {
	urls := cfg.Kiosk.PageURLs()
	if len(urls) == 0 {
		return "about:blank"
	}
	if active < 0 || active >= len(urls) {
		active = 0
	}
	return urls[active]
}

func (b *Browser) schedulerTarget(now time.Time) (int, SchedulerStatus) {
	cfg := b.cfg.Snapshot().Kiosk
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

func browserArgs(cfg *config.Config, preset string, url string, extra []string, page int) []string {
	switch preset {
	case "firefox":
		return append(firefoxArgs(cfg, page), append(extra, url)...)
	case "webkit-cog":
		return append(extra, url)
	case "epiphany":
		return append([]string{"--application-mode"}, append(extra, url)...)
	case "midori":
		return append([]string{"-e", "Fullscreen"}, append(extra, url)...)
	case "custom":
		return append(extra, url)
	case "chromium-lite":
		args := append(chromiumArgs(cfg, page), chromiumLiteArgs()...)
		return append(args, append(extra, url)...)
	default:
		return append(chromiumArgs(cfg, page), append(extra, url)...)
	}
}

func chromiumArgs(cfg *config.Config, page int) []string {
	args := []string{
		"--kiosk",
		"--user-data-dir=" + browserUserDataDir(cfg, page),
		"--no-first-run",
		"--disable-translate",
		"--disable-session-crashed-bubble",
		"--disable-infobars",
		"--autoplay-policy=user-gesture-required",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-features=TranslateUI,MediaRouter,OptimizationHints,LocalNetworkAccessChecks,BlockInsecurePrivateNetworkRequests",
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=0",
	}
	if cfg.Performance.Profile == "raspberry" || cfg.Performance.Profile == "minimal" || cfg.Performance.ReduceMotion {
		args = append(args, "--disable-smooth-scrolling")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Kiosk.Theme)) {
	case "dark":
		args = append(args, "--force-prefers-color-scheme=dark", "--enable-features=WebUIDarkMode")
	case "force-dark":
		args = append(args, "--force-dark-mode", "--enable-features=WebContentsForceDark")
	}
	args = append(args, performanceArgs(cfg.Performance.Profile)...)
	if cfg.Kiosk.ZoomPercent > 0 && cfg.Kiosk.ZoomPercent != 100 {
		args = append(args, fmt.Sprintf("--force-device-scale-factor=%.2f", float64(cfg.Kiosk.ZoomPercent)/100.0))
	}
	if cfg.Performance.GPUMode == "software" {
		args = append(args, "--disable-gpu", "--disable-gpu-compositing")
	}
	return args
}

func supportsCDP(preset string) bool {
	return preset == "chromium" || preset == "chromium-lite"
}

func backupAndResetSession(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("browser profile path is empty")
	}
	backupRoot := filepath.Join(dir, "SessionBackups", time.Now().Format("20060102-150405"))
	paths := []string{
		"Default/Local Storage",
		"Default/IndexedDB",
		"Default/Session Storage",
		"Default/Service Worker",
		"Default/WebStorage",
		"Default/Network/Cookies",
		"Default/Network/Cookies-journal",
		"Default/Cookies",
		"Default/Cookies-journal",
		"Default/Cache",
		"Default/Code Cache",
		"Default/GPUCache",
	}
	moved := false
	for _, name := range paths {
		source := filepath.Join(dir, filepath.FromSlash(name))
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		target := filepath.Join(backupRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.Rename(source, target); err != nil {
			return fmt.Errorf("back up browser session %s: %w", name, err)
		}
		moved = true
	}
	for _, name := range []string{"SingletonCookie", "SingletonLock", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	if !moved {
		_ = os.RemoveAll(backupRoot)
	}
	pruneSessionBackups(filepath.Join(dir, "SessionBackups"), 2)
	return nil
}

func pruneSessionBackups(root string, keep int) {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) <= keep {
		return
	}
	for _, entry := range entries[:len(entries)-keep] {
		if entry.IsDir() {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func (b *Browser) tripAuthGuard(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Home Assistant rejected browser authentication"
	}
	b.mu.Lock()
	if b.authGuard.Tripped {
		b.mu.Unlock()
		return
	}
	now := time.Now()
	b.authGuard = AuthGuardStatus{Tripped: true, Reason: reason, At: &now}
	b.lastError = "authentication guard: " + reason
	b.mu.Unlock()
	b.persistAuthGuard()
	b.logger.Error("Home Assistant authentication guard tripped", "reason", reason)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = b.Stop(ctx)
	}()
}

func (b *Browser) clearAuthGuard() {
	b.mu.Lock()
	b.authGuard = AuthGuardStatus{}
	b.mu.Unlock()
	if b.cfg.Path != "" {
		_ = os.Remove(b.authGuardPath())
	}
}

func (b *Browser) authGuardPath() string {
	return filepath.Join(config.ConfigDir(b.cfg.Path), "auth-guard.json")
}

func (b *Browser) persistAuthGuard() {
	if b.cfg.Path == "" {
		return
	}
	b.mu.Lock()
	data, _ := json.MarshalIndent(b.authGuard, "", "  ")
	b.mu.Unlock()
	_ = os.WriteFile(b.authGuardPath(), append(data, '\n'), 0o600)
}

func (b *Browser) loadAuthGuard() {
	if b.cfg.Path == "" {
		return
	}
	data, err := os.ReadFile(b.authGuardPath())
	if err == nil {
		_ = json.Unmarshal(data, &b.authGuard)
	}
}

func performanceArgs(profile string) []string {
	switch strings.TrimSpace(strings.ToLower(profile)) {
	case "minimal", "conservative":
		return []string{
			"--disable-dev-shm-usage",
			"--disable-extensions",
			"--disable-sync",
			"--disable-print-preview",
			"--disable-speech-api",
			"--disable-notifications",
			"--disable-background-timer-throttling",
			"--disable-renderer-backgrounding",
			"--renderer-process-limit=1",
			"--process-per-site",
		}
	case "raspberry", "low-power":
		return raspberryLowPowerArgs()
	case "quality":
		return nil
	default:
		return []string{
			"--disable-sync",
			"--disable-print-preview",
			"--disable-speech-api",
		}
	}
}

func raspberryLowPowerArgs() []string {
	return []string{
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--disable-sync",
		"--disable-print-preview",
		"--disable-speech-api",
		"--disable-notifications",
		"--renderer-process-limit=1",
		"--process-per-site",
		"--num-raster-threads=1",
	}
}

func chromiumLiteArgs() []string {
	return append(raspberryLowPowerArgs(),
		"--disable-background-timer-throttling",
		"--disable-renderer-backgrounding",
	)
}

func browserUserDataDir(cfg *config.Config, page int) string {
	base := cfg.Kiosk.UserDataDir
	if base == "" || !cfg.Kiosk.IsolateSessions {
		return base
	}
	name := cfg.Kiosk.PageName(page)
	if name == "" {
		name = fmt.Sprintf("page-%d", page+1)
	}
	return filepath.Join(base, "pages", pathSegment(name, page))
}

func pathSegment(value string, page int) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	text := strings.Trim(b.String(), "-")
	if text == "" {
		text = fmt.Sprintf("page-%d", page+1)
	}
	return fmt.Sprintf("%02d-%s", page+1, text)
}

func firefoxArgs(cfg *config.Config, page int) []string {
	args := []string{"--kiosk"}
	if cfg.Kiosk.UserDataDir != "" {
		args = append(args, "--profile", browserUserDataDir(cfg, page))
	}
	return args
}
