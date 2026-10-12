package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/coder/websocket"
)

// Tab mode keeps recently shown pages open as Chromium tabs and switches with
// Target.activateTarget instead of navigating. A switch back to a page is then
// instant: no reload, no new Home Assistant login or WebSocket handshake.
// Hidden tabs are throttled by Chromium, so the cost is mostly memory, which
// kiosk.max_tabs bounds with least-recently-used eviction.

const defaultMaxTabs = 3

func maxTabs(cfg *config.Config) int {
	if cfg.Kiosk.MaxTabs >= 1 && cfg.Kiosk.MaxTabs <= 8 {
		return cfg.Kiosk.MaxTabs
	}
	return defaultMaxTabs
}

// resetTabsLocked forgets all tab bookkeeping; called whenever a new browser
// process starts. Callers hold b.mu.
func (b *Browser) resetTabsLocked() {
	b.tabs = map[int]string{}
	b.tabUse = nil
	b.activeTarget = ""
	b.retarget = false
	b.pendingBlank = map[string]bool{}
	b.tabURL = map[int]string{}
}

// bindControlTarget records the page target the control loop attached to and
// reports whether it is a new blank tab that still needs its first navigation.
func (b *Browser) bindControlTarget(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if id == "" {
		return false
	}
	if b.tabs == nil {
		b.tabs = map[int]string{}
	}
	if b.activeTarget == "" {
		b.activeTarget = id
		if _, ok := b.tabs[b.active]; !ok {
			b.tabs[b.active] = id
			b.touchTabLocked(b.active)
			if b.tabURL == nil {
				b.tabURL = map[int]string{}
			}
			b.tabURL[b.active] = activeURL(b.cfg.Snapshot(), b.active)
		}
	}
	if b.pendingBlank[id] {
		delete(b.pendingBlank, id)
		return true
	}
	return false
}

func (b *Browser) consumeRetarget() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.retarget {
		return false
	}
	b.retarget = false
	return true
}

func (b *Browser) touchTabLocked(index int) {
	out := b.tabUse[:0]
	for _, item := range b.tabUse {
		if item != index {
			out = append(out, item)
		}
	}
	b.tabUse = append(out, index)
}

// switchTab activates (or creates) the tab for page index and moves the
// control session to it.
func (b *Browser) switchTab(ctx context.Context, cfg *config.Config, index int) error {
	b.mu.Lock()
	running := b.cmd != nil && b.cmd.Process != nil
	profile := browserUserDataDir(cfg, index)
	id := b.tabs[index]
	b.mu.Unlock()
	if !running {
		return errors.New("browser is not running")
	}
	browser, err := dialBrowserTarget(ctx, profile)
	if err != nil {
		return err
	}
	defer browser.close()
	created := false
	if id != "" {
		if err := browser.command(ctx, "Target.activateTarget", map[string]any{"targetId": id}, nil); err != nil {
			id = ""
		}
	}
	if id == "" {
		var result struct {
			TargetID string `json:"targetId"`
		}
		if err := browser.command(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &result); err != nil {
			return err
		}
		if result.TargetID == "" {
			return errors.New("Chromium returned no target id for the new tab")
		}
		id, created = result.TargetID, true
	}
	b.mu.Lock()
	if b.tabs == nil {
		b.tabs = map[int]string{}
	}
	if b.pendingBlank == nil {
		b.pendingBlank = map[string]bool{}
	}
	if b.tabURL == nil {
		b.tabURL = map[int]string{}
	}
	b.tabs[index] = id
	// A new tab, or a kept tab whose page URL was edited meanwhile, is
	// navigated once the control session attaches to it.
	want := activeURL(cfg, index)
	if created || b.tabURL[index] != want {
		b.pendingBlank[id] = true
	}
	b.tabURL[index] = want
	b.touchTabLocked(index)
	var evict []string
	for len(b.tabUse) > maxTabs(cfg) {
		oldest := b.tabUse[0]
		b.tabUse = b.tabUse[1:]
		if target := b.tabs[oldest]; target != "" && target != id {
			evict = append(evict, target)
		}
		delete(b.tabs, oldest)
		delete(b.tabURL, oldest)
	}
	b.activeTarget = id
	b.retarget = true
	control := b.control
	b.mu.Unlock()
	for _, target := range evict {
		_ = browser.command(ctx, "Target.closeTarget", map[string]any{"targetId": target}, nil)
	}
	// Closing the control session makes the monitor loop re-attach to the
	// now active tab (see consumeRetarget).
	if control != nil {
		control.close()
	}
	return nil
}

// dialBrowserTarget connects to Chromium's browser-level DevTools endpoint,
// which exposes the Target and SystemInfo domains.
func dialBrowserTarget(ctx context.Context, profile string) (*cdpSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(data))
	if len(lines) < 2 {
		return nil, errors.New("DevToolsActivePort has no browser target")
	}
	port, err := strconv.Atoi(lines[0])
	if err != nil || port < 1 || port > 65535 || !strings.HasPrefix(lines[1], "/devtools/browser/") {
		return nil, errors.New("DevToolsActivePort is invalid")
	}
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d%s", port, lines[1]), &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("connect to Chromium browser target: %w", err)
	}
	conn.SetReadLimit(8 << 20)
	return newCDPSession(conn), nil
}
