package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/system"
)

func TestBrowserProcessOutlivesStartRequestContext(t *testing.T) {
	if os.Getenv("KIOSKMATE_BROWSER_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	cfg := schedulerTestConfig()
	cfg.Path = filepath.Join(t.TempDir(), "config.json")
	cfg.Kiosk.BrowserPreset = "custom"
	cfg.Kiosk.BrowserCommand = os.Args[0]
	cfg.Kiosk.ExtraArgs = []string{"-test.run=TestBrowserProcessOutlivesStartRequestContext"}
	t.Setenv("KIOSKMATE_BROWSER_HELPER", "1")
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	requestCtx, cancel := context.WithCancel(context.Background())
	if err := browser.Start(requestCtx); err != nil {
		t.Fatal(err)
	}
	cancel()
	time.Sleep(250 * time.Millisecond)
	if !browser.Status().Running {
		t.Fatalf("browser was killed with request context: %#v", browser.Status())
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := browser.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestBackupAndResetSessionMovesCurrentChromiumStorage(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Default/Local Storage/leveldb/data", "Default/Network/Cookies", "Default/WebStorage/data"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("session"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := backupAndResetSession(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Default", "Local Storage")); !os.IsNotExist(err) {
		t.Fatalf("local storage still exists: %v", err)
	}
	backups, err := os.ReadDir(filepath.Join(dir, "SessionBackups"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("session backup missing: %v %#v", err, backups)
	}
}

func TestAuthGuardBlocksBrowserStartUntilSessionReset(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Path = filepath.Join(t.TempDir(), "config.json")
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	browser.TripAuthGuard("invalid access token")
	if err := browser.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "authentication guard") {
		t.Fatalf("start error = %v", err)
	}
}

func TestSchedulerRotationTargetsConfiguredDurations(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.Scheduler = config.KioskScheduler{Enabled: true, Mode: "rotation"}
	cfg.Kiosk.Rotation = []config.RotationItem{
		{Page: 0, DurationSeconds: 60},
		{Page: 1, DurationSeconds: 120},
	}
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 7, 7, 13, 0, 0, 0, time.Local)

	page, status := browser.schedulerTarget(now)
	if page != 0 || status.Reason != "rotation" || status.NextSwitch == nil || status.NextSwitch.Sub(now) != time.Minute {
		t.Fatalf("first rotation = page %d, status %#v", page, status)
	}

	page, status = browser.schedulerTarget(now.Add(61 * time.Second))
	if page != 1 || status.RotationIndex != 1 || status.NextSwitch == nil || status.NextSwitch.Sub(now.Add(61*time.Second)) != 2*time.Minute {
		t.Fatalf("second rotation = page %d, status %#v", page, status)
	}
}

func TestSchedulerTimeRuleOverridesHybridRotation(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.Scheduler = config.KioskScheduler{Enabled: true, Mode: "hybrid"}
	cfg.Kiosk.Rotation = []config.RotationItem{{Page: 0, DurationSeconds: 60}}
	cfg.Kiosk.TimeRules = []config.TimeRule{{
		Name:  "Lunch Dashboard",
		Page:  1,
		Start: "13:00",
		End:   "14:00",
		Days:  []string{"tue"},
	}}
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	page, status := browser.schedulerTarget(time.Date(2026, 7, 7, 13, 30, 0, 0, time.Local))
	if page != 1 || status.Reason != "time" || status.ActiveRule != "Lunch Dashboard" {
		t.Fatalf("hybrid time rule = page %d, status %#v", page, status)
	}
}

func TestSchedulerOvernightRule(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.Scheduler = config.KioskScheduler{Enabled: true, Mode: "time"}
	cfg.Kiosk.TimeRules = []config.TimeRule{{
		Name:  "Night",
		Page:  1,
		Start: "22:00",
		End:   "06:00",
	}}
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, hour := range []int{23, 2} {
		page, status := browser.schedulerTarget(time.Date(2026, 7, 7, hour, 10, 0, 0, time.Local))
		if page != 1 || status.Reason != "time" {
			t.Fatalf("overnight hour %d = page %d, status %#v", hour, page, status)
		}
	}
}

