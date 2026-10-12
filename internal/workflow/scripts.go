package workflow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Page scripts for the click, scroll and sound actions. Values are always
// JSON-encoded, never spliced into the source. Selectors also search open
// shadow roots, because Home Assistant renders its cards inside them.

const deepQuery = `const deepQuery = (selector, root = document) => {
  const found = root.querySelector(selector);
  if (found) return found;
  for (const element of root.querySelectorAll("*")) {
    if (element.shadowRoot) {
      const nested = deepQuery(selector, element.shadowRoot);
      if (nested) return nested;
    }
  }
  return null;
};`

func clickScript(selector string) string {
	encoded, _ := json.Marshal(strings.TrimSpace(selector))
	return fmt.Sprintf(`(() => {
  %s
  const element = deepQuery(%s);
  if (!element) throw new Error("no element matches " + %s);
  element.click();
})()`, deepQuery, encoded, encoded)
}

func scrollScript(value string) string {
	value = strings.TrimSpace(value)
	if pixels, err := strconv.Atoi(value); err == nil {
		return fmt.Sprintf(`(() => {
  const target = document.scrollingElement || document.documentElement;
  window.scrollBy({ top: %d, behavior: "smooth" });
  if (target) target.scrollBy({ top: %d, behavior: "smooth" });
})()`, pixels, pixels)
	}
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf(`(() => {
  %s
  const element = deepQuery(%s);
  if (!element) throw new Error("no element matches " + %s);
  element.scrollIntoView({ behavior: "smooth", block: "start" });
})()`, deepQuery, encoded, encoded)
}

// soundScript plays an audio URL, or a short two-tone chime when url is
// empty. Runtime.evaluate runs it as a user gesture, so autoplay rules allow it.
func soundScript(url string) string {
	url = strings.TrimSpace(url)
	if url != "" {
		encoded, _ := json.Marshal(url)
		return fmt.Sprintf(`new Audio(%s).play()`, encoded)
	}
	return `(() => {
  const context = new (window.AudioContext || window.webkitAudioContext)();
  [[880, 0], [660, 0.22]].forEach(([frequency, start]) => {
    const oscillator = context.createOscillator();
    const gain = context.createGain();
    oscillator.frequency.value = frequency;
    gain.gain.setValueAtTime(0.25, context.currentTime + start);
    gain.gain.exponentialRampToValueAtTime(0.001, context.currentTime + start + 0.2);
    oscillator.connect(gain).connect(context.destination);
    oscillator.start(context.currentTime + start);
    oscillator.stop(context.currentTime + start + 0.2);
  });
  setTimeout(() => context.close(), 800);
})()`
}
