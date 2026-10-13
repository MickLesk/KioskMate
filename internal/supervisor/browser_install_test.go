package supervisor

import (
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func windowsEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestWindowsBrowserPathsPreferEdgeThenChrome(t *testing.T) {
	env := windowsEnv(map[string]string{
		"ProgramFiles":      `C:\Program Files`,
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
		"LOCALAPPDATA":      `C:\Users\kiosk\AppData\Local`,
	})
	got := windowsBrowserPaths("chromium", env)
	want := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Users\kiosk\AppData\Local\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Users\kiosk\AppData\Local\Google\Chrome\Application\chrome.exe`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v\nwant %#v", got, want)
	}
	if lite := windowsBrowserPaths("chromium-lite", env); !reflect.DeepEqual(lite, want) {
		t.Fatalf("chromium-lite paths = %#v", lite)
	}
}

func TestWindowsBrowserPathsDefaultsWithoutEnvironment(t *testing.T) {
	got := windowsBrowserPaths("chromium", windowsEnv(nil))
	want := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v\nwant %#v", got, want)
	}
}

func TestWindowsBrowserPathsDeduplicateProgramFiles(t *testing.T) {
	env := windowsEnv(map[string]string{"ProgramFiles(x86)": `D:\Apps\`, "ProgramFiles": `d:\apps`})
	got := windowsBrowserPaths("chromium", env)
	if len(got) != 2 || got[0] != `D:\Apps\Microsoft\Edge\Application\msedge.exe` {
		t.Fatalf("paths = %#v", got)
	}
}

func TestWindowsBrowserPathsOtherPresets(t *testing.T) {
	if got := windowsBrowserPaths("firefox", windowsEnv(nil)); len(got) != 2 || !strings.HasSuffix(got[0], `\Mozilla Firefox\firefox.exe`) {
		t.Fatalf("firefox paths = %#v", got)
	}
	for _, preset := range []string{"custom", "webkit-cog", "epiphany", "midori"} {
		if got := windowsBrowserPaths(preset, windowsEnv(nil)); got != nil {
			t.Fatalf("%s paths = %#v", preset, got)
		}
	}
}

func TestFirstExistingPathSelectsFirstInstalledBrowser(t *testing.T) {
	paths := windowsBrowserPaths("chromium", windowsEnv(map[string]string{"LOCALAPPDATA": `C:\Users\kiosk\AppData\Local`}))
	installed := map[string]bool{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`:              true,
		`C:\Users\kiosk\AppData\Local\Microsoft\Edge\Application\msedge.exe`: true,
	}
	var checked []string
	got := firstExistingPath(paths, func(path string) bool {
		checked = append(checked, path)
		return installed[path]
	})
	if got != `C:\Users\kiosk\AppData\Local\Microsoft\Edge\Application\msedge.exe` {
		t.Fatalf("selected %q", got)
	}
	if len(checked) != 3 {
		t.Fatalf("checked %d paths, want 3: %#v", len(checked), checked)
	}
	onlyChrome := func(path string) bool { return strings.HasSuffix(path, `\chrome.exe`) }
	if got := firstExistingPath(paths, onlyChrome); got != `C:\Program Files (x86)\Google\Chrome\Application\chrome.exe` {
		t.Fatalf("chrome fallback = %q", got)
	}
	if got := firstExistingPath(paths, func(string) bool { return false }); got != "" {
		t.Fatalf("no browser installed, got %q", got)
	}
}

func TestWithoutLinuxOnlyArgs(t *testing.T) {
	args := []string{
		"--kiosk",
		"--ozone-platform=wayland",
		"--ozone-platform-hint=auto",
		"--enable-wayland-ime",
		"--enable-features=UseOzonePlatform,WebUIDarkMode,WaylandWindowDecorations",
		"--disable-features=TranslateUI,VaapiVideoDecoder",
		"--ozone-platformish=keep",
		"https://example.test",
	}
	got := withoutLinuxOnlyArgs(args)
	want := []string{
		"--kiosk",
		"--enable-features=WebUIDarkMode",
		"--disable-features=TranslateUI",
		"--ozone-platformish=keep",
		"https://example.test",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v\nwant %#v", got, want)
	}
	if got := withoutLinuxOnlyArgs([]string{"--enable-features=UseOzonePlatform"}); len(got) != 0 {
		t.Fatalf("empty feature list kept: %#v", got)
	}
}

func TestWithoutLinuxOnlyArgsCleansChromiumDefaults(t *testing.T) {
	cfg := &config.Config{}
	cfg.Kiosk.UserDataDir = `C:\Users\kiosk\AppData\Local\KioskMate\Browser`
	cfg.Kiosk.Theme = "dark"
	args := withoutLinuxOnlyArgs(presetBrowserArgs(cfg, "chromium", "about:blank", []string{"--ozone-platform=x11"}, 0))
	for _, arg := range args {
		if strings.HasPrefix(arg, "--ozone-") {
			t.Fatalf("Linux-only switch left: %s in %#v", arg, args)
		}
	}
	for _, required := range []string{"--kiosk", "--remote-debugging-port=0", "--user-data-dir=" + cfg.Kiosk.UserDataDir, "about:blank"} {
		if !slices.Contains(args, required) {
			t.Fatalf("missing %s in %#v", required, args)
		}
	}
}

func TestBrowserArgsKeepOzoneOutsideWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux display switches are dropped on Windows")
	}
	cfg := &config.Config{}
	cfg.Kiosk.UserDataDir = "/tmp/profile"
	args := browserArgs(cfg, "chromium", "about:blank", nil, 0)
	if !slices.ContainsFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "--ozone-platform") }) {
		t.Fatalf("ozone switch missing on this platform: %#v", args)
	}
}
