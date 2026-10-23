// Programme: the base programme (page sequence with durations and time
// windows) plus inserts that interleave other pages on a fixed rhythm, an
// hour preview and the order that decides overlaps.
import { synchronizeKioskWorkflow } from "./actions-kiosk.js";
import { checked, cloneConfig, closeModal, esc, field, formatDate, formatDuration, isDirty, markDirty, openModal, postJSON, renderApp, renderAppIfIdle, secondsToDuration, selectHtml, state, switchHtml, t, toast, val } from "./app-core.js";
import { icon } from "./icons.js";
import { button, collectPages, normalizePages, stateBanner, validationError } from "./renderers.js";
import { renderKioskStorybook, selectedKioskPageIndex } from "./view-kiosk.js";

const DAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];
const INTERVALS = [2, 3, 5, 10, 15, 20, 30, 45, 60, 90, 120];
const DURATIONS = [10, 15, 20, 30, 45, 60, 90, 120, 180, 300, 600];
const INSERT_COLORS = 4;

function inserts() {
  return state.config?.kiosk?.inserts || [];
}

function pages() {
  return normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls);
}

function pageName(pageID) {
  const page = pages().find((item) => item.page_id === pageID);
  return page ? page.name || page.url : t("pageMissing");
}

// Each insert keeps its colour in the cards, the hour view and the legend.
function insertTone(id) {
  const index = inserts().findIndex((item) => item.id === id);
  return `tone-${(index < 0 ? 0 : index) % INSERT_COLORS}`;
}

function dayList(days) {
  if (!days?.length || days.length === 7) return t("everyDay");
  const set = new Set(days);
  if (days.length === 5 && ["mon", "tue", "wed", "thu", "fri"].every((day) => set.has(day))) return t("weekdays");
  if (days.length === 2 && set.has("sat") && set.has("sun")) return t("weekend");
  return DAYS.filter((day) => set.has(day)).map((day) => t(`dayShort_${day}`)).join(", ");
}

// "Every 5 minutes for 30 s, 07:00–23:00, Mon–Fri"
export function describeInsert(insert) {
  const duration = formatDuration(insert.duration_seconds || 30);
  let rhythm;
  if (insert.repeat === "hourly") {
    rhythm = Number(insert.minute || 0) === 0 ? t("insertHourlyFull") : t("insertHourlyAt").replace("{minute}", String(insert.minute).padStart(2, "0"));
  } else if (insert.repeat === "times") {
    rhythm = t("insertAtTimes").replace("{times}", (insert.times || []).join(", ") || "--:--");
  } else {
    rhythm = t("insertEvery").replace("{minutes}", insert.every_minutes || 5);
  }
  const parts = [t("insertRhythmFor").replace("{rhythm}", rhythm).replace("{duration}", duration)];
  if (insert.start && insert.end) parts.push(t("insertWindow").replace("{start}", insert.start).replace("{end}", insert.end));
  if (insert.days?.length) parts.push(dayList(insert.days));
  return parts.join(" · ");
}

function priorityLabel(priority) {
  return t(["priorityNormal", "priorityHigh", "priorityHighest"][Math.max(0, Math.min(2, Number(priority || 0)))]);
}

