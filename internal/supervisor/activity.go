package supervisor

import (
	"context"
	"encoding/json"
	"time"
)

// Touch and idle workflow triggers need to know when someone uses the kiosk.
// Every page gets a tiny listener that reports input through a DevTools
// binding, at most every two seconds; no polling and no extra process.

const activityBinding = "__kioskmateActivity"

const activityScript = `(() => {
  if (window.__kioskmateActivityInstalled || typeof window.` + activityBinding + ` !== "function") return;
  window.__kioskmateActivityInstalled = true;
  let last = 0;
  const report = () => {
    const now = Date.now();
    if (now - last < 2000) return;
    last = now;
    try { window.` + activityBinding + `("input"); } catch (_) {}
  };
  for (const type of ["pointerdown", "touchstart", "keydown", "wheel"]) {
    window.addEventListener(type, report, { capture: true, passive: true });
  }
})()`

// installActivityListener registers the binding and the listener for the
// current and all future documents of the control target.
func installActivityListener(ctx context.Context, session cdpCommander) error {
	if err := session.command(ctx, "Runtime.addBinding", map[string]any{"name": activityBinding}, nil); err != nil {
		return err
	}
	if err := session.command(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": activityScript}, nil); err != nil {
		return err
	}
	return session.command(ctx, "Runtime.evaluate", map[string]any{"expression": activityScript}, nil)
}

func activityBindingCall(params json.RawMessage) bool {
	var call struct {
		Name string `json:"name"`
	}
	return json.Unmarshal(params, &call) == nil && call.Name == activityBinding
}

// SetActivityHandler registers a callback for user input on the kiosk page.
func (b *Browser) SetActivityHandler(handler func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activityHandler = handler
}

// LastActivity returns when the kiosk page last saw user input, or zero.
func (b *Browser) LastActivity() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastActivity
}

func (b *Browser) noteActivity() {
	b.mu.Lock()
	b.lastActivity = time.Now()
	handler := b.activityHandler
	b.mu.Unlock()
	if handler != nil {
		go handler()
	}
}

// OverrideSource reports whether a page override is active and who set it
// ("admin", "home_assistant", "workflow", ...).
func (b *Browser) OverrideSource() (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.override.Active || (b.override.Until != nil && !time.Now().Before(*b.override.Until)) {
		return false, ""
	}
	return true, b.override.Source
}
