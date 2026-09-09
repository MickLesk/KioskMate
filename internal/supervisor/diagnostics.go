package supervisor

import (
	"net/url"
	"strings"
)

// diagnosticURL preserves enough location information to identify a failing
// page while keeping credentials, query tokens and fragments out of logs and APIs.
func diagnosticURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return "<invalid-url>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func diagnosticArgs(args []string) []string {
	out := append([]string(nil), args...)
	for index, arg := range out {
		lower := strings.ToLower(strings.TrimSpace(arg))
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
			out[index] = diagnosticURL(arg)
		}
	}
	return out
}
