package supervisor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type devToolsTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type cdpCommander interface {
	command(context.Context, string, any, any) error
}

const themeStatusConsolePrefix = "__KIOSKMATE_THEME__"

type themeReport struct {
	OK             bool   `json:"ok"`
	Error          string `json:"error,omitempty"`
	RequestedTheme string `json:"requested_theme"`
	RequestedDark  bool   `json:"requested_dark"`
	SelectedTheme  string `json:"selected_theme,omitempty"`
	SelectedDark   *bool  `json:"selected_dark,omitempty"`
	AppliedDark    *bool  `json:"applied_dark,omitempty"`
}

func dialDevTools(ctx context.Context, profile string) (*cdpSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	target, err := waitForDevToolsTarget(ctx, profile)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.Dial(ctx, target.WebSocketDebuggerURL, &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("connect to Chromium DevTools: %w", err)
	}
	conn.SetReadLimit(16 << 20)
	session := newCDPSession(conn)
	session.target = target
	return session, nil
}

func captureDevToolsScreenshot(ctx context.Context, profile string) ([]byte, error) {
	session, err := dialDevTools(ctx, profile)
	if err != nil {
		return nil, err
	}
	defer session.close()
	if err := session.command(ctx, "Page.enable", map[string]any{}, nil); err != nil {
		return nil, err
	}
	var result struct {
		Data string `json:"data"`
	}
	if err := session.command(ctx, "Page.captureScreenshot", map[string]any{
		"format":                "png",
		"fromSurface":           true,
		"captureBeyondViewport": false,
	}, &result); err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		return nil, fmt.Errorf("decode DevTools screenshot: %w", err)
	}
	return data, nil
}

func (b *Browser) controlSession() (*cdpSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd == nil || b.cmd.Process == nil {
		return nil, errors.New("browser is not running")
	}
	if b.control == nil || !b.devTools {
		return nil, errors.New("browser control is connecting; retry when connected")
	}
	return b.control, nil
}

func (b *Browser) reloadDevTools(ctx context.Context) error {
	session, err := b.controlSession()
	if err != nil {
		return err
	}
	return session.command(ctx, "Page.reload", map[string]any{"ignoreCache": false}, nil)
}

func (b *Browser) navigateDevTools(ctx context.Context, target string) error {
	session, err := b.controlSession()
	if err != nil {
		return err
	}
	var result struct {
		ErrorText string `json:"errorText"`
	}
	if err := session.command(ctx, "Page.navigate", map[string]any{"url": target}, &result); err != nil {
		return err
	}
	if result.ErrorText != "" {
		return errors.New(result.ErrorText)
	}
	return nil
}

