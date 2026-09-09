package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func checkHomeAssistantBan(ctx context.Context, target string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false, fmt.Errorf("invalid Home Assistant URL")
	}
	parsed.Path = "/manifest.json"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.User = nil
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "KioskMate Auth Safety Check")
	client := &http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusForbidden, nil
}

type authEvidence struct {
	Kind       string
	Confidence string
	URL        string
	Resource   string
	Status     int
	Reason     string
	Action     string
}

func canonicalOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "ws" {
		scheme = "http"
	}
	if scheme == "wss" {
		scheme = "https"
	}
	if scheme != "http" && scheme != "https" {
		return ""
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

func (b *Browser) isHAOrigin(raw string) bool {
	origin := canonicalOrigin(raw)
	if origin == "" {
		return false
	}
	cfg := b.cfg.Snapshot()
	for _, page := range cfg.Kiosk.Pages {
		if !page.Disabled && (page.SourceType == "home_assistant" || likelyHomeAssistantURL(page.URL)) && canonicalOrigin(page.URL) == origin {
			return true
		}
	}
	for _, target := range cfg.Kiosk.URLs {
		if likelyHomeAssistantURL(target) && canonicalOrigin(target) == origin {
			return true
		}
	}
	return false
}

func (b *Browser) isHAAuthResource(raw, path string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Path == path && b.isHAOrigin(raw)
}

func classifyHAResponse(status int, resource, raw string) authEvidence {
	u, err := url.Parse(raw)
	if err != nil {
		return authEvidence{}
	}
	e := authEvidence{URL: raw, Resource: resource, Status: status, Confidence: "suspected", Action: "inspect_ha"}
	switch {
	case u.Path == "/auth/token" && status == 400:
		e.Kind = "token_response"
	case (u.Path == "/auth/token" || u.Path == "/api/websocket") && (status == 401 || status == 403):
		e.Kind, e.Reason = "auth_rejected", "Home Assistant denied the authentication endpoint"
	case resource == "Document" && status == 403:
		e.Kind, e.Reason = "access_denied", "Home Assistant denied the page; verify IP ban or proxy policy"
	default:
		return authEvidence{}
	}
	return e
}

func (b *Browser) blockAuthentication(e authEvidence) {
	b.tripAuthGuard(e.Reason)
	b.mu.Lock()
	b.authGuard.Kind, b.authGuard.Confidence = e.Kind, e.Confidence
	b.authGuard.Origin = canonicalOrigin(e.URL)
	b.authGuard.Resource, b.authGuard.HTTPStatus = e.Resource, e.Status
	b.authGuard.NextAction = e.Action
	if ip := localIPForTarget(e.URL); ip != "" {
		b.authGuard.KioskIP = ip
	}
	b.mu.Unlock()
	b.persistAuthGuard()
}

func localIPForTarget(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" || u.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}
	conn, err := net.DialTimeout("udp", net.JoinHostPort(u.Hostname(), port), time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

func parseAuthFailure(payload string) bool {
	var frame struct {
		Type string `json:"type"`
	}
	return json.Unmarshal([]byte(payload), &frame) == nil && frame.Type == "auth_invalid"
}