function clock(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "--:--" : date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

export function renderProgram() {
  const kiosk = state.config?.kiosk || {};
  const browser = state.status?.browser || {};
  const list = pages();
  const selected = selectedKioskPageIndex(list);
  const current = list[selected] || {};
  const override = browser.override || {};
  const items = inserts();
  return `
    <div class="page-stack program-view">
      ${!kiosk.scheduler?.enabled && (kiosk.time_rules || []).length ? stateBanner("warn", t("scheduler"), t("scheduleNeedsEnable"), "") : ""}
      <section class="program-hero">
        <div>
          <span class="eyebrow">${esc(t("nowOnScreen"))}</span>
          <strong>${esc(browser.page_name || "-")}</strong>
          <span>${esc(programReason(browser.scheduler))}${browser.scheduler?.next_switch ? ` · ${esc(t("nextSwitch"))} ${esc(clock(browser.scheduler.next_switch))}` : ""}</span>
        </div>
        ${switchHtml("scheduler-enabled", t("runAutomatically"), !!kiosk.scheduler?.enabled)}
      </section>

      <section class="card program-base" aria-labelledby="program-base-title">
        <div class="head">
          <div><h3 id="program-base-title">${esc(t("baseProgramme"))}</h3><span class="section-kicker">${esc(t("baseProgrammeHint"))}</span></div>
          <button class="primary" data-action="page-wizard-new">${icon("plus", 16)} ${esc(t("newPage"))}</button>
        </div>
        <div class="body">
          ${renderKioskStorybook(list, selected)}
          <div class="selected-page-bar">
            <div>
              <span>${esc(t("selectedPage"))}</span>
              <strong>${esc(current.name || t("noPagesConfigured"))}</strong>
            </div>
            <div class="actions">
              <button data-action="page-activate" ${list.length ? "" : "disabled"} title="${esc(t("activatePageHint"))}">${esc(t("activatePage"))}</button>
              <button data-action="page-wizard-edit" ${list.length ? "" : "disabled"}>${esc(t("editPage"))}</button>
            </div>
          </div>
          <div class="override-bar">
            <div><strong>${esc(t("temporaryOverride"))}</strong><span>${esc(override.active ? `${list[override.page]?.name || override.page + 1} · ${formatDate(override.until)}` : t("temporaryOverrideHint"))}</span></div>
            <label><span>${esc(t("durationMinutes"))}</span><input id="override-duration" type="number" min="1" max="1440" value="60"></label>
            <button data-action="override-apply" ${list.length ? "" : "disabled"}>${esc(t("showSelectedTemporarily"))}</button>
            ${override.active ? `<button data-action="override-clear">${esc(t("endOverride"))}</button>` : ""}
          </div>
        </div>
      </section>

      <section class="card program-inserts" aria-labelledby="program-inserts-title">
        <div class="head">
          <div><h3 id="program-inserts-title">${esc(t("inserts"))}</h3><span class="section-kicker">${esc(t("insertsHint"))}</span></div>
          <button class="primary" data-action="insert-new" ${list.length ? "" : "disabled"}>${icon("plus", 16)} ${esc(t("newInsert"))}</button>
        </div>
        <div class="body">
          ${items.length ? `<div class="insert-list">${items.map((insert, index) => `
            <article class="insert-card ${insertTone(insert.id)} ${insert.disabled ? "disabled" : ""}">
              <span class="insert-swatch" aria-hidden="true"></span>
              <div class="insert-copy">
                <strong>${esc(insert.name || pageName(insert.page_id))}</strong>
                <span>${esc(describeInsert(insert))}</span>
                <small>${esc(t("insertShows").replace("{page}", pageName(insert.page_id)))}${Number(insert.priority || 0) > 0 ? ` · <span class="priority-badge">${esc(priorityLabel(insert.priority))}</span>` : ""}</small>
              </div>
              <div class="actions">
                <label class="inline-check"><input type="checkbox" data-insert-toggle="${index}" ${insert.disabled ? "" : "checked"}><span>${esc(t("active"))}</span></label>
                <button data-insert-edit="${index}">${esc(t("edit"))}</button>
                <button class="danger-ghost" data-insert-remove="${index}" title="${esc(t("remove"))}" aria-label="${esc(t("remove"))}">${icon("close", 16)}</button>
              </div>
            </article>`).join("")}</div>` : `<div class="empty insert-empty"><strong>${esc(t("noInserts"))}</strong><span>${esc(t("noInsertsHint"))}</span></div>`}
        </div>
      </section>

      ${renderHourView()}

      <section class="card priority-card" aria-labelledby="priority-title">
        <div class="head"><div><h3 id="priority-title">${esc(t("priorityTitle"))}</h3><span class="section-kicker">${esc(t("priorityHint"))}</span></div></div>
        <ol class="priority-ladder">
          <li><strong>${esc(t("priorityStep1"))}</strong><span>${esc(t("priorityStep1Hint"))}</span></li>
          <li><strong>${esc(t("priorityStep2"))}</strong><span>${esc(t("priorityStep2Hint"))}</span></li>
          <li><strong>${esc(t("priorityStep3"))}</strong><span>${esc(t("priorityStep3Hint"))}</span></li>
          <li><strong>${esc(t("priorityStep4"))}</strong><span>${esc(t("priorityStep4Hint"))}</span></li>
        </ol>
      </section>

      <details class="card disclosure advanced-settings">
        <summary>${esc(t("sequenceAdvanced"))}</summary>
        <div class="disclosure-body advanced-sequence-grid">
          <div class="form-grid">
            ${field("scheduler-tick", t("tickInterval"), "number", "", secondsToDuration(kiosk.scheduler?.tick_interval, 15))}
          </div>
          <div class="actions">
            ${button("renderCheck", "render-check")}
            ${button("checkPage", "page-check")}
          </div>
          <div id="page-check-output" class="action-feedback">${esc(current.url || "-")}</div>
        </div>
      </details>
      <div class="save-bar">
        <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
        <div class="actions">${button("save", "kiosk-save")}${button("saveStartKiosk", "kiosk-save-restart", "primary")}</div>
      </div>
    </div>`;
}

function programReason(scheduler) {
  if (!scheduler?.reason) return t("manualPageControl");
  if (scheduler.reason === "insert") return t("reasonInsertNamed").replace("{name}", scheduler.active_insert || "");
  if (scheduler.reason === "time") return t("reasonTimeWindow").replace("{name}", scheduler.active_rule || "");
  if (scheduler.reason === "rotation") return t("rotationMode");
  if (scheduler.reason === "manual override") return t("temporaryOverride");
  if (scheduler.reason === "disabled") return t("manualPageControl");
  if (scheduler.reason === "no active time rule") return t("noActiveRule");
  return scheduler.reason;
}

function renderHourView() {
  const timeline = state.timeline;
  const segments = timeline?.segments || [];
  const minutes = Number(timeline?.minutes || 60);
  const from = new Date(timeline?.from || Date.now()).getTime();
  const span = minutes * 60000;
  const blocks = segments.filter((segment) => segment.insert_id).map((segment) => {
    const left = Math.max(0, (new Date(segment.start).getTime() - from) / span * 100);
    const width = Math.max(0.6, (new Date(segment.end).getTime() - new Date(segment.start).getTime()) / span * 100);
    const label = `${clock(segment.start)}–${clock(segment.end)} ${segment.name || pageName(segment.page_id)}`;
    return `<span class="hour-block ${insertTone(segment.insert_id)}" data-left="${left.toFixed(2)}" data-width="${width.toFixed(2)}" title="${esc(label)}"></span>`;
  }).join("");
  const ticks = [0, 10, 20, 30, 40, 50, 60].filter((m) => m <= minutes).map((m) => `<span data-left="${(m / minutes * 100).toFixed(2)}">${esc(clock(from + m * 60000))}</span>`).join("");
  const used = [...new Set(segments.filter((segment) => segment.insert_id).map((segment) => segment.insert_id))];
  const share = segments.filter((segment) => segment.insert_id).reduce((sum, segment) => sum + (new Date(segment.end) - new Date(segment.start)), 0) / span;
  return `<section class="card hour-view" aria-labelledby="hour-title">
    <div class="head">
      <div><h3 id="hour-title">${esc(t("nextHour"))}</h3><span class="section-kicker">${esc(timeline ? t("nextHourHint").replace("{percent}", Math.round((1 - share) * 100)) : t("loading"))}</span></div>
    </div>
    <div class="body">
      <div class="hour-track" role="img" aria-label="${esc(t("nextHour"))}">
        ${blocks}
      </div>
      <div class="hour-ticks" aria-hidden="true">${ticks}</div>
      <div class="hour-legend">
        <span class="legend-item"><i class="hour-legend-base"></i>${esc(t("baseProgramme"))}</span>
        ${used.map((id) => {
          const insert = inserts().find((item) => item.id === id) || {};
          return `<span class="legend-item ${insertTone(id)}"><i></i>${esc(insert.name || pageName(insert.page_id))}</span>`;
        }).join("")}
      </div>
    </div>
  </section>`;
}

// Recomputes the hour view from the unsaved inserts and pages.
export async function refreshDraftTimeline() {
  try {
    const kiosk = state.config?.kiosk || {};
    state.timeline = await postJSON("/api/program/timeline", { pages: kiosk.pages || [], inserts: kiosk.inserts || [], minutes: 60 });
    renderAppIfIdle();
  } catch (error) {
    toast(t("failed"), error.message, "warn");
  }
}

function newInsertID() {
  const data = new Uint8Array(4);
  globalThis.crypto.getRandomValues(data);
  return `insert_${[...data].map((value) => value.toString(16).padStart(2, "0")).join("")}`;
}

function updateInserts(change) {
  const cfg = cloneConfig();
  cfg.kiosk = cfg.kiosk || {};
  cfg.kiosk.pages = collectPages();
  cfg.kiosk.inserts = [...(cfg.kiosk.inserts || [])];
  change(cfg.kiosk);
  synchronizeKioskWorkflow(cfg.kiosk);
  state.config = cfg;
  markDirty();
  renderApp();
  refreshDraftTimeline();
}

export function bindProgram() {
  document.querySelector('[data-action="insert-new"]')?.addEventListener("click", () => openInsertDialog(null));
  document.querySelectorAll("[data-insert-edit]").forEach((button) => button.addEventListener("click", () => openInsertDialog(Number(button.dataset.insertEdit))));
  document.querySelectorAll("[data-insert-remove]").forEach((button) => button.addEventListener("click", () => {
    if (!confirm(t("confirmRemoveInsert"))) return;
    const index = Number(button.dataset.insertRemove);
    updateInserts((kiosk) => kiosk.inserts.splice(index, 1));
  }));
  document.querySelectorAll("[data-insert-toggle]").forEach((input) => input.addEventListener("change", () => {
    const index = Number(input.dataset.insertToggle);
    updateInserts((kiosk) => { kiosk.inserts[index] = { ...kiosk.inserts[index], disabled: !input.checked }; });
  }));
}

function optionList(values, current, format) {
  const list = [...values];
  if (current && !list.includes(Number(current))) list.push(Number(current));
  return list.sort((a, b) => a - b).map((value) => [value, format(value)]);
}

function openInsertDialog(index) {
  const list = pages().filter((page) => !page.disabled && page.url);
  const editing = index !== null ? inserts()[index] : null;
  const fallbackPage = list.find((page) => page.display_mode === "insert") || list[1] || list[0] || {};
  const insert = editing ? { ...editing } : { id: newInsertID(), page_id: fallbackPage.page_id, repeat: "interval", every_minutes: 5, minute: 0, times: [], duration_seconds: 30, start: "", end: "", days: [], priority: 0 };
  const page = list.find((item) => item.page_id === insert.page_id) || fallbackPage;
  const onlyInsert = editing ? page.display_mode === "insert" : suggestOnlyInsert(list, page);
  const repeat = insert.repeat || "interval";
  openModal(`
    <div class="modal insert-dialog" role="dialog" aria-modal="true" aria-labelledby="insert-title">
      <div class="modal-head"><div><h3 id="insert-title">${esc(editing ? t("editInsert") : t("newInsert"))}</h3><div class="hint">${esc(t("insertDialogHint"))}</div></div><button data-modal-close aria-label="${esc(t("close"))}">${icon("close", 18)}</button></div>
      <div class="modal-body insert-form">
        <div class="form-grid">
          ${selectHtml("insert-page", t("insertPage"), page.page_id || "", list.map((item) => [item.page_id, item.name || item.url]))}
          ${field("insert-name", t("insertName"), "text", "", insert.name || "", t("insertNamePlaceholder"))}
        </div>
        <fieldset class="segmented-field">
          <legend>${esc(t("insertRhythm"))}</legend>
          <div class="segmented" role="radiogroup">
            ${["interval", "hourly", "times"].map((value) => `<label class="${repeat === value ? "active" : ""}"><input type="radio" name="insert-repeat" value="${value}" ${repeat === value ? "checked" : ""}><span>${esc(t(`repeat_${value}`))}</span></label>`).join("")}
          </div>
        </fieldset>
        <div class="form-grid">
          <div data-repeat="interval" ${repeat === "interval" ? "" : "hidden"}>${selectHtml("insert-every", t("insertEveryLabel"), insert.every_minutes || 5, optionList(INTERVALS, insert.every_minutes, (value) => `${value} min`))}</div>
          <div data-repeat="hourly" ${repeat === "hourly" ? "" : "hidden"}>${field("insert-minute", t("insertMinuteLabel"), "number", "", insert.minute ?? 0)}</div>
          <div data-repeat="times" ${repeat === "times" ? "" : "hidden"}>${field("insert-times", t("insertTimesLabel"), "text", "", (insert.times || []).join(", "), "08:00, 12:30, 18:00")}</div>
          ${selectHtml("insert-duration", t("insertDurationLabel"), insert.duration_seconds || 30, optionList(DURATIONS, insert.duration_seconds, (value) => formatDuration(value)))}
        </div>
        ${switchHtml("insert-window", t("insertWindowLabel"), !!(insert.start && insert.end))}
        <div class="form-grid" data-window ${insert.start && insert.end ? "" : "hidden"}>
          ${field("insert-start", t("start"), "time", "", insert.start || "07:00")}
          ${field("insert-end", t("end"), "time", "", insert.end || "23:00")}
        </div>
        <fieldset class="day-picker"><legend>${esc(t("days"))} <small class="hint">${esc(t("insertDaysHint"))}</small></legend><div>${DAYS.map((day) => `<label><input id="insert-day-${day}" type="checkbox" ${(insert.days || []).includes(day) ? "checked" : ""}><span>${esc(t(`dayShort_${day}`))}</span></label>`).join("")}</div></fieldset>
        <div class="form-grid">
          ${selectHtml("insert-priority", t("insertPriority"), Number(insert.priority || 0), [[0, t("priorityNormal")], [1, t("priorityHigh")], [2, t("priorityHighest")]])}
        </div>
        <div class="insert-options">
          ${switchHtml("insert-only", t("insertOnlyToggle"), onlyInsert)}
          ${switchHtml("insert-preload", t("preloadPage"), !!page.preload)}
          <small class="hint">${esc(t(state.config?.kiosk?.page_switch === "tabs" ? "insertPreloadHint" : "insertPreloadNeedsTabs"))}</small>
        </div>
        <p class="insert-summary" id="insert-summary" aria-live="polite"></p>
      </div>
      <div class="modal-foot"><button data-modal-close>${esc(t("cancel"))}</button><button class="primary" data-action="insert-commit">${esc(t("apply"))}</button></div>
    </div>`);
  const form = document.querySelector(".insert-dialog");
  const summary = () => {
    const draft = readInsertForm(insert, false);
    const target = document.getElementById("insert-summary");
    if (target) target.textContent = t("insertSummary").replace("{page}", pageName(draft.page_id)).replace("{rhythm}", describeInsert(draft));
  };
  form?.querySelectorAll('input[name="insert-repeat"]').forEach((radio) => radio.addEventListener("change", () => {
    form.querySelectorAll("[data-repeat]").forEach((group) => { group.hidden = group.dataset.repeat !== radio.value; });
    form.querySelectorAll(".segmented label").forEach((label) => label.classList.toggle("active", label.querySelector("input")?.checked));
  }));
  document.getElementById("insert-window")?.addEventListener("change", (event) => {
    const group = form?.querySelector("[data-window]");
    if (group) group.hidden = !event.target.checked;
  });
  document.getElementById("insert-page")?.addEventListener("change", (event) => {
    const chosen = list.find((item) => item.page_id === event.target.value) || {};
    const preload = document.getElementById("insert-preload");
    if (preload) preload.checked = !!chosen.preload;
    const only = document.getElementById("insert-only");
    if (only) only.checked = suggestOnlyInsert(list, chosen);
  });
  form?.addEventListener("input", summary);
  form?.addEventListener("change", summary);
  summary();
  document.querySelector('[data-action="insert-commit"]')?.addEventListener("click", () => {
    try {
      const next = readInsertForm(insert, true);
      const only = checked("insert-only");
      const preload = checked("insert-preload");
      updateInserts((kiosk) => {
        if (index === null) kiosk.inserts.push(next);
        else kiosk.inserts[index] = next;
        const target = kiosk.pages.find((item) => item.page_id === next.page_id);
        if (target) {
          target.preload = preload;
          if (only) target.display_mode = "insert";
          else if (target.display_mode === "insert") target.display_mode = "duration";
        }
      });
      closeModal();
      toast(t("unsavedChanges"), t("insertSaveReminder"), "warn");
    } catch (error) {
      toast(t("validationFailed"), error.message, "error");
    }
  });
}

// A page picked for an insert usually should not also run in the base
// programme, except the first page (the main dashboard) or a time window page.
function suggestOnlyInsert(list, page) {
  if (page.display_mode === "insert") return true;
  return list.length > 1 && page.page_id !== list[0]?.page_id && page.display_mode === "duration";
}

function readInsertForm(base, validate) {
  const repeat = document.querySelector('input[name="insert-repeat"]:checked')?.value || "interval";
  const windowOn = checked("insert-window");
  const next = {
    id: base.id,
    name: val("insert-name").trim(),
    page_id: val("insert-page"),
    repeat,
    duration_seconds: Number(val("insert-duration") || 30),
    priority: Number(val("insert-priority") || 0),
    days: DAYS.filter((day) => checked(`insert-day-${day}`)),
    disabled: !!base.disabled,
  };
  if (repeat === "interval") next.every_minutes = Number(val("insert-every") || 5);
  if (repeat === "hourly") next.minute = Math.round(Number(val("insert-minute") || 0));
  if (repeat === "times") next.times = val("insert-times").split(/[,;\s]+/).map((item) => item.trim()).filter(Boolean).map(normalizeTime);
  if (windowOn) {
    next.start = val("insert-start");
    next.end = val("insert-end");
  }
  if (!next.days.length) delete next.days;
  if (!validate) return next;
  if (!next.page_id) validationError("insert-page", t("validationInsertPage"));
  if (repeat === "hourly" && (next.minute < 0 || next.minute > 59)) validationError("insert-minute", t("validationInsertMinute"));
  if (repeat === "times" && (!next.times.length || next.times.some((item) => !/^\d{2}:\d{2}$/.test(item)))) validationError("insert-times", t("validationInsertTimes"));
  if (repeat === "interval" && next.duration_seconds >= next.every_minutes * 60) validationError("insert-duration", t("validationInsertDuration"));
  if (windowOn && (!next.start || !next.end)) validationError("insert-start", t("validationTime"));
  return next;
}

function normalizeTime(value) {
  const match = /^(\d{1,2})[:.](\d{2})$/.exec(value);
  if (!match) return value;
  return `${match[1].padStart(2, "0")}:${match[2]}`;
}
