package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MickLesk/KioskMate/internal/workflow"
)

// WorkflowEngine is the workflow rule runner used by the rule endpoints.
type WorkflowEngine interface {
	HandleWebhook(ctx context.Context, id, token string) error
	Test(ctx context.Context, id string) error
	History() []workflow.Run
}

// SetWorkflow attaches the workflow engine.
func (s *Server) SetWorkflow(engine WorkflowEngine) {
	s.workflow = engine
}

// ruleWebhook serves POST /api/rules/<id>/trigger with the rule token in the
// "token" query parameter or the X-KioskMate-Token header. It needs no Admin
// session, so failures count against the same throttle as logins.
func (s *Server) ruleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id, ok := ruleIDFromPath(r.URL.Path, "/trigger")
	if !ok || s.workflow == nil {
		http.NotFound(w, r)
		return
	}
	if allowed, retry := s.allowAttempt(r); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "too many failed attempts", "retry_after_seconds": int(retry.Seconds())})
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if header := strings.TrimSpace(r.Header.Get("X-KioskMate-Token")); header != "" {
		token = header
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	err := s.workflow.HandleWebhook(ctx, id, token)
	if errors.Is(err, workflow.ErrUnknownRule) {
		s.recordFailedAttempt(r)
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ruleAction serves POST /api/rules/<id>/test and GET /api/rules/history for
// signed-in Admin users.
func (s *Server) ruleAction(w http.ResponseWriter, r *http.Request) {
	if s.workflow == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "workflow engine unavailable"})
		return
	}
	if r.URL.Path == "/api/rules/history" {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": s.workflow.History()})
		return
	}
	id, ok := ruleIDFromPath(r.URL.Path, "/test")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.workflow.Test(ctx, id); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, workflow.ErrUnknownRule) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func ruleIDFromPath(path, suffix string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/api/rules/")
	if !ok {
		return "", false
	}
	id, ok := strings.CutSuffix(rest, suffix)
	if !ok || id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}
