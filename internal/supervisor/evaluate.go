package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Evaluate runs JavaScript in the active kiosk page through the existing
// DevTools control session. Workflow "script" actions use it.
func (b *Browser) Evaluate(ctx context.Context, script string) error {
	if err := b.authenticationAllowed(); err != nil {
		return err
	}
	session, err := b.controlSession()
	if err != nil {
		return err
	}
	var result struct {
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := session.command(ctx, "Runtime.evaluate", map[string]any{
		"expression":    script,
		"returnByValue": true,
		"userGesture":   true,
	}, &result); err != nil {
		return err
	}
	if details := result.ExceptionDetails; details != nil {
		message := strings.TrimSpace(details.Exception.Description)
		if message == "" {
			message = details.Text
		}
		return fmt.Errorf("page script failed: %s", message)
	}
	return nil
}

// ShowOverlay displays text as a banner over the current page for duration
// without navigating. The text is inserted with textContent, never as HTML.
func (b *Browser) ShowOverlay(ctx context.Context, text string, duration time.Duration) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("overlay text is empty")
	}
	return b.Evaluate(ctx, overlayScript(text, duration))
}

func overlayScript(text string, duration time.Duration) string {
	if duration <= 0 {
		duration = 10 * time.Second
	}
	encoded, _ := json.Marshal(text)
	return fmt.Sprintf(`(() => {
  const id = "kioskmate-overlay";
  document.getElementById(id)?.remove();
  const box = document.createElement("div");
  box.id = id;
  box.textContent = %s;
  box.setAttribute("role", "status");
  box.style.cssText = "position:fixed;left:50%%;top:24px;transform:translateX(-50%%);z-index:2147483647;max-width:min(90vw,960px);padding:18px 28px;border-radius:14px;background:rgba(10,14,18,.92);color:#fff;font:600 clamp(18px,2.4vw,34px)/1.3 system-ui,sans-serif;box-shadow:0 12px 40px rgba(0,0,0,.45);text-align:center;pointer-events:none";
  document.documentElement.appendChild(box);
  setTimeout(() => box.remove(), %d);
})()`, encoded, duration.Milliseconds())
}

// ActivePage returns the index of the page currently shown.
func (b *Browser) ActivePage() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.active
}

// PageVideo describes one <video> element on the page on screen.
type PageVideo struct {
	// Kind is webrtc (a MediaStream, as Home Assistant's WebRTC players
	// use), mse (Media Source, for example HLS or go2rtc MSE) or file.
	Kind    string `json:"kind"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Playing bool   `json:"playing"`
}

// pageVideosScript lists the videos of the page, including those inside
// shadow roots (Home Assistant cards) and same-origin iframes.
const pageVideosScript = `(() => {
  const out = [];
  const seen = new Set();
  const visit = (root, depth) => {
    if (!root || depth > 40 || seen.has(root)) return;
    seen.add(root);
    for (const el of root.querySelectorAll("*")) {
      if (el.tagName === "VIDEO") {
        const kind = el.srcObject && typeof MediaStream !== "undefined" && el.srcObject instanceof MediaStream ? "webrtc"
          : String(el.currentSrc || el.src || "").startsWith("blob:") ? "mse" : "file";
        out.push({kind, width: el.videoWidth || 0, height: el.videoHeight || 0, playing: !el.paused && !el.ended && el.readyState > 2});
        if (out.length >= 32) return;
      }
      if (el.shadowRoot) visit(el.shadowRoot, depth + 1);
      if (el.tagName === "IFRAME") { try { visit(el.contentDocument, depth + 1); } catch (e) {} }
    }
  };
  visit(document, 0);
  return out;
})()`

// PageVideos reports the videos on the page currently shown.
func (b *Browser) PageVideos(ctx context.Context) ([]PageVideo, error) {
	session, err := b.controlSession()
	if err != nil {
		return nil, err
	}
	var result struct {
		Result struct {
			Value []PageVideo `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := session.command(ctx, "Runtime.evaluate", map[string]any{
		"expression":    pageVideosScript,
		"returnByValue": true,
	}, &result); err != nil {
		return nil, err
	}
	if result.ExceptionDetails != nil {
		return nil, fmt.Errorf("video scan failed: %s", result.ExceptionDetails.Text)
	}
	return result.Result.Value, nil
}
