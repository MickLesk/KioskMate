// Programme: the base programme (page sequence with durations and time
// windows) plus inserts that interleave other pages on a fixed rhythm, an
// hour preview and the order that decides overlaps.
import { showPageNow, synchronizeKioskWorkflow } from "./actions-kiosk.js";
import { checked, cloneConfig, closeModal, offerUndo, thumbnailImg, esc, field, formatDuration, isDirty, markDirty, openModal, postJSON, renderApp, renderAppIfIdle, secondsToDuration, selectHtml, state, switchHtml, t, toast, val } from "./app-core.js";
import { icon } from "./icons.js";
import { button, collectPages, isLocalSource, normalizePages, sourceMark, stateBanner, validationError } from "./renderers.js";
import { clock, insertTone as toneOf, renderTimeline } from "./timeline.js";
import { clockField, describeClock, isScheduleClock, isSunClock, readClock, resolveClock } from "./clock.js";
import { displayModeLabel, localPageSummary } from "./view-kiosk.js";

const DAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];
const INTERVALS = [2, 3, 5, 10, 15, 20, 30, 45, 60, 90, 120];
const DURATIONS = [10, 15, 20, 30, 45, 60, 90, 120, 180, 300, 600];

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

function insertTone(id) {
  return toneOf(id, inserts());
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
    rhythm = t("insertAtTimes").replace("{times}", (insert.times || []).map((value) => describeClock(value, false)).join(", ") || "--:--");
  } else {
    rhythm = t("insertEvery").replace("{minutes}", insert.every_minutes || 5);
  }
  const parts = [t("insertRhythmFor").replace("{rhythm}", rhythm).replace("{duration}", duration)];
  if (insert.start && insert.end) parts.push(t("insertWindow").replace("{start}", describeClock(insert.start, false)).replace("{end}", describeClock(insert.end, false)));
  if (insert.days?.length) parts.push(dayList(insert.days));
  if (insert.condition?.topic) parts.push(t(insert.condition.negate ? "insertOnlyWhenNot" : "insertOnlyWhen").replace("{topic}", insert.condition.topic).replace("{value}", insert.condition.value || ""));
  return parts.join(" · ");
}

function priorityLabel(priority) {
  return t(["priorityNormal", "priorityHigh", "priorityHighest"][Math.max(0, Math.min(2, Number(priority || 0)))]);
}

const OVERRIDE_MINUTES = [15, 30, 60, 120, 240, 480];
const RANGES = [[60, "range1h"], [360, "range6h"], [1440, "range24h"]];

