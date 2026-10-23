// Overview: what is on screen now, what comes next, the few controls needed
// every day, and today's programme at a glance. Troubleshooting tools stay
// one click away in a disclosure.
import { formatThemeStatus } from "./actions-core.js";
import { esc, formatDate, formatDuration, formatMQTTState, formatValue, selectHtml, state, t } from "./app-core.js";
import { button, cpuCoresUsed, cpuShare, cpuShareValue, normalizePages, powerBanner, stateBanner, statusTile } from "./renderers.js";
import { healthChecks } from "./view-device.js";

function clock(value) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "--:--" : date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function reasonText(scheduler) {
  const reason = scheduler?.reason;
  if (reason === "insert") return t("reasonInsertNamed").replace("{name}", scheduler.active_insert || "");
  if (reason === "time") return t("reasonTimeWindow").replace("{name}", scheduler.active_rule || "");
  if (reason === "rotation") return t("reasonRotation");
  if (reason === "manual override") return t("temporaryOverride");
  if (reason === "no active time rule") return t("noActiveRule");
  return t("manualPageControl");
}

function insertTone(id) {
  const index = (state.config?.kiosk?.inserts || []).findIndex((item) => item.id === id);
  return `tone-${(index < 0 ? 0 : index) % 4}`;
}

export function renderDashboard() {
  const browser = state.status?.browser || {};
  const browserKnown = !!state.status?.browser;
  const cfg = state.config || {};
  const mqtt = state.status?.mqtt || {};
  const stats = browser.stats || {};
  const hw = state.hardware || {};
  const recovery = browser.recovery || {};
  const pages = normalizePages(cfg.kiosk?.pages, cfg.kiosk?.urls);
  const enabledPages = pages.filter((page) => !page.disabled && page.url);
  const activePage = enabledPages[Number(browser.active || 0)] || {};
  const runningLabel = !browserKnown ? t("loading") : (browser.ready ? t("running") : browser.running ? t("connecting") : t("stopped"));
  const runningTone = !browserKnown ? "" : (browser.ready ? "ok" : browser.running ? "warn" : "bad");
  const schedulerReasonKey = String(browser.scheduler?.reason || "").toLowerCase();
  const hasTimeRules = (cfg.kiosk?.time_rules || []).length > 0;
  const schedulerNeedsAttention = schedulerReasonKey === "disabled" && hasTimeRules;
  const problems = healthChecks().filter((item) => item.level !== "ok" && item.title !== t("checkPower"));
  const displayPower = String(hw.display?.power || "").toUpperCase();
  return `
    <div class="page-stack overview">
      ${renderUpdateNotice()}
      ${powerBanner()}
      ${recovery.state && !["healthy", "idle"].includes(recovery.state) ? stateBanner(recovery.state === "failed" || recovery.state === "auth_blocked" ? "bad" : "warn", t("recoveryNeedsAttention"), recovery.last_result || recovery.reason || recovery.state, button("recoverNow", "browser-auto-recover", "primary")) : ""}
      ${schedulerNeedsAttention ? stateBanner("warn", t("scheduler"), t("schedulerDisabledWithRulesHint"), `<button data-view="program">${esc(t("openProgram"))}</button>`) : ""}
      ${problems.length ? stateBanner(problems.some((item) => item.level === "bad") ? "bad" : "warn", t("healthProblems").replace("{count}", problems.length), problems.map((item) => `${item.title}: ${item.detail}`).join(" · "), `<button data-view="device">${esc(t("openDevice"))}</button>`) : ""}

      <section class="overview-hero">
        <div class="card live-card">
          <div class="preview-shell">
            ${state.snapshotURL ? `<img id="snapshot-image" class="snapshot-image" src="${esc(state.snapshotURL)}" alt="${esc(t("liveView"))}" />` : `<div id="snapshot-empty" class="empty">${esc(browser.running ? t("snapshotOnDemand") : t("liveViewStopped"))}</div>`}
          </div>
          <div class="preview-meta">
            <span>${esc(state.snapshotTime ? formatDate(state.snapshotTime) : t("liveView"))}</span>
            <div class="actions">${button("refreshSnapshot", "dashboard-snapshot-refresh")}${button("openPreview", "dashboard-preview-open")}</div>
          </div>
        </div>
        <div class="card now-card">
          <div class="now-head">
            <span class="eyebrow">${esc(t("nowOnScreen"))}</span>
            <span class="chip ${runningTone}"><span class="dot" aria-hidden="true"></span>${esc(runningLabel)}</span>
          </div>
          <strong class="now-title">${esc(browser.page_name || activePage.name || (browserKnown ? "-" : t("loading")))}</strong>
          <span class="now-reason">${esc(reasonText(browser.scheduler))}${browser.scheduler?.next_switch ? ` · ${esc(t("nextSwitch"))} ${esc(clock(browser.scheduler.next_switch))}` : ""}</span>
          <div class="now-actions">
            <div class="page-picker">
              ${selectHtml("overview-page", t("choosePage"), Number(browser.active || 0), enabledPages.map((page, index) => [index, page.name || page.url]))}
              <button class="primary" data-busy="overview-show" data-action="overview-show" ${enabledPages.length ? "" : "disabled"}>${esc(t("showNow"))}</button>
            </div>
            <div class="action-grid">
              ${browser.running ? button("reloadBrowser", "browser-reload") : button("startBrowser", "browser-start", "primary")}
              ${enabledPages.length > 1 ? button("nextPage", "browser-next") : ""}
              ${hw.support?.display_status ? `<button data-busy="overview-display" data-action="overview-display" data-power="${displayPower === "OFF" ? "ON" : "OFF"}">${esc(t(displayPower === "OFF" ? "displayTurnOn" : "displayTurnOff"))}</button>` : ""}
              ${browser.running ? button("stopBrowser", "browser-stop") : ""}
            </div>
          </div>
          ${renderUpNext(browser)}
        </div>
      </section>

      <section class="status-strip" aria-label="${esc(t("status"))}">
        ${statusTile(t("processorLoad"), cpuShare(stats.cpu_percent), Number(cpuShareValue(stats.cpu_percent) || 0) > 75 ? "warn" : "", `${cpuCoresUsed(stats.cpu_percent)} · ${formatValue(stats.rss_mb, " MB RAM")}`)}
        ${statusTile(t("memory"), formatValue(hw.system?.memory_usage_percent, " %"), Number(hw.system?.memory_usage_percent || 0) > 85 ? "warn" : "", formatValue(hw.system?.memory_size_gib, " GiB"))}
        ${statusTile(t("temperature"), formatValue(hw.system?.processor_temperature_c, " °C"), Number(hw.system?.processor_temperature_c || 0) >= 75 ? "warn" : "", hw.device?.model || "")}
        ${statusTile("MQTT", formatMQTTState(mqtt.state), mqtt.connected ? "ok" : mqtt.state === "auth_error" || mqtt.state === "error" ? "bad" : "", mqtt.last_error || (cfg.mqtt?.enabled ? cfg.mqtt?.url || "" : t("notConfigured")))}
      </section>

      ${renderToday()}

      <details class="card disclosure">
        <summary>${esc(t("troubleshooting"))}</summary>
        <div class="disclosure-body">
          <div class="action-matrix">
            ${button("restartBrowser", "browser-restart")}
            ${button("recoverNow", "browser-auto-recover")}
            ${button("checkPage", "dashboard-page-check")}
            ${button("renderCheck", "dashboard-render-check")}
            ${button("repairHA", "browser-repair-ha")}
            ${button("recoveryWorkflow", "browser-recover")}
            ${button("browserDoctor", "browser-doctor")}
            ${button("diagnostics", "dashboard-diagnostics")}
            ${button("resetSession", "browser-reset-session", "danger-ghost")}
          </div>
          <div id="page-check-output" class="action-feedback" aria-live="polite"></div>
          <dl class="mini-facts">
            <div><dt>${esc(t("haThemeSync"))}</dt><dd>${esc(formatThemeStatus(browser.theme_status))}</dd></div>
            <div><dt>${esc(t("browserGeneration"))}</dt><dd>${esc(String(browser.generation || 0))}</dd></div>
            ${browser.last_error ? `<div><dt>${esc(t("lastError"))}</dt><dd>${esc(browser.last_error)}</dd></div>` : ""}
          </dl>
        </div>
      </details>
    </div>`;
}

