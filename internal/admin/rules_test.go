package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/workflow"
)

type fakeWorkflow struct {
	token  string
	fired  []string
	tested []string
}

func (f *fakeWorkflow) HandleWebhook(_ context.Context, id, token string) error {
	if id != "doorbell" || token != f.token {
		return workflow.ErrUnknownRule
	}
	f.fired = append(f.fired, id)
	return nil
}

func (f *fakeWorkflow) Test(_ context.Context, id string) error {
	if id != "doorbell" {
		return workflow.ErrUnknownRule
	}
	f.tested = append(f.tested, id)
	return nil
}

func (f *fakeWorkflow) History() []workflow.Run {
	return []workflow.Run{{RuleID: "doorbell", Source: "test"}}
}

func TestRuleWebhookChecksTokenAndCountsFailures(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, nil, nil, "test", nil)
	engine := &fakeWorkflow{token: "0123456789abcdef"}
	server.SetWorkflow(engine)

	post := func(target string, header string) int {
		req := httptest.NewRequest(http.MethodPost, target, nil)
		req.RemoteAddr = "192.0.2.20:1234"
		if header != "" {
			req.Header.Set("X-KioskMate-Token", header)
		}
		rec := httptest.NewRecorder()
		server.ruleWebhook(rec, req)
		return rec.Code
	}

	if code := post("/api/rules/doorbell/trigger?token=0123456789abcdef", ""); code != http.StatusOK {
		t.Fatalf("query token status = %d", code)
	}
	if code := post("/api/rules/doorbell/trigger", "0123456789abcdef"); code != http.StatusOK {
		t.Fatalf("header token status = %d", code)
	}
	if len(engine.fired) != 2 {
		t.Fatalf("fired = %v", engine.fired)
	}
	if code := post("/api/rules/doorbell/trigger?token=wrong", ""); code != http.StatusNotFound {
		t.Fatalf("wrong token status = %d, want 404", code)
	}
	if code := post("/api/rules/a/b/trigger", ""); code != http.StatusNotFound {
		t.Fatalf("nested id status = %d, want 404", code)
	}
	for range loginAttemptLimit {
		post("/api/rules/doorbell/trigger?token=wrong", "")
	}
	if code := post("/api/rules/doorbell/trigger?token=0123456789abcdef", ""); code != http.StatusTooManyRequests {
		t.Fatalf("throttled status = %d, want 429", code)
	}
}

func TestRuleActionRunsTestAndListsHistory(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, nil, nil, "test", nil)
	engine := &fakeWorkflow{}
	server.SetWorkflow(engine)

	rec := httptest.NewRecorder()
	server.ruleAction(rec, httptest.NewRequest(http.MethodPost, "/api/rules/doorbell/test", nil))
	if rec.Code != http.StatusOK || len(engine.tested) != 1 {
		t.Fatalf("test status = %d, tested = %v", rec.Code, engine.tested)
	}

	rec = httptest.NewRecorder()
	server.ruleAction(rec, httptest.NewRequest(http.MethodPost, "/api/rules/missing/test", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown rule status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	server.ruleAction(rec, httptest.NewRequest(http.MethodGet, "/api/rules/history", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"doorbell"`) {
		t.Fatalf("history status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
