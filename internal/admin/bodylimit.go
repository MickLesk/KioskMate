package admin

import (
	"net/http"
	"strings"
)

// Request bodies are capped so a broken or hostile client cannot make the
// kiosk read hundreds of MB into memory. Endpoints reachable before login
// (login, setup, webhooks) get a small limit, the rest one large enough for
// a configuration with local HTML pages.
const (
	publicBodyLimit = 64 << 10
	apiBodyLimit    = 8 << 20
)

func bodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			if limit := requestBodyLimit(r.URL.Path); limit > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requestBodyLimit(path string) int64 {
	switch {
	case strings.HasPrefix(path, "/api/auth/login"), strings.HasPrefix(path, "/api/auth/setup"),
		strings.HasPrefix(path, "/api/rules/") && strings.HasSuffix(path, "/trigger"):
		return publicBodyLimit
	case strings.HasPrefix(path, "/api/"), strings.HasPrefix(path, "/local/"):
		return apiBodyLimit
	}
	return publicBodyLimit
}