func TestExpectedBrowserStopDoesNotRecordLastError(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.BrowserPreset = "custom"
	cfg.Kiosk.BrowserCommand = "go"
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd := exec.Command("go", "env", "-badflag")
	if err := cmd.Start(); err != nil {
		t.Skipf("go command unavailable: %v", err)
	}
	browser.mu.Lock()
	browser.cmd = cmd
	browser.stopping = true
	browser.mu.Unlock()

	browser.wait(cmd, nil)

	status := browser.Status()
	if status.LastError != "" {
		t.Fatalf("expected no last error after intentional stop, got %q", status.LastError)
	}
	if status.LastExit == nil {
		t.Fatal("expected last exit timestamp")
	}
}

func TestUnexpectedBrowserExitRecordsLastError(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.BrowserPreset = "custom"
	cfg.Kiosk.BrowserCommand = "go"
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd := exec.Command("go", "env", "-badflag")
	if err := cmd.Start(); err != nil {
		t.Skipf("go command unavailable: %v", err)
	}
	browser.mu.Lock()
	browser.cmd = cmd
	browser.mu.Unlock()

	browser.wait(cmd, nil)

	status := browser.Status()
	if status.LastError == "" {
		t.Fatal("expected last error after unexpected exit")
	}
}

func TestQuickSuccessfulBrowserExitRecordsLastError(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.BrowserPreset = "custom"
	cfg.Kiosk.BrowserCommand = "go"
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd := exec.Command("go", "version")
	if err := cmd.Start(); err != nil {
		t.Skipf("go command unavailable: %v", err)
	}
	browser.mu.Lock()
	browser.cmd = cmd
	browser.started = time.Now()
	browser.mu.Unlock()

	browser.wait(cmd, nil)

	status := browser.Status()
	if status.LastError == "" || !strings.Contains(status.LastError, "without error") {
		t.Fatalf("expected quick successful exit to be visible, got %q", status.LastError)
	}
}

func TestBrowserPresetArgs(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.UserDataDir = t.TempDir()
	cfg.Kiosk.ExtraArgs = []string{"--flag"}

	chromium := browserArgs(cfg, "chromium-lite", "http://ha.local", cfg.Kiosk.ExtraArgs, 0)
	if !contains(chromium, "--renderer-process-limit=1") || !contains(chromium, "--num-raster-threads=1") || !contains(chromium, "--flag") || chromium[len(chromium)-1] != "http://ha.local" {
		t.Fatalf("chromium-lite args = %#v", chromium)
	}
	if !containsPrefix(chromium, "--disable-features=TranslateUI,MediaRouter,OptimizationHints,LocalNetworkAccessChecks,BlockInsecurePrivateNetworkRequests") {
		t.Fatalf("chromium-lite args missing local network feature disables: %#v", chromium)
	}

	firefox := browserArgs(cfg, "firefox", "http://ha.local", cfg.Kiosk.ExtraArgs, 0)
	if contains(firefox, "--disable-gpu") || !contains(firefox, "--kiosk") || firefox[len(firefox)-1] != "http://ha.local" {
		t.Fatalf("firefox args = %#v", firefox)
	}

	cog := browserArgs(cfg, "webkit-cog", "http://ha.local", nil, 0)
	if len(cog) != 1 || cog[0] != "http://ha.local" {
		t.Fatalf("cog args = %#v", cog)
	}
}

func TestBrowserIsolatedPageSessionArgs(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.UserDataDir = t.TempDir()
	cfg.Kiosk.IsolateSessions = true

	main := browserArgs(cfg, "chromium", "http://ha.local/main", nil, 0)
	calendar := browserArgs(cfg, "chromium", "http://ha.local/calendar", nil, 1)

	if !containsPrefix(main, "--user-data-dir="+cfg.Kiosk.UserDataDir) || !containsPrefix(calendar, "--user-data-dir="+cfg.Kiosk.UserDataDir) {
		t.Fatalf("isolated args missing base dir: %#v %#v", main, calendar)
	}
	if main[1] == calendar[1] {
		t.Fatalf("expected different user-data dirs, got %#v", main[1])
	}
}

