// Page sequence helpers shared by the programme and pages views.
import { esc, formatDuration, state, t } from "./app-core.js";
import { isLocalSource, normalizePages, sourceLabel, sourceMark } from "./renderers.js";

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
        if (page.display_mode === "schedule") return `${page.schedule?.start || "--:--"} - ${page.schedule?.end || "--:--"}`;
        if (page.display_mode === "mqtt") return t("mqttTrigger");
        if (page.display_mode === "insert") return t("insertOnlyShort");
        return formatDuration(Number(page.duration_seconds || 3600));
      }

      export function displayModeLabel(mode) {
        return t({ schedule: "fixedSchedule", mqtt: "triggerBased", insert: "insertOnly" }[mode] || "customDuration");
      }

      export function renderKioskStorybook(pages, selected) {
        if (!pages.length) return `<div class="sequence-empty"><strong>${esc(t("noPagesConfigured"))}</strong><span>${esc(t("addFirstPageHint"))}</span><button class="primary" data-action="page-wizard-new">+ ${esc(t("newPage"))}</button></div>`;
        return `<div class="storybook-track" data-sequence-track>
          ${pages.map((page, index) => `${index ? `<button class="insert-page" data-page-insert="${index}" title="${esc(t("insertPageHere"))}">+</button>` : ""}${renderSequenceCard(page, index, selected)}`).join("")}
          <button class="add-page-card" data-page-insert="${pages.length}"><span>+</span><strong>${esc(t("addPage"))}</strong><small>${esc(t("addPageAtEnd"))}</small></button>
        </div>`;
      }

      export function localPageSummary(page) {
        const local = page.local || {};
        if (page.source_type === "slideshow") return local.folder || t("notConfigured");
        return local.title || sourceLabel(page.source_type);
      }

      function renderSequenceCard(page, index, selected) {
        let host = "";
        if (isLocalSource(page.source_type)) host = sourceLabel(page.source_type);
        else try { host = new URL(page.url).host; } catch { host = page.url || t("notConfigured"); }
        return `<article class="sequence-card ${selected === index ? "selected" : ""} ${page.disabled ? "disabled" : ""}" draggable="true" data-sequence-index="${index}" data-page-select="${index}">
          <div class="sequence-card-top"><span class="step-number">${index + 1}</span><span class="duration-badge">${esc(pageTimingLabel(page))}</span><button class="icon-button drag-handle" data-page-drag-handle title="${esc(t("dragToReorder"))}" aria-label="${esc(t("dragToReorder"))}">&#8942;&#8942;</button></div>
          <button class="page-preview-tile" data-page-edit="${index}" title="${esc(t("editPage"))}">
            <span class="source-mark">${esc(sourceMark(page.source_type))}</span>
            <strong>${esc(host)}</strong>
            <small>${esc(displayModeLabel(page.display_mode))}</small>
          </button>
          <div class="sequence-card-copy"><strong>${esc(page.name || `${t("pages")} ${index + 1}`)}</strong><span>${esc(isLocalSource(page.source_type) ? localPageSummary(page) : page.url || t("notConfigured"))}</span></div>
          <div class="sequence-card-actions">
            <button data-page-move="${index}" data-direction="-1" ${index === 0 ? "disabled" : ""} title="${esc(t("moveUp"))}" aria-label="${esc(t("moveUp"))}">&larr;</button>
            <button data-page-move="${index}" data-direction="1" ${index === normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls).length - 1 ? "disabled" : ""} title="${esc(t("moveDown"))}" aria-label="${esc(t("moveDown"))}">&rarr;</button>
            <button data-page-edit="${index}">${esc(t("edit"))}</button>
            <button class="danger-ghost" data-page-remove="${index}" title="${esc(t("remove"))}">&times;</button>
          </div>
        </article>`;
      }
