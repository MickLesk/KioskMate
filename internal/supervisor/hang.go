package supervisor

import (
	"context"
	"errors"
	"time"
)

// Renderer heartbeat: a page whose JavaScript no longer runs (endless loop,
// out of memory, GPU process stuck) can keep its DevTools connection alive
// while the screen is frozen. Every 30 s heartbeat evaluates a trivial
// expression in a page that finished loading. Two missed answers in a row
// reload the page, four restart the browser, and when the restart budget is
// used up and watchdog.reboot_on_hang is set, the system is rebooted.

var (
	errRendererHung   = errors.New("kiosk page stopped responding")
	errRecoveryBudget = errors.New("automatic recovery budget exhausted")
)

// rendererResponds reports whether the page answers a trivial evaluation.
func rendererResponds(ctx context.Context, session cdpCommander) bool {
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := session.command(probe, "Runtime.evaluate", map[string]any{"expression": "1", "returnByValue": true}, nil)
	return err == nil || ctx.Err() != nil
}

// SetRebootHandler sets how the last stage of the renderer watchdog reboots
// the system (main wires it to the reboot system action).
func (b *Browser) SetRebootHandler(handler func(context.Context) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rebootHandler = handler
}

// handleRendererHang is called after the page kept missing heartbeats
// despite a reload. It restarts the browser and escalates to a reboot when allowed.
func (b *Browser) handleRendererHang() {
	b.logger.Warn("kiosk page unresponsive after reload; restarting browser")
	b.mu.Lock()
	journal := b.journal
	b.mu.Unlock()
	if journal != nil {
		journal.Record("browser", "renderer_hang", "running", "page did not respond after a reload; restarting the browser", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err := b.operate(ctx, "recover", func() error {
		return b.recover(ctx, "renderer unresponsive", true)
	})
	if err == nil || !errors.Is(err, errRecoveryBudget) {
		return
	}
	b.mu.Lock()
	reboot := b.rebootHandler
	allowed := b.cfg.Snapshot().Watchdog.RebootOnHang && !b.rebootRequested
	if allowed {
		b.rebootRequested = true
	}
	b.mu.Unlock()
	if !allowed || reboot == nil {
		return
	}
	b.logger.Warn("browser restarts did not help; rebooting", "error", err)
	if journal != nil {
		journal.Record("browser", "renderer_hang", "reboot", "restart budget exhausted; rebooting the system", nil)
	}
	if err := reboot(ctx); err != nil {
		b.logger.Warn("watchdog reboot failed", "error", err)
		b.mu.Lock()
		b.rebootRequested = false
		b.mu.Unlock()
	}
}