func TestPerformanceProfileArgs(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.UserDataDir = t.TempDir()
	cfg.Performance.Profile = "minimal"

	args := browserArgs(cfg, "chromium", "http://ha.local/main", nil, 0)
	if !contains(args, "--renderer-process-limit=1") || !contains(args, "--disable-extensions") {
		t.Fatalf("minimal profile args = %#v", args)
	}

	cfg.Performance.Profile = "quality"
	args = browserArgs(cfg, "chromium", "http://ha.local/main", nil, 0)
	if contains(args, "--renderer-process-limit=1") || contains(args, "--renderer-process-limit=2") {
		t.Fatalf("quality profile should not limit renderers: %#v", args)
	}

	cfg.Performance.Profile = "raspberry"
	args = browserArgs(cfg, "chromium", "http://ha.local/main", nil, 0)
	for _, want := range []string{"--renderer-process-limit=1", "--num-raster-threads=1", "--disable-dev-shm-usage"} {
		if !contains(args, want) {
			t.Fatalf("raspberry profile missing %s in %#v", want, args)
		}
	}
	for _, risky := range []string{"--enable-low-end-device-mode", "--disable-gpu-rasterization", "--disable-zero-copy"} {
		if contains(args, risky) {
			t.Fatalf("raspberry profile should not include risky startup flag %s in %#v", risky, args)
		}
	}
}

func TestKioskForceDarkThemeForcesChromiumDarkMode(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.UserDataDir = t.TempDir()
	cfg.Kiosk.Theme = "force-dark"

	args := browserArgs(cfg, "chromium", "http://ha.local/main", nil, 0)
	if !contains(args, "--force-dark-mode") || !contains(args, "--enable-features=WebContentsForceDark") {
		t.Fatalf("force-dark theme args = %#v", args)
	}

	cfg.Kiosk.Theme = "dark"
	args = browserArgs(cfg, "chromium", "http://ha.local/main", nil, 0)
	if contains(args, "--force-dark-mode") {
		t.Fatalf("native dark theme should not force Chromium dark mode: %#v", args)
	}
	if !contains(args, "--force-prefers-color-scheme=dark") {
		t.Fatalf("native dark theme should request dark color scheme: %#v", args)
	}
}

type recordingCDPCommander struct {
	calls []struct {
		method string
		params any
	}
}

func (r *recordingCDPCommander) command(_ context.Context, method string, params any, _ any) error {
	r.calls = append(r.calls, struct {
		method string
		params any
	}{method: method, params: params})
	return nil
}

