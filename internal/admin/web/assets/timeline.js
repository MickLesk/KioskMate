// Programme timeline shared by the overview and the programme view: the base
// programme (time windows, rotation, display off) as one track and the
// inserts from /api/program/timeline on top of it.
import { esc, formatDuration, state, t } from "./app-core.js";

const WEEKDAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"];
const INSERT_COLORS = 4;

export function clock(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "--:--" : date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function clockMinutes(value) {
  const [hour, minute] = String(value || "").split(":").map(Number);
  return Number.isFinite(hour) && Number.isFinite(minute) ? hour * 60 + minute : null;
}

// Each insert keeps its colour everywhere it is drawn.
export function insertTone(id, inserts = state.config?.kiosk?.inserts || []) {
  const index = inserts.findIndex((item) => item.id === id);
  return `tone-${(index < 0 ? 0 : index) % INSERT_COLORS}`;
}

// What the base programme shows at a moment, following the scheduler: the
// first matching time window wins; outside all windows the display goes off
// when a page asks for it (or the mode is time only), otherwise the rotation
// runs.
function baseAt(kiosk, date) {
  const scheduler = kiosk.scheduler || {};
  if (!scheduler.enabled) return { kind: "manual", name: t("manualPageControl") };
  const mode = scheduler.mode || "rotation";
  const rotation = kiosk.rotation || [];
  if (mode === "time" || mode === "hybrid") {
    const weekday = WEEKDAYS[date.getDay()];
    const current = date.getHours() * 60 + date.getMinutes();
    for (const rule of kiosk.time_rules || []) {
      if (rule.disabled || (rule.days?.length && !rule.days.includes(weekday))) continue;
      const start = clockMinutes(rule.start);
      const end = clockMinutes(rule.end);
      if (start === null || end === null) continue;
      const active = start <= end ? current >= start && current < end : current >= start || current < end;
      if (active) return { kind: "window", name: rule.name || `${rule.start}–${rule.end}` };
    }
    const powerOff = (kiosk.pages || []).some((page) => !page.disabled && page.url && (page.display_mode === "schedule" || page.display_options?.power_off_after));
    if (mode === "time" || powerOff || !rotation.length) return { kind: "off", name: t("displayOffPeriod") };
  }
  if (!rotation.length) return { kind: "off", name: t("nothingPlanned") };
  return { kind: "rotation", name: t("rotationMode") };
}

// Base programme runs between from and from+minutes, sampled per minute and
// merged.
export function baseRuns(kiosk, from, minutes) {
  const runs = [];
  for (let minute = 0; minute < minutes; minute++) {
    const start = from + minute * 60000;
    const base = baseAt(kiosk, new Date(start));
    const last = runs[runs.length - 1];
    if (last && last.kind === base.kind && last.name === base.name) last.end = start + 60000;
    else runs.push({ ...base, start, end: start + 60000 });
  }
  return runs;
}

// Insert segments that really show: an insert never wakes a display the
// schedule turned off.
export function visibleInserts(segments, runs) {
  return (segments || []).filter((segment) => {
    if (!segment.insert_id) return false;
    const start = new Date(segment.start).getTime();
    const run = runs.find((item) => start >= item.start && start < item.end);
    return !run || run.kind !== "off";
  });
}

function tickStep(minutes) {
  if (minutes <= 60) return 10;
  if (minutes <= 180) return 30;
  if (minutes <= 360) return 60;
  return 180;
}

// Renders the two-layer track. Options: timeline (API result), kiosk
// (config, possibly a draft), pageName(id) and an optional now marker.
export function renderTimeline({ timeline, kiosk, pageName = (id) => id, showNow = true, compact = false }) {
  const minutes = Number(timeline?.minutes || 60);
  const from = new Date(timeline?.from || Date.now()).getTime();
  const span = minutes * 60000;
  const inserts = kiosk.inserts || [];
  const runs = baseRuns(kiosk, from, minutes);
  const shown = visibleInserts(timeline?.segments, runs);
  const pct = (value) => Math.max(0, Math.min(100, value / span * 100)).toFixed(3);
  const baseBlocks = runs.map((run) => {
    const label = `${clock(run.start)}–${clock(run.end)} ${run.name}`;
    const wide = (run.end - run.start) / span > 0.12;
    return `<span class="tl-base ${run.kind}" data-left="${pct(run.start - from)}" data-width="${pct(run.end - run.start)}" title="${esc(label)}">${wide && !compact ? `<em>${esc(run.name)}</em>` : ""}</span>`;
  }).join("");
  const insertBlocks = shown.map((segment) => {
    const start = new Date(segment.start).getTime();
    const end = new Date(segment.end).getTime();
    const name = segment.name || pageName(segment.page_id);
    const label = `${clock(start)}–${clock(end)} ${name} (${formatDuration(Math.round((end - start) / 1000))})`;
    return `<span class="tl-insert ${insertTone(segment.insert_id, inserts)}" data-left="${pct(start - from)}" data-width="${Math.max(minutes > 360 ? 0.18 : 0.5, (end - start) / span * 100).toFixed(3)}" title="${esc(label)}"></span>`;
  }).join("");
  const step = tickStep(minutes);
  const firstTick = Math.ceil(from / (step * 60000)) * step * 60000;
  const ticks = [];
  for (let at = firstTick; at <= from + span; at += step * 60000) ticks.push(`<span data-left="${pct(at - from)}">${esc(clock(at))}</span>`);
  const now = Date.now();
  const nowMarker = showNow && now >= from && now <= from + span ? `<span class="tl-now" data-left="${pct(now - from)}" title="${esc(clock(now))}"></span>` : "";
  const usedInserts = [...new Set(shown.map((segment) => segment.insert_id))];
  const kinds = [...new Set(runs.map((run) => run.kind))];
  const insertTime = shown.reduce((sum, segment) => sum + (new Date(segment.end) - new Date(segment.start)), 0);
  return {
    share: span ? insertTime / span : 0,
    html: `<div class="tl ${compact ? "compact" : ""}">
      <div class="tl-track" role="img" aria-label="${esc(t("programmeTimeline"))}">${baseBlocks}${insertBlocks}${nowMarker}</div>
      <div class="tl-ticks" aria-hidden="true">${ticks.join("")}</div>
      <div class="tl-legend">
        ${kinds.map((kind) => `<span class="legend-item base-${kind}"><i></i>${esc(t(`timelineKind_${kind}`))}</span>`).join("")}
        ${usedInserts.map((id) => {
          const insert = inserts.find((item) => item.id === id) || {};
          return `<span class="legend-item ${insertTone(id, inserts)}"><i></i>${esc(insert.name || pageName(insert.page_id))}</span>`;
        }).join("")}
      </div>
    </div>`,
  };
}
