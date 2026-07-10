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
	"sync/atomic"
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

type cdpSession struct {
	conn *websocket.Conn
	next atomic.Int64
}

func dialDevTools(ctx context.Context, profile string) (*cdpSession, error) {
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
	return &cdpSession{conn: conn}, nil
}

func (s *cdpSession) close() {
	_ = s.conn.Close(websocket.StatusNormalClosure, "done")
}

func (s *cdpSession) command(ctx context.Context, method string, params any, result any) error {
	id := s.next.Add(1)
	payload, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	if err := s.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return err
	}
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			return err
		}
		var message cdpMessage
		if err := json.Unmarshal(data, &message); err != nil || message.ID != id {
			continue
		}
		if message.Error != nil {
			return fmt.Errorf("DevTools %s failed: %s", method, message.Error.Message)
		}
		if result != nil && len(message.Result) > 0 {
			return json.Unmarshal(message.Result, result)
		}
		return nil
	}
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

func (b *Browser) reloadDevTools(ctx context.Context) error {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	profile := browserUserDataDir(cfg, b.active)
	running := b.cmd != nil && b.cmd.Process != nil
	b.mu.Unlock()
	if !running {
		return errors.New("browser is not running")
	}
	session, err := dialDevTools(ctx, profile)
	if err != nil {
		return err
	}
	defer session.close()
	return session.command(ctx, "Page.reload", map[string]any{"ignoreCache": false}, nil)
}

func (b *Browser) navigateDevTools(ctx context.Context, target string) error {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	profile := browserUserDataDir(cfg, b.active)
	b.mu.Unlock()
	session, err := dialDevTools(ctx, profile)
	if err != nil {
		return err
	}
	defer session.close()
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
	session, err := dialDevTools(ctx, profile)
	if err != nil {
		b.logger.Warn("Chromium DevTools unavailable", "error", err)
		return
	}
	defer session.close()
	if err := configureHomeAssistantTheme(ctx, session, theme); err != nil {
		b.logger.Warn("Home Assistant theme synchronization failed", "theme", theme, "error", err)
	} else if _, ok := homeAssistantThemeScript(theme); ok {
		b.logger.Info("Home Assistant theme synchronization enabled", "theme", theme)
	}
	if err := session.command(ctx, "Network.enable", map[string]any{}, nil); err != nil {
		b.logger.Warn("Chromium DevTools network monitor failed", "error", err)
		return
	}
	b.mu.Lock()
	b.devTools = true
	b.mu.Unlock()
	b.logger.Info("Chromium DevTools monitor attached")
	for {
		_, data, err := session.conn.Read(ctx)
		if err != nil {
			return
		}
		var event cdpMessage
		if json.Unmarshal(data, &event) != nil {
			continue
		}
		switch event.Method {
		case "Network.webSocketFrameReceived":
			var params struct {
				Response struct {
					PayloadData string `json:"payloadData"`
				} `json:"response"`
			}
			if json.Unmarshal(event.Params, &params) == nil && strings.Contains(params.Response.PayloadData, `"type":"auth_invalid"`) {
				b.tripAuthGuard("Home Assistant rejected the stored WebSocket access token")
				return
			}
		case "Network.responseReceived":
			var params struct {
				Type     string `json:"type"`
				Response struct {
					Status float64 `json:"status"`
					URL    string  `json:"url"`
				} `json:"response"`
			}
			if json.Unmarshal(event.Params, &params) == nil && int(params.Response.Status) == http.StatusForbidden && authRelevantResourceType(params.Type) && likelyHomeAssistantURL(params.Response.URL) {
				b.tripAuthGuard("Home Assistant returned HTTP 403 for " + params.Type)
				return
			}
		}
	}
}

func configureHomeAssistantTheme(ctx context.Context, session *cdpSession, theme string) error {
	script, ok := homeAssistantThemeScript(theme)
	if !ok {
		return nil
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

// Home Assistant stores theme mode per user and can override Chromium's media preference.
// Use its frontend event so native theme variables are applied without force-dark rendering.
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
  if (globalThis[timerKey]) clearInterval(globalThis[timerKey]);
  let attempts = 0;
  let stableChecks = 0;
  const apply = () => {
    attempts += 1;
    const root = document.querySelector("home-assistant");
    const selected = root && root.hass && root.hass.selectedTheme;
    const applied = root && root.hass && root.hass.themes && root.hass.themes.darkMode;
    if (selected && selected.dark === desiredDark && applied === desiredDark) {
      stableChecks += 1;
      if (stableChecks >= 12) {
        clearInterval(globalThis[timerKey]);
        globalThis[timerKey] = 0;
      }
      return;
    }
    stableChecks = 0;
    if (root && root.hass) {
      root.dispatchEvent(new CustomEvent("settheme", {
        detail: { dark: desiredDark },
        bubbles: true,
        composed: true
      }));
    }
    if (attempts >= 120) {
      clearInterval(globalThis[timerKey]);
      globalThis[timerKey] = 0;
    }
  };
  globalThis[timerKey] = setInterval(apply, 250);
  apply();
})();`, desiredDark), true
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
