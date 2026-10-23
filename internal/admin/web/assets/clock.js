// Schedule times: HH:MM or a sun time ("sunrise", "sunset", optionally with
// an offset in minutes like "sunset-30"). The server resolves them for the
// kiosk's location and sends sunrise/sunset per day with the timeline; until
// then sunrise counts as 06:00 and sunset as 18:00, like on the server.
import { esc, t } from "./app-core.js";

const SUN = /^(sunrise|sunset)\s*(?:([+-])\s*(\d{1,3}))?$/i;
const sunByDate = new Map();

export function parseSun(value) {
  const match = SUN.exec(String(value || "").trim());
  if (!match) return null;
  const offset = match[3] ? Number(match[3]) * (match[2] === "-" ? -1 : 1) : 0;
  return { event: match[1].toLowerCase(), offset };
}

export function isSunClock(value) {
  return !!parseSun(value);
}

export function isScheduleClock(value) {
  return /^\d{2}:\d{2}$/.test(String(value || "")) || isSunClock(value);
}

function dateKey(date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

function hhmm(value) {
  const [hour, minute] = String(value || "").split(":").map(Number);
  return Number.isFinite(hour) && Number.isFinite(minute) ? hour * 60 + minute : null;
}

// Stores the sunrise/sunset list of a timeline response.
export function rememberSun(list) {
  for (const day of list || []) sunByDate.set(day.date, day);
}

// Minute of the day value stands for on date, or null.
export function resolveClock(value, date = new Date()) {
  const plain = hhmm(value);
  if (plain !== null && /^\s*\d{1,2}:\d{2}/.test(String(value))) return plain;
  const sun = parseSun(value);
  if (!sun) return null;
  const day = sunByDate.get(dateKey(date));
  const base = hhmm(day?.[sun.event]) ?? (sun.event === "sunrise" ? 360 : 1080);
  return Math.max(0, Math.min(1439, base + sun.offset));
}

function minuteText(minute) {
  return `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`;
}

// "Sonnenuntergang −30 min (≈ 17:32)" or the plain time.
export function describeClock(value, withTime = true) {
  const sun = parseSun(value);
  if (!sun) return value || "--:--";
  const offset = sun.offset ? ` ${sun.offset > 0 ? "+" : "−"}${Math.abs(sun.offset)} min` : "";
  const minute = resolveClock(value);
  return `${t(sun.event === "sunrise" ? "sunrise" : "sunset")}${offset}${withTime && minute !== null ? ` (≈ ${minuteText(minute)})` : ""}`;
}

// A time input that can switch to sunrise/sunset with an offset.
export function clockField(id, label, value, fallback = "08:00") {
  const sun = parseSun(value);
  const kind = sun ? sun.event : "clock";
  const options = [["clock", t("clockTime")], ["sunrise", t("sunrise")], ["sunset", t("sunset")]];
  return `<div class="clock-field">
    <label for="${esc(id)}-kind">${esc(label)}</label>
    <div class="clock-field-row">
      <select id="${esc(id)}-kind" data-clock-kind="${esc(id)}" aria-label="${esc(label)}">${options.map(([option, text]) => `<option value="${option}" ${option === kind ? "selected" : ""}>${esc(text)}</option>`).join("")}</select>
      <input id="${esc(id)}" type="time" value="${esc(sun ? fallback : value || fallback)}" ${sun ? "hidden" : ""} />
      <span class="clock-offset" data-clock-offset="${esc(id)}" ${sun ? "" : "hidden"}><input id="${esc(id)}-offset" type="number" step="5" min="-360" max="360" value="${sun ? sun.offset : 0}" aria-label="${esc(t("sunOffset"))}" /><small>${esc(t("minutesShort"))}</small></span>
    </div>
  </div>`;
}

// Reads a clockField back as "HH:MM" or "sunset-30".
export function readClock(id) {
  const kind = document.getElementById(`${id}-kind`)?.value || "clock";
  if (kind === "clock") return document.getElementById(id)?.value || "";
  const offset = Math.round(Number(document.getElementById(`${id}-offset`)?.value || 0));
  const clamped = Math.max(-360, Math.min(360, offset));
  return clamped ? `${kind}${clamped > 0 ? "+" : "-"}${Math.abs(clamped)}` : kind;
}

document.addEventListener("change", (event) => {
  const id = event.target?.dataset?.clockKind;
  if (!id) return;
  const sun = event.target.value !== "clock";
  const input = document.getElementById(id);
  const offset = document.querySelector(`[data-clock-offset="${CSS.escape(id)}"]`);
  if (input) input.hidden = sun;
  if (offset) offset.hidden = !sun;
});
