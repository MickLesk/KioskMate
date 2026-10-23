// Page sequence helpers shared by the programme and pages views.
import { formatDuration, state, t } from "./app-core.js";
import { sourceLabel } from "./renderers.js";
import { describeClock } from "./clock.js";

      export function selectedKioskPageIndex(pages) {
        // state.status.browser.active is in enabled-page index space (matches backend
        // SetActive/PageCount), so it must be converted to an absolute index before it
        // can be used to index into the full (including disabled) pages array.
        const fallback = absolutePageIndexFromEnabled(pages, Number(state.status?.browser?.active || 0));
        const selected = state.kioskSelectedPageIndex === null ? fallback : Number(state.kioskSelectedPageIndex);
        return Math.max(0, Math.min(pages.length - 1, Number.isFinite(selected) ? selected : 0));
      }

      // Converts an absolute index into `pages` (which includes disabled/url-less
      // pages) into the enabled-page index space expected by /api/browser/page and
      // /api/browser/override. Returns -1 if the page at absoluteIndex is not enabled.
      export function enabledPageIndexFromAbsolute(pages, absoluteIndex) {
        let enabledIndex = 0;
        for (let i = 0; i < pages.length; i++) {
          const isEnabled = !pages[i].disabled && pages[i].url;
          if (i === absoluteIndex) return isEnabled ? enabledIndex : -1;
          if (isEnabled) enabledIndex++;
        }
        return -1;
      }

      // Converts an enabled-page index (as reported by the backend, e.g. browser.active)
      // back into the absolute index used by the UI's pages array. Returns -1 if no such
      // enabled page exists.
      export function absolutePageIndexFromEnabled(pages, enabledIndex) {
        let counted = 0;
        for (let i = 0; i < pages.length; i++) {
          if (!pages[i].disabled && pages[i].url) {
            if (counted === enabledIndex) return i;
            counted++;
          }
        }
        return -1;
      }

      export function pageTimingLabel(page) {
        if (page.display_mode === "schedule") return `${describeClock(page.schedule?.start, false)} - ${describeClock(page.schedule?.end, false)}`;
        if (page.display_mode === "mqtt") return t("mqttTrigger");
        if (page.display_mode === "insert") return t("insertOnlyShort");
        return formatDuration(Number(page.duration_seconds || 3600));
      }

      export function displayModeLabel(mode) {
        return t({ schedule: "fixedSchedule", mqtt: "triggerBased", insert: "insertOnly" }[mode] || "customDuration");
      }

      export function localPageSummary(page) {
        const local = page.local || {};
        if (page.source_type === "slideshow") return local.folder || t("notConfigured");
        return local.title || sourceLabel(page.source_type);
      }
