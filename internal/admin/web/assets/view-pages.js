// Pages: everything the kiosk can show, as a library. When a page runs is
// set in the programme; this view only adds, edits and checks pages.
import { openPageWizard } from "./actions-kiosk.js";
import { esc, isDirty, postJSON, refreshCore, renderApp, runAction, state, t } from "./app-core.js";
import { icon } from "./icons.js";
import { button, collectPages, isLocalSource, normalizePages, sourceLabel, sourceMark } from "./renderers.js";
import { describeInsert } from "./view-program.js";
import { enabledPageIndexFromAbsolute, localPageSummary, pageTimingLabel } from "./view-kiosk.js";

const NEW_TYPES = [
  ["home_assistant", "homeAssistantDashboard", "newHomeAssistantHint"],
  ["url", "webAddress", "newWebsiteHint"],
  ["clock", "sourceClock", "newClockHint"],
  ["slideshow", "sourceSlideshow", "newSlideshowHint"],
  ["html", "sourceHtml", "newHtmlHint"],
];

// Where a page is used: base programme, time window, inserts, workflows.
function pageUses(page) {
  const uses = [];
  if (page.display_mode === "duration") uses.push(`${t("baseProgramme")} · ${pageTimingLabel(page)}`);
  if (page.display_mode === "schedule") uses.push(`${t("timeWindow")} ${pageTimingLabel(page)}`);
  if (page.display_mode === "mqtt") uses.push(t("triggerBased"));
  for (const insert of state.config?.kiosk?.inserts || []) {
    if (insert.page_id === page.page_id && !insert.disabled) uses.push(`${t("insert")}: ${describeInsert(insert).split(" · ")[0]}`);
  }
  for (const rule of state.config?.kiosk?.rules || []) {
    if (!rule.disabled && (rule.actions || []).some((action) => action.type === "show_page" && action.page_id === page.page_id)) uses.push(`${t("navAutomations")}: ${rule.name || rule.id}`);
  }
  return uses;
}

function pageHost(page) {
  if (isLocalSource(page.source_type)) return localPageSummary(page);
  try {
    const parsed = new URL(page.url);
    return `${parsed.host}${parsed.pathname === "/" ? "" : parsed.pathname}`;
  } catch {
    return page.url || t("notConfigured");
  }
}

