//go:build windows

package supervisor

import "os"

// installedBrowser looks for Edge or Chrome in their install folders, which
// are not on PATH on a default Windows installation.
func installedBrowser(preset string) string {
	return firstExistingPath(windowsBrowserPaths(preset, os.Getenv), regularFileExists)
}

// platformBrowserArgs drops Linux display-stack switches (Ozone, Wayland)
// that the shared Chromium defaults or a configuration copied from a Linux
// kiosk may contain.
func platformBrowserArgs(preset string, args []string) []string {
	if !supportsCDP(preset) {
		return args
	}
	return withoutLinuxOnlyArgs(args)
}
