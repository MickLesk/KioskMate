package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBodyLimit(t *testing.T) {
	var read int
	var readErr error
	handler := bodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		read, readErr = len(data), err
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(strings.Repeat("x", publicBodyLimit+10)))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if readErr == nil || read > publicBodyLimit {
		t.Fatalf("login body not limited: read %d, err %v", read, readErr)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(strings.Repeat("x", publicBodyLimit+10)))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if readErr != nil || read != publicBodyLimit+10 {
		t.Fatalf("config body limited too early: read %d, err %v", read, readErr)
	}
	if requestBodyLimit("/api/rules/abc/trigger") != publicBodyLimit || requestBodyLimit("/api/rules/abc") != apiBodyLimit {
		t.Fatal("webhook limit")
	}
}
