// Pages: everything the kiosk can show, as a library. When a page runs is
// set in the programme; this view adds, edits, checks and filters pages.
import { openPageWizard, showPageNow, synchronizeKioskWorkflow } from "./actions-kiosk.js";
import { cloneConfig, esc, formatDuration, isDirty, markDirty, postJSON, renderApp, runAction, state, t } from "./app-core.js";
import { icon } from "./icons.js";
import { button, collectPages, isLocalSource, normalizePages, sourceLabel, sourceMark } from "./renderers.js";
import { describeInsert } from "./view-program.js";
import { localPageSummary } from "./view-kiosk.js";

const NEW_TYPES = [
  ["home_assistant", "homeAssistantDashboard", "newHomeAssistantHint"],
  ["url", "webAddress", "newWebsiteHint"],
  ["clock", "sourceClock", "newClockHint"],
  ["slideshow", "sourceSlideshow", "newSlideshowHint"],
  ["html", "sourceHtml", "newHtmlHint"],
];

const KINDS = ["all", "base", "insert", "trigger", "unused", "disabled"];

// Where a page is used: base programme, time window, inserts, workflows.
function pageUses(page) {
  const uses = [];
  if (page.display_mode === "duration") uses.push({ kind: "base", text: `${t("rotationShort")} · ${formatDuration(Number(page.duration_seconds || 3600))}` });
  if (page.display_mode === "schedule") uses.push({ kind: "base", text: `${t("timeWindow")} ${page.schedule?.start || "--:--"}–${page.schedule?.end || "--:--"}` });
  if (page.display_mode === "mqtt") uses.push({ kind: "trigger", text: t("triggerBased") });
  for (const insert of state.config?.kiosk?.inserts || []) {
    if (insert.page_id === page.page_id && !insert.disabled) uses.push({ kind: "insert", text: `${t("insert")}: ${describeInsert(insert).split(" · ")[0]}` });
  }
  for (const rule of state.config?.kiosk?.rules || []) {
    if (!rule.disabled && (rule.actions || []).some((action) => action.type === "show_page" && action.page_id === page.page_id)) uses.push({ kind: "trigger", text: `${t("navAutomations")}: ${rule.name || rule.id}` });
  }
  return uses;
}

