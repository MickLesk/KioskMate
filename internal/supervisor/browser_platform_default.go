//go:build !windows

package supervisor

// installedBrowser has no install folders to search outside Windows: Linux
// browsers are found on PATH.
func installedBrowser(string) string {
	return ""
}

func platformBrowserArgs(_ string, args []string) []string {
	return args
}