function renderUpNext(browser) {
  const upcoming = (state.timeline?.upcoming || []).slice(0, 4);
  const now = Date.now();
  const rows = upcoming.map((segment) => {
    const running = new Date(segment.start).getTime() <= now;
    const seconds = Math.round((new Date(segment.end) - new Date(segment.start)) / 1000);
    return `<li class="${insertTone(segment.insert_id)}"><time>${esc(running ? t("now") : clock(segment.start))}</time><i aria-hidden="true"></i><span>${esc(segment.name || t("insert"))}</span><small>${esc(formatDuration(seconds))}</small></li>`;
  });
  if (!rows.length && browser.scheduler?.next_switch) rows.push(`<li><time>${esc(clock(browser.scheduler.next_switch))}</time><i aria-hidden="true"></i><span>${esc(t("nextSwitch"))}</span><small></small></li>`);
  return `<div class="up-next">
    <div class="up-next-head"><strong>${esc(t("upNext"))}</strong><button class="link-button" data-view="program">${esc(t("openProgram"))}</button></div>
    ${rows.length ? `<ul>${rows.join("")}</ul>` : `<p class="hint">${esc(t("upNextEmpty"))}</p>`}
  </div>`;
}

// Today's base programme as a 24-hour bar: time windows from the pages, the
// current time, and a line per insert rhythm.
function renderToday() {
  const kiosk = state.config?.kiosk || {};
  const pages = normalizePages(kiosk.pages, kiosk.urls);
  const today = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"][new Date().getDay()];
  const minutes = (value) => {
    const [h, m] = String(value || "").split(":").map(Number);
    return Number.isFinite(h) && Number.isFinite(m) ? h * 60 + m : null;
  };
  const windows = [];
  pages.forEach((page) => {
    if (page.disabled || page.display_mode !== "schedule") return;
    const days = page.schedule?.days || [];
    if (days.length && !days.includes(today)) return;
    const start = minutes(page.schedule?.start);
    const end = minutes(page.schedule?.end);
    if (start === null || end === null) return;
    if (start < end) windows.push([start, end, page.name]);
    else { windows.push([start, 1440, page.name]); windows.push([0, end, page.name]); }
  });
  const rotation = pages.filter((page) => !page.disabled && page.url && page.display_mode === "duration");
  const now = new Date();
  const nowPct = ((now.getHours() * 60 + now.getMinutes()) / 1440) * 100;
  const inserts = (kiosk.inserts || []).filter((insert) => !insert.disabled);
  const blocks = windows.length
    ? windows.map(([start, end, name]) => `<span class="day-window" data-left="${(start / 1440 * 100).toFixed(2)}" data-width="${((end - start) / 1440 * 100).toFixed(2)}" title="${esc(name)}"><em>${esc(name)}</em></span>`).join("")
    : (rotation.length ? `<span class="day-window rotation" data-left="0" data-width="100"><em>${esc(rotation.map((page) => page.name).join(" · "))}</em></span>` : "");
  return `<section class="card today-card" aria-labelledby="today-title">
    <div class="head"><div><h3 id="today-title">${esc(t("today"))}</h3><span class="section-kicker">${esc(windows.length ? t("todayWindowsHint") : t("todayRotationHint"))}</span></div><button data-view="program">${esc(t("openProgram"))}</button></div>
    <div class="body">
      <div class="day-track">${blocks}<span class="day-now" data-left="${nowPct.toFixed(2)}" title="${esc(clock(now))}"></span></div>
      <div class="day-ticks" aria-hidden="true">${[0, 6, 12, 18, 24].map((hour) => `<span data-left="${(hour / 24 * 100).toFixed(2)}">${String(hour).padStart(2, "0")}:00</span>`).join("")}</div>
      ${inserts.length ? `<div class="today-inserts">${inserts.map((insert) => `<span class="legend-item ${insertTone(insert.id)}"><i></i>${esc(insert.name || t("insert"))}</span>`).join("")}</div>` : ""}
    </div>
  </section>`;
}

function renderUpdateNotice() {
  const update = state.update || {};
  if (!update.update_available) return "";
  return `<section class="update-notice" role="status">
    <div><strong>${esc(t("updateAvailable"))}: ${esc(update.latest_version || "")}</strong><span>${esc(t("dashboardUpdateHint"))}</span></div>
    <button class="primary" data-view="device-maintenance">${esc(t("openUpdate"))}</button>
  </section>`;
}