function pageKind(page, uses) {
  if (page.disabled) return ["disabled"];
  const kinds = new Set(uses.map((use) => use.kind));
  if (!kinds.size) kinds.add("unused");
  return [...kinds];
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

function matches(page, kinds) {
  const filter = String(state.pageFilter || "").trim().toLowerCase();
  const kind = state.pageKind || "all";
  if (kind !== "all" && !kinds.includes(kind)) return false;
  return !filter || `${page.name} ${page.url}`.toLowerCase().includes(filter);
}

function checkChip(page) {
  const result = state.pageChecks?.[page.page_id];
  if (!result) return "";
  if (result.pending) return `<span class="check-chip pending">${esc(t("checking"))}</span>`;
  return `<span class="check-chip ${result.ok ? "ok" : "bad"}" title="${esc(result.detail || "")}">${icon(result.ok ? "check" : "warn", 12)}${esc(result.label)}</span>`;
}

export function renderPages() {
  const pages = normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls);
  const saved = normalizePages(state.persistedConfig?.kiosk?.pages, state.persistedConfig?.kiosk?.urls).filter((page) => !page.disabled && page.url);
  const activeID = state.status?.browser?.running ? saved[Number(state.status?.browser?.active ?? -1)]?.page_id : "";
  const rows = pages.map((page, index) => {
    const uses = pageUses(page);
    return { page, index, uses, kinds: pageKind(page, uses) };
  });
  const counts = Object.fromEntries(KINDS.map((kind) => [kind, kind === "all" ? rows.length : rows.filter((row) => row.kinds.includes(kind)).length]));
  const visible = rows.filter((row) => matches(row.page, row.kinds)).length;
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
        <div class="filter-chips" role="radiogroup" aria-label="${esc(t("filterPages"))}">
          ${KINDS.filter((kind) => kind === "all" || counts[kind]).map((kind) => `<button role="radio" aria-checked="${(state.pageKind || "all") === kind}" class="${(state.pageKind || "all") === kind ? "active" : ""}" data-page-kind="${kind}">${esc(t(`pageKind_${kind}`))} <span>${counts[kind]}</span></button>`).join("")}
        </div>
        <button data-busy="page-check-all" data-action="page-check-all-cards" ${pages.length ? "" : "disabled"}>${esc(t("checkAllPages"))}</button>
      </div>
      ${pages.length ? `<section class="page-library" aria-label="${esc(t("navPages"))}">${rows.map(({ page, index, uses, kinds }) => {
        const onScreen = page.page_id === activeID;
        return `<article class="library-card source-${esc(page.source_type)} ${page.disabled ? "disabled" : ""} ${onScreen ? "on-screen" : ""}" data-page-card="${index}" ${matches(page, kinds) ? "" : "hidden"}>
          <div class="library-thumb">
            <span class="source-mark">${esc(sourceMark(page.source_type))}</span>
            <span class="library-kind">${esc(sourceLabel(page.source_type))}</span>
            ${onScreen ? `<span class="library-live">${esc(t("onScreenNow"))}</span>` : ""}
            ${checkChip(page)}
          </div>
          <div class="library-body">
            <div class="library-title">
              <strong>${esc(page.name || `${t("pages")} ${index + 1}`)}</strong>
              <label class="mini-switch" title="${esc(t(page.disabled ? "pageDisabledHint" : "pageEnabledHint"))}"><input type="checkbox" data-page-enable="${index}" ${page.disabled ? "" : "checked"}><span class="visually-hidden">${esc(t("active"))}</span></label>
            </div>
            <span class="library-url">${esc(pageHost(page))}</span>
            <div class="tag-row">
              ${page.disabled ? `<span class="tag muted-tag">${esc(t("disabled"))}</span>` : ""}
              ${uses.length ? uses.map((use) => `<span class="tag tag-${use.kind}">${esc(use.text)}</span>`).join("") : `<span class="tag muted-tag">${esc(t("notScheduled"))}</span>`}
              ${page.preload ? `<span class="tag">${esc(t("preloaded"))}</span>` : ""}
              ${Number(page.display_options?.brightness ?? 100) !== 100 ? `<span class="tag">${esc(t("brightness"))} ${esc(String(page.display_options.brightness))} %</span>` : ""}
            </div>
            <div class="library-actions">
              <button class="primary-ghost" data-page-show-id="${esc(page.page_id)}" data-busy="page-show-${esc(page.page_id)}" ${page.disabled ? "disabled" : ""} title="${esc(t("showNowHint"))}">${esc(t("showNow"))}</button>
              <button data-page-edit="${index}">${esc(t("edit"))}</button>
              <button class="icon-button" data-page-check-one="${index}" title="${esc(t("checkPage"))}" aria-label="${esc(t("checkPage"))}">${icon("check", 16)}</button>
              <button class="icon-button" data-page-duplicate="${index}" title="${esc(t("duplicate"))}" aria-label="${esc(t("duplicate"))}">${icon("copy", 16)}</button>
              <button class="icon-button" data-page-open="${index}" title="${esc(t("openInNewTab"))}" aria-label="${esc(t("openInNewTab"))}">${icon("link", 16)}</button>
              <button class="icon-button danger-ghost" data-page-remove="${index}" title="${esc(t("remove"))}" aria-label="${esc(t("remove"))}">${icon("close", 16)}</button>
            </div>
          </div>
        </article>`;
      }).join("")}</section>
      ${visible ? "" : `<div class="empty"><strong>${esc(t("noMatchingPages"))}</strong></div>`}` : `<div class="empty"><strong>${esc(t("noPagesConfigured"))}</strong><span>${esc(t("addFirstPageHint"))}</span></div>`}
      <details class="card disclosure advanced-settings">
        <summary>${esc(t("pagesAdvanced"))}</summary>
        <div class="disclosure-body advanced-sequence-grid">
          <div class="actions">
            ${button("importPages", "pages-import")}
            ${button("exportPages", "pages-export")}
            ${button("enableAllPages", "page-enable-all")}
            ${button("disableAllPages", "page-disable-all")}
          </div>
          <input id="pages-import-file" type="file" accept="application/json,.json" class="hidden" />
        </div>
      </details>
      <div class="save-bar">
        <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
        <div class="actions">${button("save", "kiosk-save")}${button("saveStartKiosk", "kiosk-save-restart", "primary")}</div>
      </div>
    </div>`;
}

async function checkOne(page) {
  state.pageChecks = { ...(state.pageChecks || {}), [page.page_id]: { pending: true } };
  renderApp();
  let entry;
  try {
    const result = await postJSON("/api/browser/check-page", { index: 0, url: page.url });
    const label = result.ok ? `${t("reachable")} · ${result.statusCode || ""}`.trim() : result.statusCode ? `HTTP ${result.statusCode}` : t("unreachable");
    entry = { ok: !!result.ok, label, detail: result.statusCode === 403 ? t("haForbiddenHint") : result.hint || result.error || result.status || "" };
  } catch (error) {
    entry = { ok: false, label: t("unreachable"), detail: error.message };
  }
  state.pageChecks = { ...(state.pageChecks || {}), [page.page_id]: entry };
  renderApp();
  return entry;
}

export function bindPages() {
  document.querySelectorAll("[data-page-new]").forEach((button) => button.addEventListener("click", () => openPageWizard(null, collectPages().length, button.dataset.pageNew)));
  document.getElementById("page-filter")?.addEventListener("input", (event) => {
    state.pageFilter = event.target.value || "";
    renderApp();
  });
  document.querySelectorAll("[data-page-kind]").forEach((button) => button.addEventListener("click", () => {
    state.pageKind = button.dataset.pageKind;
    renderApp();
  }));
  document.querySelectorAll("[data-page-show-id]").forEach((button) => button.addEventListener("click", () => showPageNow(button.dataset.pageShowId, Number(state.overrideMinutes || 60), `page-show-${button.dataset.pageShowId}`)));
  document.querySelectorAll("[data-page-open]").forEach((button) => button.addEventListener("click", () => {
    const page = collectPages()[Number(button.dataset.pageOpen)];
    if (page?.url) window.open(page.url, "_blank", "noopener");
  }));
  document.querySelectorAll("[data-page-check-one]").forEach((button) => button.addEventListener("click", () => {
    const page = collectPages()[Number(button.dataset.pageCheckOne)];
    if (page?.url) checkOne(page);
  }));
  document.querySelector('[data-action="page-check-all-cards"]')?.addEventListener("click", () => runAction("page-check-all", async () => {
    for (const page of collectPages()) {
      if (page.url && !page.disabled) await checkOne(page);
    }
  }, t("checkAllPages")));
  document.querySelectorAll("[data-page-enable]").forEach((input) => input.addEventListener("change", () => {
    const cfg = cloneConfig();
    cfg.kiosk = cfg.kiosk || {};
    cfg.kiosk.pages = collectPages();
    const page = cfg.kiosk.pages[Number(input.dataset.pageEnable)];
    if (page) page.disabled = !input.checked;
    synchronizeKioskWorkflow(cfg.kiosk);
    state.config = cfg;
    markDirty();
    renderApp();
  }));
}