func (b *Browser) monitorDevTools(profile string, theme string, done <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-done:
			cancel()
		case <-ctx.Done():
		}
	}()
	navigated := false
	for attempt := 0; ctx.Err() == nil; attempt++ {
		session, err := dialDevTools(ctx, profile)
		if err == nil {
			err = b.runDevTools(ctx, session, theme, done, &navigated)
			session.close()
		}
		b.mu.Lock()
		if b.done == done {
			b.devTools = false
			b.control = nil
			if !b.authGuard.Tripped && err != nil {
				b.lastError = err.Error()
			}
		}
		b.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		b.mu.Lock()
		blocked := b.authGuard.Tripped
		b.mu.Unlock()
		if blocked {
			return
		}
		b.logger.Warn("Chromium control reconnect", "error", err)
		delay := time.Second * time.Duration(1<<minRuntimeInt(attempt, 4))
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (b *Browser) runDevTools(ctx context.Context, session *cdpSession, theme string, done <-chan struct{}, navigated *bool) error {
	if err := session.command(ctx, "Network.enable", map[string]any{}, nil); err != nil {
		return err
	}
	if err := session.command(ctx, "Page.enable", map[string]any{}, nil); err != nil {
		return err
	}
	if err := session.command(ctx, "Runtime.enable", map[string]any{}, nil); err != nil {
		return err
	}
	if err := configureHomeAssistantTheme(ctx, session, theme); err != nil {
		b.setThemeError(theme, err)
	}
	b.mu.Lock()
	if b.done != done {
		b.mu.Unlock()
		return context.Canceled
	}
	b.control, b.devTools = session, true
	b.mu.Unlock()
	if !*navigated {
		cfg := b.cfg.Snapshot()
		b.mu.Lock()
		target := activeURL(cfg, b.active)
		b.mu.Unlock()
		if err := b.authenticationAllowed(); err != nil {
			return err
		}
		var result struct {
			Error string `json:"errorText"`
		}
		if err := session.command(ctx, "Page.navigate", map[string]any{"url": target}, &result); err != nil {
			return err
		}
		if result.Error != "" {
			return fmt.Errorf("navigate to kiosk page: %s", result.Error)
		}
		*navigated = true
		b.mu.Lock()
		if b.done == done && !b.authGuard.Tripped {
			b.recovery.State, b.recovery.Stage = "healthy", "running"
			b.recovery.LastResult = "page control connected"
			b.lastError = ""
		}
		b.mu.Unlock()
	}
	sockets := map[string]string{}
	tokenFailures := map[string]authEvidence{}
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-session.done:
			return errors.New("Chromium control disconnected")
		case <-heartbeat.C:
			if err := session.command(ctx, "Page.getFrameTree", map[string]any{}, nil); err != nil {
				return err
			}
		case event := <-session.events:
			if event.Method == "Runtime.consoleAPICalled" {
				if report, ok := parseThemeConsoleEvent(event.Params); ok {
					b.setThemeReport(theme, report)
				}
				continue
			}
			var params struct {
				ID       string `json:"requestId"`
				URL      string `json:"url"`
				Type     string `json:"type"`
				Response struct {
					Status  int    `json:"status"`
					URL     string `json:"url"`
					Payload string `json:"payloadData"`
				} `json:"response"`
			}
			if json.Unmarshal(event.Params, &params) != nil {
				continue
			}
			switch event.Method {
			case "Network.webSocketCreated":
				if len(sockets) >= 128 {
					sockets = map[string]string{}
				}
				sockets[params.ID] = params.URL
			case "Network.webSocketFrameReceived":
				if b.isHAAuthResource(sockets[params.ID], "/api/websocket") && homeAssistantAuthFailureFrame(params.Response.Payload) {
					b.blockAuthentication(authEvidence{Kind: "invalid_token", Confidence: "confirmed", URL: sockets[params.ID], Resource: "WebSocket", Reason: "Home Assistant rejected this browser session", Action: "sign_in"})
					return context.Canceled
				}
			case "Network.responseReceived":
				raw := params.Response.URL
				if !b.isHAOrigin(raw) {
					continue
				}
				evidence := classifyHAResponse(params.Response.Status, params.Type, raw)
				if evidence.Kind == "token_response" {
					if len(tokenFailures) < 128 {
						tokenFailures[params.ID] = evidence
					}
					continue
				}
				if evidence.Kind != "" {
					b.blockAuthentication(evidence)
					return context.Canceled
				}
			case "Network.loadingFinished":
				if evidence, ok := tokenFailures[params.ID]; ok {
					delete(tokenFailures, params.ID)
					var body struct {
						Body   string `json:"body"`
						Base64 bool   `json:"base64Encoded"`
					}
					if err := session.command(ctx, "Network.getResponseBody", map[string]any{"requestId": params.ID}, &body); err != nil {
						continue
					}
					if body.Base64 {
						bytes, err := base64.StdEncoding.DecodeString(body.Body)
						if err != nil {
							continue
						}
						body.Body = string(bytes)
					}
					var failure struct {
						Error string `json:"error"`
					}
					if json.Unmarshal([]byte(body.Body), &failure) == nil && failure.Error == "invalid_grant" {
						evidence.Kind, evidence.Confidence, evidence.Reason, evidence.Action = "invalid_grant", "confirmed", "Home Assistant refresh grant is no longer valid", "sign_in"
						b.blockAuthentication(evidence)
						return context.Canceled
					}
				}
			}
		}
	}
}

func homeAssistantAuthFailureFrame(payload string) bool {
	return parseAuthFailure(payload)
}

func homeAssistantAuthFailureResponse(status int, kind string, rawURL string) bool {
	if !authRelevantResourceType(kind) || !likelyHomeAssistantURL(rawURL) {
		return false
	}
	evidence := classifyHAResponse(status, kind, rawURL)
	return evidence.Kind != "" && evidence.Kind != "token_response"
}

func configureHomeAssistantTheme(ctx context.Context, session cdpCommander, theme string) error {
	script, ok := homeAssistantThemeScript(theme)
	if !ok {
		return nil
	}
	desiredScheme := "light"
	if strings.EqualFold(strings.TrimSpace(theme), "dark") || strings.EqualFold(strings.TrimSpace(theme), "force-dark") {
		desiredScheme = "dark"
	}
	if err := session.command(ctx, "Emulation.setEmulatedMedia", map[string]any{
		"features": []map[string]string{{
			"name":  "prefers-color-scheme",
			"value": desiredScheme,
		}},
	}, nil); err != nil {
		return fmt.Errorf("emulate prefers-color-scheme %s: %w", desiredScheme, err)
	}
	if err := session.command(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{
		"source": script,
	}, nil); err != nil {
		return err
	}
	var result struct {
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := session.command(ctx, "Runtime.evaluate", map[string]any{
		"expression":    script,
		"awaitPromise":  false,
		"returnByValue": false,
	}, &result); err != nil {
		return err
	}
	if result.ExceptionDetails != nil {
		return fmt.Errorf("Home Assistant theme script failed: %s", result.ExceptionDetails.Text)
	}
	return nil
}

// TouchKio's Electron nativeTheme exposes dark/light as an OS color-scheme preference.
// Chromium CDP emulates that preference; this script only verifies the page applied it.
func homeAssistantThemeScript(theme string) (string, bool) {
	var desiredDark bool
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "dark", "force-dark":
		desiredDark = true
	case "light":
		desiredDark = false
	default:
		return "", false
	}
	return fmt.Sprintf(`(() => {
  const desiredDark = %t;
  const timerKey = "__kioskmateThemeTimer";
  const resultKey = "__kioskmateThemeResult";
  if (globalThis[timerKey]) clearInterval(globalThis[timerKey]);
  let attempts = 0;
  let stableChecks = 0;
  const report = (ok, error, selected, applied) => {
    const value = {
      ok,
      error: error || "",
      requested_theme: "preserve-current",
      requested_dark: desiredDark,
      selected_theme: selected && selected.theme || "",
      selected_dark: selected && typeof selected.dark === "boolean" ? selected.dark : null,
      applied_dark: typeof applied === "boolean" ? applied : null
    };
    globalThis[resultKey] = value;
    console.info("%s" + JSON.stringify(value));
  };
  const apply = () => {
    attempts += 1;
    const root = document.querySelector("home-assistant");
    const selected = root && root.hass && root.hass.selectedTheme;
    const applied = root && root.hass && root.hass.themes && root.hass.themes.darkMode;
    const mediaDark = matchMedia("(prefers-color-scheme: dark)").matches;
    if (mediaDark === desiredDark && applied === desiredDark) {
      stableChecks += 1;
      if (stableChecks >= 12) {
        clearInterval(globalThis[timerKey]);
        globalThis[timerKey] = 0;
        report(true, "", selected, applied);
      }
      return;
    }
    stableChecks = 0;
    if (attempts >= 120) {
      clearInterval(globalThis[timerKey]);
      globalThis[timerKey] = 0;
      report(false, root && root.hass ? "Home Assistant did not follow the emulated prefers-color-scheme value" : "Home Assistant frontend was not detected", selected, applied);
    }
  };
  globalThis[timerKey] = setInterval(apply, 250);
  apply();
})();`, desiredDark, themeStatusConsolePrefix), true
}