func TestConfigureHomeAssistantThemeEmulatesNativeColorScheme(t *testing.T) {
	for _, test := range []struct {
		theme  string
		scheme string
	}{
		{theme: "dark", scheme: "dark"},
		{theme: "light", scheme: "light"},
	} {
		recorder := &recordingCDPCommander{}
		if err := configureHomeAssistantTheme(context.Background(), recorder, test.theme); err != nil {
			t.Fatalf("theme %q: %v", test.theme, err)
		}
		if len(recorder.calls) != 3 {
			t.Fatalf("theme %q sent %d commands, want 3", test.theme, len(recorder.calls))
		}
		if recorder.calls[0].method != "Emulation.setEmulatedMedia" {
			t.Fatalf("theme %q first command = %q", test.theme, recorder.calls[0].method)
		}
		encoded, err := json.Marshal(recorder.calls[0].params)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"features":[{"name":"prefers-color-scheme","value":"` + test.scheme + `"}]}`
		if string(encoded) != want {
			t.Fatalf("theme %q media params = %s, want %s", test.theme, encoded, want)
		}
	}
}

func TestHomeAssistantThemeScriptVerifiesEmulatedColorScheme(t *testing.T) {
	for _, test := range []struct {
		theme string
		dark  bool
	}{
		{theme: "dark", dark: true},
		{theme: "force-dark", dark: true},
		{theme: "light", dark: false},
	} {
		script, ok := homeAssistantThemeScript(test.theme)
		if !ok {
			t.Fatalf("theme %q did not produce a Home Assistant script", test.theme)
		}
		if !strings.Contains(script, fmt.Sprintf("const desiredDark = %t", test.dark)) {
			t.Fatalf("theme %q script has wrong mode: %s", test.theme, script)
		}
		if !strings.Contains(script, `matchMedia("(prefers-color-scheme: dark)").matches`) {
			t.Fatalf("theme %q script does not verify the emulated browser color scheme", test.theme)
		}
		if strings.Contains(script, `new CustomEvent("settheme"`) {
			t.Fatalf("theme %q script must preserve Home Assistant's selected custom theme", test.theme)
		}
	}
	if script, ok := homeAssistantThemeScript("invalid"); ok || script != "" {
		t.Fatalf("invalid theme produced script %q", script)
	}
}

func TestParseThemeConsoleEvent(t *testing.T) {
	dark := true
	payload, err := json.Marshal(map[string]any{
		"args": []map[string]any{{
			"value": themeStatusConsolePrefix + `{"ok":true,"requested_theme":"preserve-current","requested_dark":true,"selected_theme":"material_you","selected_dark":true,"applied_dark":true}`,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, ok := parseThemeConsoleEvent(payload)
	if !ok || !report.OK || report.RequestedTheme != "preserve-current" || report.SelectedTheme != "material_you" || report.SelectedDark == nil || *report.SelectedDark != dark || report.AppliedDark == nil || !*report.AppliedDark {
		t.Fatalf("unexpected theme report: %#v, ok=%v", report, ok)
	}
}

func TestThemeReportIsExposedInBrowserStatus(t *testing.T) {
	cfg := schedulerTestConfig()
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	dark := true
	browser.setThemeReport("dark", themeReport{
		OK:             true,
		RequestedTheme: "preserve-current",
		RequestedDark:  true,
		SelectedTheme:  "material_you",
		SelectedDark:   &dark,
		AppliedDark:    &dark,
	})
	status := browser.Status().Theme
	if status.State != "applied" || status.SelectedTheme != "material_you" || status.AppliedDark == nil || !*status.AppliedDark {
		t.Fatalf("unexpected browser theme status: %#v", status)
	}
}

func TestCPUOnlyWatchdogUsesMinimumGrace(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Watchdog.MaxCPUPercent = 100
	cfg.Watchdog.CPUGrace = 45 * time.Second
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	browser.hotSince = time.Now().Add(-2 * time.Minute)

	restart, reason := browser.shouldRestart(processStats(0, 250))
	if restart {
		t.Fatalf("cpu-only pressure restarted too early: %s", reason)
	}

	browser.hotSince = time.Now().Add(-11 * time.Minute)
	restart, reason = browser.shouldRestart(processStats(0, 250))
	if !restart {
		t.Fatalf("expected cpu-only restart after minimum grace, reason %q", reason)
	}
}

func TestWatchdogRestartRateLimit(t *testing.T) {
	cfg := schedulerTestConfig()
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now()

	for i := 0; i < watchdogMaxRestartsInWindow; i++ {
		if !browser.watchdogRestartAllowedLocked(now.Add(time.Duration(i) * time.Minute)) {
			t.Fatalf("restart %d was unexpectedly suppressed", i+1)
		}
	}
	if browser.watchdogRestartAllowedLocked(now.Add(4 * time.Minute)) {
		t.Fatal("expected restart loop to be suppressed")
	}
	if browser.watchdog.SuppressedUntil == nil {
		t.Fatal("expected suppressed_until to be set")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsPrefix(values []string, want string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, want) {
			return true
		}
	}
	return false
}

func processStats(rss uint64, cpu float64) system.ProcessTreeStats {
	return system.ProcessTreeStats{RSSMB: rss, CPUPercent: cpu}
}

func schedulerTestConfig() *config.Config {
	return &config.Config{
		Kiosk: config.KioskConfig{
			Pages: []config.KioskPage{
				{Name: "Main", URL: "http://ha.local/main"},
				{Name: "Calendar", URL: "http://ha.local/calendar"},
			},
		},
	}
}
