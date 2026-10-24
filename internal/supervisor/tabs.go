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
	b.insertTab = false
}

// keepTabForInsertLocked prepares navigate mode for an insert: the tab now
// on screen is recorded as the tab of page base, so switchTab opens the
// insert next to it and can activate it again afterwards. It reports false
// when the control session has no tab yet. Callers hold b.mu.
func (b *Browser) keepTabForInsertLocked(cfg *config.Config, base int) bool {
	if b.activeTarget == "" {
		return false
	}
	b.tabs = map[int]string{base: b.activeTarget}
	b.tabURL = map[int]string{base: activeURL(cfg, base)}
	b.tabUse = []int{base}
	b.pendingBlank = map[string]bool{}
	b.insertBase = base
	return true
}

// closeOtherTabs closes every kept tab except the one of page keep and ends
// the insert bookkeeping of navigate mode.
func (b *Browser) closeOtherTabs(ctx context.Context, cfg *config.Config, keep int) {
	b.mu.Lock()
	keepID := b.tabs[keep]
	if keepID == "" {
		// The switch fell back to navigating the tab on screen.
		keepID = b.activeTarget
	}
	var closing []string
	for index, id := range b.tabs {
		if index != keep && id != keepID && id != b.activeTarget {
			closing = append(closing, id)
		}
	}
	b.insertTab = false
	if keepID != "" {
		b.tabs = map[int]string{keep: keepID}
		b.tabURL = map[int]string{keep: activeURL(cfg, keep)}
		b.tabUse = []int{keep}
	}
	profile := browserUserDataDir(cfg, keep)
	b.mu.Unlock()
	if len(closing) == 0 {
		return
	}
	browser, err := dialBrowserTarget(ctx, profile)
	if err != nil {
		b.logger.Debug("closing insert tabs skipped", "error", err)
		return
	}
	defer browser.close()
	for _, id := range closing {
		_ = browser.command(ctx, "Target.closeTarget", map[string]any{"targetId": id}, nil)
	}
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
	evict := b.evictTabsLocked(cfg, index)
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

// evictTabsLocked closes least recently used tabs above kiosk.max_tabs.
// Preloaded pages and the page being shown are never evicted. Callers hold
// b.mu.
func (b *Browser) evictTabsLocked(cfg *config.Config, keep int) []string {
	preloaded := map[int]bool{}
	for _, index := range cfg.Kiosk.PreloadIndexes() {
		preloaded[index] = true
	}
	limit := maxTabs(cfg)
	if b.insertTab || cfg.Kiosk.PageSwitch != "tabs" {
		// The page underneath an insert must survive it.
		preloaded[b.insertBase] = true
		limit = max(limit, 2)
	}
	var evict []string
	for len(b.tabUse) > limit {
		victim := -1
		for position, index := range b.tabUse {
			if index != keep && !preloaded[index] {
				victim = position
				break
			}
		}
		if victim < 0 {
			break
		}
		oldest := b.tabUse[victim]
		b.tabUse = append(b.tabUse[:victim], b.tabUse[victim+1:]...)
		if target := b.tabs[oldest]; target != "" && target != b.tabs[keep] {
			evict = append(evict, target)
		}
		delete(b.tabs, oldest)
		delete(b.tabURL, oldest)
	}
	return evict
}

// preloadTabs opens the pages marked "preload" as background tabs once the
// first page is shown, so switching to them later is instant.
func (b *Browser) preloadTabs(ctx context.Context, cfg *config.Config) {
	if cfg.Kiosk.PageSwitch != "tabs" || cfg.Kiosk.IsolateSessions {
		return
	}
	indexes := cfg.Kiosk.PreloadIndexes()
	if len(indexes) == 0 || b.authenticationAllowed() != nil {
		return
	}
	b.mu.Lock()
	profile := browserUserDataDir(cfg, b.active)
	b.mu.Unlock()
	browser, err := dialBrowserTarget(ctx, profile)
	if err != nil {
		b.logger.Debug("tab preload skipped", "error", err)
		return
	}
	defer browser.close()
	for _, index := range indexes {
		b.mu.Lock()
		_, open := b.tabs[index]
		b.mu.Unlock()
		if open {
			continue
		}
		url := activeURL(cfg, index)
		var result struct {
			TargetID string `json:"targetId"`
		}
		if err := browser.command(ctx, "Target.createTarget", map[string]any{"url": url, "background": true}, &result); err != nil || result.TargetID == "" {
			b.logger.Warn("tab preload failed", "page", index, "error", err)
			continue
		}
		b.mu.Lock()
		if b.tabs == nil {
			b.tabs = map[int]string{}
		}
		if b.tabURL == nil {
			b.tabURL = map[int]string{}
		}
		b.tabs[index] = result.TargetID
		b.tabURL[index] = url
		// Preloaded tabs count as least recently used so a page actually
		// shown moves ahead of them.
		b.tabUse = append([]int{index}, b.tabUse...)
		b.mu.Unlock()
	}
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