func parseThemeConsoleEvent(data json.RawMessage) (themeReport, bool) {
	var params struct {
		Args []struct {
			Value json.RawMessage `json:"value"`
		} `json:"args"`
	}
	if json.Unmarshal(data, &params) != nil {
		return themeReport{}, false
	}
	for _, arg := range params.Args {
		var value string
		if json.Unmarshal(arg.Value, &value) != nil || !strings.HasPrefix(value, themeStatusConsolePrefix) {
			continue
		}
		var report themeReport
		if json.Unmarshal([]byte(strings.TrimPrefix(value, themeStatusConsolePrefix)), &report) == nil {
			return report, true
		}
	}
	return themeReport{}, false
}

func (b *Browser) setThemeReport(configured string, report themeReport) {
	now := time.Now()
	requestedDark := report.RequestedDark
	state := "failed"
	if report.OK {
		state = "applied"
	}
	b.mu.Lock()
	b.themeStatus = ThemeStatus{
		State:          state,
		Configured:     configured,
		RequestedTheme: report.RequestedTheme,
		RequestedDark:  &requestedDark,
		SelectedTheme:  report.SelectedTheme,
		SelectedDark:   report.SelectedDark,
		AppliedDark:    report.AppliedDark,
		Error:          report.Error,
		Updated:        &now,
	}
	b.mu.Unlock()
	b.logger.Info("Home Assistant theme synchronization result", "state", state, "theme", report.SelectedTheme, "dark", report.AppliedDark, "error", report.Error)
}

func (b *Browser) setThemeError(configured string, err error) {
	now := time.Now()
	b.mu.Lock()
	b.themeStatus = ThemeStatus{State: "failed", Configured: configured, Error: err.Error(), Updated: &now}
	b.mu.Unlock()
}

func authRelevantResourceType(kind string) bool {
	switch kind {
	case "Document", "XHR", "Fetch", "WebSocket":
		return true
	default:
		return false
	}
}

func waitForDevToolsTarget(ctx context.Context, profile string) (devToolsTarget, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		target, err := readDevToolsTarget(ctx, profile)
		if err == nil {
			return target, nil
		}
		select {
		case <-ctx.Done():
			return devToolsTarget{}, fmt.Errorf("wait for Chromium DevTools: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func readDevToolsTarget(ctx context.Context, profile string) (devToolsTarget, error) {
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return devToolsTarget{}, err
	}
	lines := strings.Fields(string(data))
	if len(lines) < 1 {
		return devToolsTarget{}, errors.New("DevToolsActivePort is empty")
	}
	port, err := strconv.Atoi(lines[0])
	if err != nil || port < 1 || port > 65535 {
		return devToolsTarget{}, errors.New("DevToolsActivePort contains an invalid port")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/json/list", port), nil)
	if err != nil {
		return devToolsTarget{}, err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return devToolsTarget{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return devToolsTarget{}, fmt.Errorf("DevTools target list returned %s", resp.Status)
	}
	var targets []devToolsTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return devToolsTarget{}, err
	}
	for _, target := range targets {
		if target.Type == "page" && target.WebSocketDebuggerURL != "" {
			return target, nil
		}
	}
	return devToolsTarget{}, errors.New("no Chromium page target found")
}

func likelyHomeAssistantURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, ":8123") || strings.Contains(lower, "homeassistant") || strings.Contains(lower, "home-assistant") || strings.Contains(lower, "/api/websocket")
}