export function renderPages() {
  const pages = normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls);
  const filter = String(state.pageFilter || "").trim().toLowerCase();
  const active = Number(state.status?.browser?.active ?? -1);
  return `
    <div class="page-stack pages-view">
      <section class="card new-page-card" aria-labelledby="new-page-title">
        <div class="head"><div><h3 id="new-page-title">${esc(t("addNewPage"))}</h3><span class="section-kicker">${esc(t("addNewPageHint"))}</span></div></div>
        <div class="body new-page-grid">
          ${NEW_TYPES.map(([type, label, hint]) => `<button class="new-page-type" data-page-new="${type}"><span class="source-mark">${esc(sourceMark(type))}</span><strong>${esc(t(label))}</strong><small>${esc(t(hint))}</small></button>`).join("")}
        </div>
      </section>
      <div class="pages-toolbar">
        <label class="search-field">${icon("pages", 16)}<span class="visually-hidden">${esc(t("searchPages"))}</span><input id="page-filter" type="search" data-no-dirty placeholder="${esc(t("searchPages"))}" value="${esc(state.pageFilter || "")}"></label>
        <span class="hint" id="visible-pages-count">${esc(t("pageCount").replace("{count}", pages.length))}</span>
      </div>
      ${pages.length ? `<section class="page-library" aria-label="${esc(t("navPages"))}">${pages.map((page, index) => {
        const enabled = enabledPageIndexFromAbsolute(pages, index);
        const uses = pageUses(page);
        const hidden = filter && !`${page.name} ${page.url}`.toLowerCase().includes(filter);
        const onScreen = enabled >= 0 && enabled === active && state.status?.browser?.running;
        return `<article class="library-card source-${esc(page.source_type)} ${page.disabled ? "disabled" : ""}" data-page-card="${index}" ${hidden ? "hidden" : ""}>
          <div class="library-thumb">
            <span class="source-mark">${esc(sourceMark(page.source_type))}</span>
            <span class="library-kind">${esc(sourceLabel(page.source_type))}</span>
            ${onScreen ? `<span class="library-live">${esc(t("onScreenNow"))}</span>` : ""}
          </div>
          <div class="library-body">
            <strong>${esc(page.name || `${t("pages")} ${index + 1}`)}</strong>
            <span class="library-url">${esc(pageHost(page))}</span>
            <div class="tag-row">
              ${page.disabled ? `<span class="tag muted-tag">${esc(t("disabled"))}</span>` : ""}
              ${uses.length ? uses.map((use) => `<span class="tag">${esc(use)}</span>`).join("") : `<span class="tag muted-tag">${esc(t("notScheduled"))}</span>`}
              ${page.preload ? `<span class="tag">${esc(t("preloaded"))}</span>` : ""}
            </div>
            <div class="library-actions">
              <button data-page-show="${index}" ${enabled < 0 ? "disabled" : ""} title="${esc(t("showNowHint"))}">${esc(t("showNow"))}</button>
              <button data-page-edit="${index}">${esc(t("edit"))}</button>
              <button class="danger-ghost" data-page-remove="${index}" title="${esc(t("remove"))}" aria-label="${esc(t("remove"))}">${icon("close", 16)}</button>
            </div>
          </div>
        </article>`;
      }).join("")}</section>` : `<div class="empty"><strong>${esc(t("noPagesConfigured"))}</strong><span>${esc(t("addFirstPageHint"))}</span></div>`}
      <details class="card disclosure advanced-settings">
        <summary>${esc(t("pagesAdvanced"))}</summary>
        <div class="disclosure-body advanced-sequence-grid">
          <div class="actions">
            ${button("checkAllPages", "page-check-all")}
            ${button("importPages", "pages-import")}
            ${button("exportPages", "pages-export")}
            ${button("enableAllPages", "page-enable-all")}
            ${button("disableAllPages", "page-disable-all")}
          </div>
          <input id="pages-import-file" type="file" accept="application/json,.json" class="hidden" />
          <div id="page-check-all-output" class="check-panel"></div>
        </div>
      </details>
      <div class="save-bar">
        <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
        <div class="actions">${button("save", "kiosk-save")}${button("saveStartKiosk", "kiosk-save-restart", "primary")}</div>
      </div>
    </div>`;
}

export function bindPages() {
  document.querySelectorAll("[data-page-new]").forEach((button) => button.addEventListener("click", () => openPageWizard(null, collectPages().length, button.dataset.pageNew)));
  document.getElementById("page-filter")?.addEventListener("input", (event) => {
    state.pageFilter = event.target.value || "";
    const filter = state.pageFilter.trim().toLowerCase();
    const pages = collectPages();
    let visible = 0;
    document.querySelectorAll("[data-page-card]").forEach((card) => {
      const page = pages[Number(card.dataset.pageCard)] || {};
      card.hidden = !!filter && !`${page.name} ${page.url}`.toLowerCase().includes(filter);
      if (!card.hidden) visible++;
    });
    const counter = document.getElementById("visible-pages-count");
    if (counter) counter.textContent = t("pageCount").replace("{count}", visible);
  });
  document.querySelectorAll("[data-page-show]").forEach((button) => button.addEventListener("click", () => {
    const pages = collectPages();
    const page = enabledPageIndexFromAbsolute(pages, Number(button.dataset.pageShow));
    runAction("page-show", async () => {
      if (page < 0) throw new Error(t("validationPageDisabledForActivation"));
      await postJSON("/api/browser/override", { page, duration_seconds: 3600, source: "admin-activate" });
      await refreshCore();
      renderApp();
    }, t("overrideApplied"));
  }));
}
