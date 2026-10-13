package supervisor

import (
	"os"
	"strings"
)

// The helpers in this file describe Windows browser discovery and argument
// rules. They carry no build tag so the rules are unit-tested on Linux; the
// platform files decide whether they are used.

// windowsBrowserPaths lists the usual per-machine and per-user install
// locations of Microsoft Edge and Google Chrome (or Firefox for its preset),
// Edge first because it ships with Windows 10 and 11.
func windowsBrowserPaths(preset string, getenv func(string) string) []string {
	programFiles := envOr(getenv, "ProgramFiles", `C:\Program Files`)
	programFilesX86 := envOr(getenv, "ProgramFiles(x86)", `C:\Program Files (x86)`)
	localAppData := envOr(getenv, "LOCALAPPDATA", "")
	roots := []string{programFilesX86, programFiles, localAppData}
	var products [][]string
	switch preset {
	case "chromium", "chromium-lite":
		products = [][]string{
			{"Microsoft", "Edge", "Application", "msedge.exe"},
			{"Google", "Chrome", "Application", "chrome.exe"},
		}
	case "firefox":
		roots = []string{programFiles, programFilesX86}
		products = [][]string{{"Mozilla Firefox", "firefox.exe"}}
	default:
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	for _, product := range products {
		for _, root := range roots {
			if root == "" {
				continue
			}
			path := windowsJoin(root, product...)
			if key := strings.ToLower(path); !seen[key] {
				seen[key] = true
				paths = append(paths, path)
			}
		}
	}
	return paths
}

// windowsJoin joins with backslashes regardless of the build platform.
func windowsJoin(root string, parts ...string) string {
	return strings.TrimRight(root, `\/`) + `\` + strings.Join(parts, `\`)
}

func envOr(getenv func(string) string, key string, fallback string) string {
	if value := strings.TrimSpace(getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstExistingPath(paths []string, exists func(string) bool) string {
	for _, path := range paths {
		if exists(path) {
			return path
		}
	}
	return ""
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// linuxOnlySwitches are Chromium switches that only mean something on Linux
// display stacks (Ozone, Wayland). Edge and Chrome on Windows ignore most of
// them at best, so they are dropped there instead of being passed along.
var linuxOnlySwitches = []string{
	"--ozone-platform",
	"--ozone-platform-hint",
	"--enable-wayland-ime",
	"--wayland-text-input-version",
}

var linuxOnlyFeatures = map[string]bool{
	"UseOzonePlatform":              true,
	"WaylandWindowDecorations":      true,
	"WaylandFractionalScaleV1":      true,
	"WaylandPerSurfaceScale":        true,
	"WaylandTextInputV3":            true,
	"WaylandUiScale":                true,
	"WaylandLinuxDrmSyncobj":        true,
	"VaapiVideoDecoder":             true,
	"VaapiVideoDecodeLinuxGL":       true,
	"VaapiVideoEncoder":             true,
	"AcceleratedVideoDecodeLinuxGL": true,
}

// withoutLinuxOnlyArgs removes Linux-only switches and Linux-only entries of
// --enable-features/--disable-features lists. A feature switch whose list
// becomes empty is dropped completely.
func withoutLinuxOnlyArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if isLinuxOnlySwitch(arg) {
			continue
		}
		if prefix, list, ok := featureSwitch(arg); ok {
			var kept []string
			for _, feature := range strings.Split(list, ",") {
				name := strings.TrimSpace(feature)
				if name == "" || linuxOnlyFeatures[strings.SplitN(name, "<", 2)[0]] {
					continue
				}
				kept = append(kept, name)
			}
			if len(kept) == 0 {
				continue
			}
			arg = prefix + strings.Join(kept, ",")
		}
		out = append(out, arg)
	}
	return out
}

func isLinuxOnlySwitch(arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	for _, item := range linuxOnlySwitches {
		if name == item {
			return true
		}
	}
	return false
}

func featureSwitch(arg string) (prefix string, list string, ok bool) {
	for _, prefix := range []string{"--enable-features=", "--disable-features="} {
		if strings.HasPrefix(arg, prefix) {
			return prefix, strings.TrimPrefix(arg, prefix), true
		}
	}
	return "", "", false
}