// Pages that make up the base programme: rotation pages and time windows.
// Insert-only and trigger pages are listed separately.
function isBasePage(page) {
  return !page.disabled && page.url && ["duration", "schedule"].includes(page.display_mode || "duration");
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

function minutesOf(value) {
  return resolveClock(value) ?? 0;
}

// A 24-hour strip with the page's time window, for schedule pages.
function windowStrip(page) {
  const start = minutesOf(page.schedule?.start);
  const end = minutesOf(page.schedule?.end);
  const parts = start <= end ? [[start, end]] : [[start, 1440], [0, end]];
  return `<span class="window-strip" aria-hidden="true">${parts.map(([a, b]) => `<i data-left="${(a / 14.4).toFixed(2)}" data-width="${((b - a) / 14.4).toFixed(2)}"></i>`).join("")}</span>`;
}

function baseModeLabel(page) {
  if (page.display_mode === "schedule") {
    const days = page.schedule?.days?.length ? ` · ${dayList(page.schedule.days)}` : "";
    return `${t("timeWindow")} ${describeClock(page.schedule?.start, false)}–${describeClock(page.schedule?.end, false)}${days}`;
  }
  return `${t("rotationShort")} · ${formatDuration(Number(page.duration_seconds || 3600))}`;
}

const ROTATION_DURATIONS = [15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200];

// Duration or window edited right in the row; sun-based windows keep the
// label and are edited in the dialog.
function inlineTiming(page, index) {
  if (page.display_mode === "schedule") {
    const start = page.schedule?.start || "";
    const end = page.schedule?.end || "";
    if (isSunClock(start) || isSunClock(end)) return `<span class="mode-chip window">${esc(baseModeLabel(page))}</span>`;
    const days = page.schedule?.days?.length ? `<small>${esc(dayList(page.schedule.days))}</small>` : "";
    return `<span class="inline-window mode-chip window"><span>${esc(t("timeWindow"))}</span><input type="time" data-inline-start="${index}" value="${esc(start)}" aria-label="${esc(t("start"))}" data-no-dirty><span aria-hidden="true">–</span><input type="time" data-inline-end="${index}" value="${esc(end)}" aria-label="${esc(t("end"))}" data-no-dirty>${days}</span>`;
  }
  const current = Number(page.duration_seconds || 3600);
  return `<label class="inline-duration mode-chip rotation"><span>${esc(t("rotationShort"))}</span><select data-inline-duration="${index}" aria-label="${esc(t("durationSeconds"))}" data-no-dirty>${optionList(ROTATION_DURATIONS, current, (value) => formatDuration(value)).map(([value, text]) => `<option value="${value}" ${value === current ? "selected" : ""}>${esc(text)}</option>`).join("")}</select></label>`;
}

function nextRunOf(insertID) {
  const value = state.timeline?.next?.[insertID];
  if (!value) return "";
  const at = new Date(value);
  if (at.getTime() <= Date.now() + 1000) return t("now");
  const tomorrow = at.toDateString() !== new Date().toDateString();
  return tomorrow ? `${t("tomorrow")} ${clock(at)}` : clock(at);
}

export function renderProgram() {
  const kiosk = state.config?.kiosk || {};
  const browser = state.status?.browser || {};
  const list = pages();
  const override = browser.override || {};
  const items = inserts();
  const base = list.map((page, index) => ({ page, index })).filter(({ page }) => isBasePage(page));
  const others = list.map((page, index) => ({ page, index })).filter(({ page }) => !isBasePage(page));
  const activeID = browser.running ? (state.persistedConfig?.kiosk?.pages || []).filter((page) => !page.disabled && page.url)[Number(browser.active ?? -1)]?.page_id : "";
  const range = Number(state.programRange || 60);
  const draftTimeline = renderTimeline({ timeline: state.timeline, kiosk, pageName });
  return `
    <div class="page-stack program-view">
      ${!kiosk.scheduler?.enabled && (kiosk.time_rules || []).length ? stateBanner("warn", t("scheduler"), t("scheduleNeedsEnable"), "") : ""}
      <section class="program-hero">
        <div class="program-hero-now">
          <span class="eyebrow">${esc(t("nowOnScreen"))}</span>
          <strong>${esc(browser.running ? browser.page_name || "-" : t("displayStopped"))}</strong>
          <span>${esc(programReason(browser.scheduler))}${browser.scheduler?.next_switch ? ` · ${esc(t("nextSwitch"))} ${esc(clock(browser.scheduler.next_switch))}` : ""}</span>
          ${override.active ? `<div class="override-note">${icon("clock", 14)}<span>${esc(t("overrideUntil").replace("{time}", clock(override.until)))}</span><button class="link-button" data-busy="override-clear" data-action="override-clear">${esc(t("backToProgramme"))}</button></div>` : ""}
        </div>
        <div class="program-hero-controls">
          ${switchHtml("scheduler-enabled", t("runAutomatically"), !!kiosk.scheduler?.enabled)}
          <label class="inline-select"><span>${esc(t("showNowFor"))}</span><select id="override-minutes" data-no-dirty>${OVERRIDE_MINUTES.map((value) => `<option value="${value}" ${Number(state.overrideMinutes || 60) === value ? "selected" : ""}>${esc(formatDuration(value * 60))}</option>`).join("")}</select></label>
        </div>
      </section>

      <section class="card program-base" aria-labelledby="program-base-title">
        <div class="head">
          <div><h3 id="program-base-title">${esc(t("baseProgramme"))}</h3><span class="section-kicker">${esc(t("baseProgrammeHint"))}</span></div>
          <button class="primary" data-action="page-wizard-new">${icon("plus", 16)} ${esc(t("newPage"))}</button>
        </div>
        <div class="body">
          ${base.length ? `<ol class="base-list">${base.map(({ page, index }, position) => `
            <li class="base-row ${page.display_mode === "schedule" ? "is-window" : "is-rotation"} ${page.page_id === activeID ? "on-screen" : ""}" draggable="true" data-sequence-index="${index}">
              <span class="drag-handle" title="${esc(t("dragToReorder"))}" aria-hidden="true">${icon("grip", 16)}</span>
              <span class="step-number">${position + 1}</span>
              ${thumbnailImg(page.page_id, "base-thumb") || `<span class="source-mark">${esc(sourceMark(page.source_type))}</span>`}
              <div class="base-copy">
                <strong>${esc(page.name || page.url)}${page.page_id === activeID ? ` <span class="live-badge">${esc(t("onScreenNow"))}</span>` : ""}</strong>
                <span>${esc(pageHost(page))}</span>
              </div>
              <div class="base-mode">
                ${inlineTiming(page, index)}
                ${page.display_mode === "schedule" ? windowStrip(page) : ""}
              </div>
              <div class="base-actions">
                <button data-page-show-id="${esc(page.page_id)}" data-busy="page-show-${esc(page.page_id)}">${esc(t("showNow"))}</button>
                <button data-page-edit="${index}">${esc(t("edit"))}</button>
                <button class="icon-button" data-base-move="${index}" data-direction="-1" ${position === 0 ? "disabled" : ""} title="${esc(t("moveUp"))}" aria-label="${esc(t("moveUp"))}">${icon("up", 16)}</button>
                <button class="icon-button" data-base-move="${index}" data-direction="1" ${position === base.length - 1 ? "disabled" : ""} title="${esc(t("moveDown"))}" aria-label="${esc(t("moveDown"))}">${icon("down", 16)}</button>
                <button class="icon-button danger-ghost" data-page-remove="${index}" title="${esc(t("remove"))}" aria-label="${esc(t("remove"))}">${icon("close", 16)}</button>
              </div>
            </li>`).join("")}</ol>` : `<div class="empty"><strong>${esc(t("baseEmpty"))}</strong><span>${esc(t("baseEmptyHint"))}</span></div>`}
          <p class="base-explainer">${esc(t("baseExplainer"))}</p>
          ${others.length ? `<div class="other-pages">
            <h4>${esc(t("otherPages"))}</h4>
            <ul>${others.map(({ page, index }) => `<li>
              <span class="source-mark">${esc(sourceMark(page.source_type))}</span>
              <div><strong>${esc(page.name || page.url)}</strong><span>${esc(page.disabled ? t("disabled") : displayModeLabel(page.display_mode))}${usedByInserts(page.page_id)}</span></div>
              <div class="actions">
                ${page.disabled ? "" : `<button data-page-show-id="${esc(page.page_id)}" data-busy="page-show-${esc(page.page_id)}">${esc(t("showNow"))}</button>`}
                <button data-page-tobase="${index}">${esc(t("addToBase"))}</button>
                <button data-page-edit="${index}">${esc(t("edit"))}</button>
              </div>
            </li>`).join("")}</ul>
          </div>` : ""}
        </div>
      </section>

      <section class="card program-inserts" aria-labelledby="program-inserts-title">
        <div class="head">
          <div><h3 id="program-inserts-title">${esc(t("inserts"))}</h3><span class="section-kicker">${esc(t("insertsHint"))}</span></div>
          <button class="primary" data-action="insert-new" ${list.length ? "" : "disabled"}>${icon("plus", 16)} ${esc(t("newInsert"))}</button>
        </div>
        <div class="body">
          ${items.length ? `<div class="insert-list">${items.map((insert, index) => {
            const next = insert.disabled ? "" : nextRunOf(insert.id);
            return `
            <article class="insert-card ${insertTone(insert.id)} ${insert.disabled ? "disabled" : ""}">
              <span class="insert-swatch" aria-hidden="true"></span>
              <div class="insert-copy">
                <strong>${esc(insert.name || pageName(insert.page_id))}</strong>
                <span>${esc(describeInsert(insert))}</span>
                <small>${esc(t("insertShows").replace("{page}", pageName(insert.page_id)))}${Number(insert.priority || 0) > 0 ? ` · <span class="priority-badge">${esc(priorityLabel(insert.priority))}</span>` : ""}</small>
                ${insert.condition && state.timeline?.conditions && insert.id in state.timeline.conditions ? `<span class="condition-chip ${state.timeline.conditions[insert.id] ? "met" : ""}">${icon("link", 12)} ${esc(t(state.timeline.conditions[insert.id] ? "conditionMetNow" : "conditionNotMetNow"))}</span>` : ""}
              </div>
              ${next ? `<span class="insert-next"><small>${esc(t("nextRun"))}</small><strong>${esc(next)}</strong></span>` : ""}
              <div class="actions">
                <label class="inline-check"><input type="checkbox" data-insert-toggle="${index}" ${insert.disabled ? "" : "checked"}><span>${esc(t("active"))}</span></label>
                <button data-insert-edit="${index}">${esc(t("edit"))}</button>
                <button data-insert-duplicate="${index}" title="${esc(t("duplicate"))}">${esc(t("duplicate"))}</button>
                <button class="danger-ghost" data-insert-remove="${index}" title="${esc(t("remove"))}" aria-label="${esc(t("remove"))}">${icon("close", 16)}</button>
              </div>
            </article>`;
          }).join("")}</div>` : `<div class="insert-templates">
            <p class="hint">${esc(t("noInsertsHint"))}</p>
            <div class="template-grid">
              ${[["interval", "templateEvery5", "templateEvery5Hint"], ["hourly", "templateHourly", "templateHourlyHint"], ["times", "templateTimes", "templateTimesHint"]].map(([repeat, title, hint]) => `<button class="template-tile" data-insert-template="${repeat}" ${list.length ? "" : "disabled"}>${icon(repeat === "times" ? "clock" : "repeat", 20)}<strong>${esc(t(title))}</strong><small>${esc(t(hint))}</small></button>`).join("")}
            </div>
          </div>`}
        </div>
      </section>

      <section class="card timeline-card" aria-labelledby="timeline-title">
        <div class="head">
          <div><h3 id="timeline-title">${esc(t("programmeFlow"))}</h3><span class="section-kicker">${esc(state.timeline ? t("flowHint").replace("{percent}", Math.round((1 - draftTimeline.share) * 100)) : t("loading"))}</span></div>
          <div class="segmented compact" role="radiogroup" aria-label="${esc(t("programmeFlow"))}">
            ${RANGES.map(([value, label]) => `<button role="radio" aria-checked="${range === value}" class="${range === value ? "active" : ""}" data-program-range="${value}">${esc(t(label))}</button>`).join("")}
          </div>
        </div>
        <div class="body">${draftTimeline.html}</div>
      </section>

      ${renderPresenceCard(kiosk)}

      <details class="card disclosure priority-card" data-keep-open="programPriority" ${state.openDetails?.has("programPriority") ? "open" : ""}>
        <summary>${esc(t("priorityTitle"))}</summary>
        <div class="disclosure-body">
          <p class="hint">${esc(t("priorityHint"))}</p>
          <ol class="priority-ladder">
            <li><strong>${esc(t("priorityStep1"))}</strong><span>${esc(t("priorityStep1Hint"))}</span></li>
            <li><strong>${esc(t("priorityStep2"))}</strong><span>${esc(t("priorityStep2Hint"))}</span></li>
            <li><strong>${esc(t("priorityStep3"))}</strong><span>${esc(t("priorityStep3Hint"))}</span></li>
            <li><strong>${esc(t("priorityStep4"))}</strong><span>${esc(t("priorityStep4Hint"))}</span></li>
          </ol>
        </div>
      </details>

      <details class="card disclosure advanced-settings" data-keep-open="programAdvanced" ${state.openDetails?.has("programAdvanced") ? "open" : ""}>
        <summary>${esc(t("sequenceAdvanced"))}</summary>
        <div class="disclosure-body advanced-sequence-grid">
          <div class="form-grid">
            ${field("scheduler-tick", t("tickInterval"), "number", "", secondsToDuration(kiosk.scheduler?.tick_interval, 15))}
          </div>
          ${renderLocation(kiosk)}
          <div class="actions">
            ${button("renderCheck", "render-check")}
            ${button("checkPage", "page-check")}
          </div>
          <div id="page-check-output" class="action-feedback"></div>
        </div>
      </details>
      <div class="save-bar">
        <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
        <div class="actions">${button("save", "kiosk-save")}${button("saveStartKiosk", "kiosk-save-restart", "primary")}</div>
      </div>
    </div>`;
}

const PRESENCE_TIMEOUTS = [1, 2, 5, 10, 15, 30, 60, 120];

function renderPresenceCard(kiosk) {
  const presence = kiosk.presence || {};
  const scheduler = state.status?.browser?.scheduler || {};
  const live = presence.enabled && state.persistedConfig?.kiosk?.presence?.enabled
    ? (scheduler.presence === "present" ? t("presenceNow").replace("{time}", clock(scheduler.presence_until)) : t("presenceNobody"))
    : "";
  const open = state.openDetails?.has("programPresence") || (presence.enabled && !state.openDetails?.has("programPresence:closed"));
  return `<details class="card disclosure presence-card" data-keep-open="programPresence" ${open ? "open" : ""}>
    <summary>${esc(t("presenceTitle"))} <span class="mode-chip ${presence.enabled ? "rotation" : ""}">${esc(t(presence.enabled ? "active" : "disabled"))}</span></summary>
    <div class="disclosure-body">
      <p class="hint">${esc(t("presenceHint"))}</p>
      ${switchHtml("presence-enabled", t("presenceEnable"), !!presence.enabled)}
      <div class="presence-fields" ${presence.enabled ? "" : "hidden"}>
        <div class="form-grid">
          ${selectHtml("presence-mode", t("presenceMode"), presence.mode || "wake", [["wake", t("presenceModeWake")], ["require", t("presenceModeRequire")]])}
          ${selectHtml("presence-timeout", t("presenceTimeout"), presence.timeout_minutes || 5, PRESENCE_TIMEOUTS.map((value) => [value, formatDuration(value * 60)]))}
          ${field("presence-topic", t("presenceTopic"), "text", "", presence.topic || "", "zigbee2mqtt/flur_bewegung/occupancy")}
          ${field("presence-value", t("presenceValue"), "text", "", presence.value || "", t("presenceValuePlaceholder"))}
        </div>
        ${switchHtml("presence-touch", t("presenceTouch"), !!presence.touch)}
        <p class="hint">${esc(t(state.config?.mqtt?.enabled ? "presenceSources" : "presenceSourcesNoMQTT"))}</p>
        ${live ? `<p class="presence-live">${icon("touch", 14)} ${esc(live)}</p>` : ""}
      </div>
    </div>
  </details>`;
}

function renderLocation(kiosk) {
  const scheduler = kiosk.scheduler || {};
  const today = (state.timeline?.sun || []).find((day) => day.date === new Date().toLocaleDateString("sv"));
  return `<div class="location-box">
    <h4>${esc(t("locationTitle"))}</h4>
    <p class="hint">${esc(t("locationHint"))}</p>
    <div class="form-grid">
      ${field("location-lat", t("latitude"), "number", "", scheduler.latitude || "", "52.52")}
      ${field("location-lon", t("longitude"), "number", "", scheduler.longitude || "", "13.40")}
    </div>
    <div class="actions"><button data-action="location-detect">${esc(t("locationDetect"))}</button>${today && state.timeline?.location ? `<span class="hint">${esc(t("sunToday").replace("{sunrise}", today.sunrise).replace("{sunset}", today.sunset))}</span>` : ""}</div>
  </div>`;
}

// Presence and location edit the draft config directly.
function bindPresenceAndLocation() {
  const update = (change) => {
    const cfg = cloneConfig();
    cfg.kiosk = cfg.kiosk || {};
    change(cfg.kiosk);
    state.config = cfg;
    markDirty();
  };
  const readPresence = () => update((kiosk) => {
    kiosk.presence = {
      enabled: checked("presence-enabled"),
      mode: val("presence-mode") || "wake",
      timeout_minutes: Number(val("presence-timeout") || 5),
      topic: val("presence-topic").trim(),
      value: val("presence-value").trim(),
      touch: checked("presence-touch"),
    };
  });
  ["presence-mode", "presence-timeout", "presence-topic", "presence-value", "presence-touch"].forEach((id) => document.getElementById(id)?.addEventListener("change", readPresence));
  document.getElementById("presence-enabled")?.addEventListener("change", (event) => {
    const fields = document.querySelector(".presence-fields");
    if (fields) fields.hidden = !event.target.checked;
    readPresence();
  });
  const readLocation = () => {
    const lat = Number(val("location-lat"));
    const lon = Number(val("location-lon"));
    if (val("location-lat") && (!Number.isFinite(lat) || lat < -90 || lat > 90)) return validationError("location-lat", t("validationLatitude"));
    if (val("location-lon") && (!Number.isFinite(lon) || lon < -180 || lon > 180)) return validationError("location-lon", t("validationLongitude"));
    update((kiosk) => {
      kiosk.scheduler = { ...(kiosk.scheduler || {}), latitude: Math.round((lat || 0) * 10000) / 10000, longitude: Math.round((lon || 0) * 10000) / 10000 };
    });
    refreshDraftTimeline();
  };
  const safeLocation = () => { try { readLocation(); } catch (error) { toast(t("validationFailed"), error.message, "error"); } };
  ["location-lat", "location-lon"].forEach((id) => document.getElementById(id)?.addEventListener("change", safeLocation));
  document.querySelector('[data-action="location-detect"]')?.addEventListener("click", () => {
    if (!navigator.geolocation) return toast(t("failed"), t("locationUnavailable"), "warn");
    navigator.geolocation.getCurrentPosition((position) => {
      const lat = document.getElementById("location-lat");
      const lon = document.getElementById("location-lon");
      if (lat) lat.value = position.coords.latitude.toFixed(4);
      if (lon) lon.value = position.coords.longitude.toFixed(4);
      safeLocation();
    }, () => toast(t("failed"), t("locationUnavailable"), "warn"), { timeout: 10000 });
  });
}

function usedByInserts(pageID) {
  const count = inserts().filter((insert) => insert.page_id === pageID && !insert.disabled).length;
  if (!count) return "";
  return ` · ${count === 1 ? t("usedByOneInsert") : t("usedByInserts").replace("{count}", count)}`;
}

function programReason(scheduler) {
  if (!scheduler?.reason) return t("manualPageControl");
  if (scheduler.reason === "no active time rule" && scheduler.presence === "present") return t("reasonPresence");
  if (scheduler.reason === "insert") return t("reasonInsertNamed").replace("{name}", scheduler.active_insert || "");
  if (scheduler.reason === "time") return t("reasonTimeWindow").replace("{name}", scheduler.active_rule || "");
  if (scheduler.reason === "rotation") return t("rotationMode");
  if (scheduler.reason === "manual override") return t("temporaryOverride");
  if (scheduler.reason === "disabled") return t("manualPageControl");
  if (scheduler.reason === "no active time rule") return t("noActiveRule");
  return scheduler.reason;
}

// Recomputes the timeline from the unsaved inserts and pages.
export async function refreshDraftTimeline() {
  try {
    const kiosk = state.config?.kiosk || {};
    state.timeline = await postJSON("/api/program/timeline", { pages: kiosk.pages || [], inserts: kiosk.inserts || [], scheduler: kiosk.scheduler || {}, minutes: Number(state.programRange || 60) });
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

function updatePages(change) {
  const cfg = cloneConfig();
  cfg.kiosk = cfg.kiosk || {};
  cfg.kiosk.pages = collectPages();
  change(cfg.kiosk);
  synchronizeKioskWorkflow(cfg.kiosk);
  state.config = cfg;
  markDirty();
  renderApp();
  refreshDraftTimeline();
}

export function bindProgram() {
  bindPresenceAndLocation();
  // Disclosures stay open across re-renders.
  document.querySelectorAll(".program-view [data-keep-open]").forEach((details) => details.addEventListener("toggle", () => {
    state.openDetails = state.openDetails || new Set();
    const key = details.dataset.keepOpen;
    state.openDetails.delete(key);
    state.openDetails.delete(`${key}:closed`);
    state.openDetails.add(details.open ? key : `${key}:closed`);
  }));
  document.querySelector('[data-action="insert-new"]')?.addEventListener("click", () => openInsertDialog(null));
  document.querySelectorAll("[data-insert-template]").forEach((button) => button.addEventListener("click", () => openInsertDialog(null, button.dataset.insertTemplate)));
  document.querySelectorAll("[data-insert-duplicate]").forEach((button) => button.addEventListener("click", () => {
    const source = inserts()[Number(button.dataset.insertDuplicate)];
    if (!source) return;
    updateInserts((kiosk) => kiosk.inserts.splice(Number(button.dataset.insertDuplicate) + 1, 0, { ...source, id: newInsertID(), name: source.name ? `${source.name} ${t("copy")}` : "", disabled: true }));
  }));
  document.querySelectorAll("[data-page-show-id]").forEach((button) => button.addEventListener("click", () => showPageNow(button.dataset.pageShowId, Number(state.overrideMinutes || 60), `page-show-${button.dataset.pageShowId}`)));
  document.getElementById("override-minutes")?.addEventListener("change", (event) => {
    state.overrideMinutes = Number(event.target.value) || 60;
    try { localStorage.setItem("kioskmate.overrideMinutes", String(state.overrideMinutes)); } catch {}
  });
  document.querySelectorAll("[data-program-range]").forEach((button) => button.addEventListener("click", () => {
    state.programRange = Number(button.dataset.programRange) || 60;
    try { localStorage.setItem("kioskmate.programRange", String(state.programRange)); } catch {}
    refreshDraftTimeline();
  }));
  // Arrows move a page past its neighbour in the base programme; pages that
  // are not part of it keep their place.
  document.querySelectorAll("[data-base-move]").forEach((button) => button.addEventListener("click", () => {
    const from = Number(button.dataset.baseMove);
    const direction = Number(button.dataset.direction);
    updatePages((kiosk) => {
      const base = kiosk.pages.map((page, index) => (isBasePage(page) ? index : -1)).filter((index) => index >= 0);
      const target = base[base.indexOf(from) + direction];
      if (target === undefined) return;
      const [page] = kiosk.pages.splice(from, 1);
      kiosk.pages.splice(target, 0, page);
    });
  }));
  document.querySelectorAll("[data-page-tobase]").forEach((button) => button.addEventListener("click", () => {
    const index = Number(button.dataset.pageTobase);
    updatePages((kiosk) => {
      const page = kiosk.pages[index];
      if (!page) return;
      page.disabled = false;
      page.display_mode = "duration";
      page.duration_seconds = Number(page.duration_seconds || 300);
    });
  }));
  document.querySelectorAll("[data-inline-duration]").forEach((select) => select.addEventListener("change", () => {
    const index = Number(select.dataset.inlineDuration);
    updatePages((kiosk) => { if (kiosk.pages[index]) kiosk.pages[index].duration_seconds = Number(select.value); });
  }));
  document.querySelectorAll("[data-inline-start], [data-inline-end]").forEach((input) => input.addEventListener("change", () => {
    const index = Number(input.dataset.inlineStart ?? input.dataset.inlineEnd);
    if (!/^\d{2}:\d{2}$/.test(input.value)) return;
    updatePages((kiosk) => {
      const page = kiosk.pages[index];
      if (!page) return;
      page.schedule = { ...(page.schedule || {}), [input.dataset.inlineStart !== undefined ? "start" : "end"]: input.value };
    });
  }));
  document.querySelectorAll("[data-insert-edit]").forEach((button) => button.addEventListener("click", () => openInsertDialog(Number(button.dataset.insertEdit))));
  document.querySelectorAll("[data-insert-remove]").forEach((button) => button.addEventListener("click", () => {
    const index = Number(button.dataset.insertRemove);
    const before = cloneConfig();
    const removed = inserts()[index];
    updateInserts((kiosk) => kiosk.inserts.splice(index, 1));
    offerUndo(before, t("insertRemoved"), removed?.name || pageName(removed?.page_id), refreshDraftTimeline);
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

const TEMPLATES = {
  interval: { repeat: "interval", every_minutes: 5, duration_seconds: 30 },
  hourly: { repeat: "hourly", minute: 0, duration_seconds: 60 },
  times: { repeat: "times", times: ["08:00", "12:00", "18:00"], duration_seconds: 120 },
};

function openInsertDialog(index, template = "interval") {
  const list = pages().filter((page) => !page.disabled && page.url);
  const editing = index !== null ? inserts()[index] : null;
  const fallbackPage = list.find((page) => page.display_mode === "insert") || list[1] || list[0] || {};
  const insert = editing ? { ...editing } : { id: newInsertID(), page_id: fallbackPage.page_id, repeat: "interval", every_minutes: 5, minute: 0, times: [], duration_seconds: 30, start: "", end: "", days: [], priority: 0, ...(TEMPLATES[template] || {}) };
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
          ${clockField("insert-start", t("start"), insert.start || "07:00", "07:00")}
          ${clockField("insert-end", t("end"), insert.end || "23:00", "23:00")}
        </div>
        <fieldset class="day-picker"><legend>${esc(t("days"))} <small class="hint">${esc(t("insertDaysHint"))}</small></legend><div>${DAYS.map((day) => `<label><input id="insert-day-${day}" type="checkbox" ${(insert.days || []).includes(day) ? "checked" : ""}><span>${esc(t(`dayShort_${day}`))}</span></label>`).join("")}</div></fieldset>
        <div class="form-grid">
          ${selectHtml("insert-priority", t("insertPriority"), Number(insert.priority || 0), [[0, t("priorityNormal")], [1, t("priorityHigh")], [2, t("priorityHighest")]])}
        </div>
        ${switchHtml("insert-condition", t("insertConditionToggle"), !!insert.condition)}
        <div class="condition-box" data-condition ${insert.condition ? "" : "hidden"}>
          <p class="hint">${esc(t(state.config?.mqtt?.enabled ? "insertConditionHint" : "insertConditionNeedsMQTT"))}</p>
          <div class="form-grid">
            ${field("insert-condition-topic", t("mqttTopic"), "text", "", insert.condition?.topic || "", "zigbee2mqtt/flur_bewegung/occupancy")}
            ${selectHtml("insert-condition-negate", t("insertConditionOperator"), insert.condition?.negate ? "1" : "0", [["0", t("conditionEquals")], ["1", t("conditionNotEquals")]])}
            ${field("insert-condition-value", t("insertConditionValue"), "text", "", insert.condition?.value || "ON", "ON")}
          </div>
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
  document.getElementById("insert-condition")?.addEventListener("change", (event) => {
    const group = form?.querySelector("[data-condition]");
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
    next.start = readClock("insert-start");
    next.end = readClock("insert-end");
  }
  if (checked("insert-condition")) {
    next.condition = { topic: val("insert-condition-topic").trim(), value: val("insert-condition-value").trim(), negate: val("insert-condition-negate") === "1" };
  }
  if (!next.days.length) delete next.days;
  if (!validate) return next;
  if (!next.page_id) validationError("insert-page", t("validationInsertPage"));
  if (repeat === "hourly" && (next.minute < 0 || next.minute > 59)) validationError("insert-minute", t("validationInsertMinute"));
  if (repeat === "times" && (!next.times.length || next.times.some((item) => !isScheduleClock(item)))) validationError("insert-times", t("validationInsertTimes"));
  if (repeat === "interval" && next.duration_seconds >= next.every_minutes * 60) validationError("insert-duration", t("validationInsertDuration"));
  if (windowOn && (!next.start || !next.end)) validationError("insert-start", t("validationTime"));
  if (next.condition && (!next.condition.topic || /[#+]/.test(next.condition.topic))) validationError("insert-condition-topic", t("validationConditionTopic"));
  if (next.condition && !next.condition.value) validationError("insert-condition-value", t("validationConditionValue"));
  return next;
}

function normalizeTime(value) {
  if (isSunClock(value)) return value.toLowerCase().replace(/\s+/g, "");
  const match = /^(\d{1,2})[:.](\d{2})$/.exec(value);
  if (!match) return value;
  return `${match[1].padStart(2, "0")}:${match[2]}`;
}
