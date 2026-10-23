// Overview: what is on screen now, what comes next, the few controls needed
// every day, and today's programme at a glance. Troubleshooting tools stay
// one click away in a disclosure.
import { formatThemeStatus } from "./actions-core.js";
import { esc, formatMQTTState, formatValue, selectHtml, state, t } from "./app-core.js";
import { icon } from "./icons.js";
import { button, cpuCoresUsed, cpuShare, cpuShareValue, humanizeMessage, normalizePages, powerBanner, stateBanner, statusTile } from "./renderers.js";
import { baseRuns, clock, insertTone, renderTimeline, visibleInserts } from "./timeline.js";
import { healthChecks } from "./view-device.js";
import { describeInsert } from "./view-program.js";

function reasonText(scheduler) {
  const reason = scheduler?.reason;
  if (reason === "insert") return t("reasonInsertNamed").replace("{name}", scheduler.active_insert || "");
  if (reason === "time") return t("reasonTimeWindow").replace("{name}", scheduler.active_rule || "");
  if (reason === "rotation") return t("reasonRotation");
  if (reason === "manual override") return t("temporaryOverride");
  if (reason === "no active time rule") return t("noActiveRule");
  return t("manualPageControl");
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
  const recoveryBanner = recovery.state && !["healthy", "idle"].includes(recovery.state);
  const problems = healthChecks().filter((item) => item.level !== "ok" && item.title !== t("checkPower") && !(recoveryBanner && item.title === t("recoveryState")));
  const displayPower = String(hw.display?.power || "").toUpperCase();
  return `
    <div class="page-stack overview">
      ${renderUpdateNotice()}
      ${powerBanner()}
      ${recoveryBanner ? stateBanner(recovery.state === "failed" || recovery.state === "auth_blocked" ? "bad" : "warn", t("recoveryNeedsAttention"), humanizeMessage(recovery.last_result || recovery.reason || recovery.state), button("recoverNow", "browser-auto-recover", "primary")) : ""}
      ${schedulerNeedsAttention ? stateBanner("warn", t("scheduler"), t("schedulerDisabledWithRulesHint"), `<button data-view="program">${esc(t("openProgram"))}</button>`) : ""}
      ${problems.length ? stateBanner(problems.some((item) => item.level === "bad") ? "bad" : "warn", t("healthProblems").replace("{count}", problems.length), problems.map((item) => String(item.detail).toLowerCase().includes(String(item.title).toLowerCase()) ? item.detail : `${item.title}: ${item.detail}`).join(" · "), `<button data-view="device">${esc(t("openDevice"))}</button>`) : ""}

      <section class="overview-hero">
        <div class="card live-card">
          <div class="preview-shell">
            ${state.snapshotURL ? `<img id="snapshot-image" class="snapshot-image" src="${esc(state.snapshotURL)}" alt="${esc(t("liveView"))}" />` : `<div id="snapshot-empty" class="empty">${icon("camera", 28)}<span>${esc(browser.running ? (state.snapshotAuto && browser.devtools ? t("snapshotLoading") : t("snapshotOnDemand")) : t("liveViewStopped"))}</span></div>`}
          </div>
          <div class="preview-meta">
            <span>${esc(state.snapshotTime ? t("snapshotTakenAt").replace("{time}", clock(state.snapshotTime)) : t("liveView"))}</span>
            <label class="inline-check" title="${esc(t("snapshotAutoHint"))}"><input type="checkbox" id="snapshot-auto" data-no-dirty ${state.snapshotAuto ? "checked" : ""}><span>${esc(t("snapshotAuto"))}</span></label>
            <div class="actions">${button("refreshSnapshot", "dashboard-snapshot-refresh")}${button("openPreview", "dashboard-preview-open")}</div>
          </div>
        </div>
        <div class="card now-card">
          <div class="now-head">
            <span class="eyebrow">${esc(t("nowOnScreen"))}</span>
            <span class="chip ${runningTone}"><span class="dot" aria-hidden="true"></span>${esc(runningLabel)}</span>
          </div>
          <strong class="now-title">${esc(!browserKnown ? t("loading") : browser.running ? (browser.page_name || activePage.name || "-") : t("displayStopped"))}</strong>
          <span class="now-reason">${browser.running || !browserKnown
            ? `${esc(reasonText(browser.scheduler))}${browser.scheduler?.next_switch ? ` · ${esc(t("nextSwitch"))} ${esc(clock(browser.scheduler.next_switch))}` : ""}`
            : esc(t("plannedNow").replace("{page}", browser.page_name || activePage.name || "-"))}</span>
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
            ${browser.last_error ? `<div><dt>${esc(t("lastError"))}</dt><dd>${esc(humanizeMessage(browser.last_error))}</dd></div>` : ""}
          </dl>
        </div>
      </details>
    </div>`;
}

// Up next: each insert once with its next run and rhythm, plus the next
// change of the base programme (another time window, display off).
function renderUpNext(browser) {
  const kiosk = state.config?.kiosk || {};
  const pages = normalizePages(kiosk.pages, kiosk.urls);
  const timeline = state.timelineToday || state.timeline;
  const now = Date.now();
  const from = Math.floor(now / 60000) * 60000;
  const runs = baseRuns(kiosk, from, 24 * 60);
  const shown = visibleInserts(timeline?.upcoming || [], runs);
  const rows = [];
  const seen = new Set();
  for (const segment of shown) {
    if (seen.has(segment.insert_id)) continue;
    seen.add(segment.insert_id);
    const insert = (kiosk.inserts || []).find((item) => item.id === segment.insert_id) || {};
    const start = new Date(segment.start).getTime();
    rows.push({ at: start, html: `<li class="${insertTone(segment.insert_id)}"><time>${esc(start <= now ? t("now") : clock(start))}</time><i aria-hidden="true"></i><span>${esc(segment.name || insert.name || pages.find((page) => page.page_id === segment.page_id)?.name || t("insert"))}<small>${esc(insert.id ? describeInsert(insert).split(" · ")[0] : "")}</small></span></li>` });
  }
  const change = runs[1];
  if (change && kiosk.scheduler?.enabled) {
    const label = change.kind === "off" ? t("displayOffPeriod") : change.kind === "rotation" ? t("rotationMode") : change.name;
    rows.push({ at: change.start, html: `<li class="base-change ${change.kind}"><time>${esc(clock(change.start))}</time><i aria-hidden="true"></i><span>${esc(label)}<small>${esc(t("baseChange"))}</small></span></li>` });
  } else if (!rows.length && browser.scheduler?.next_switch) {
    rows.push({ at: 0, html: `<li><time>${esc(clock(browser.scheduler.next_switch))}</time><i aria-hidden="true"></i><span>${esc(t("nextSwitch"))}</span></li>` });
  }
  rows.sort((a, b) => a.at - b.at);
  return `<div class="up-next">
    <div class="up-next-head"><strong>${esc(t("upNext"))}</strong><button class="link-button" data-view="program">${esc(t("openProgram"))}</button></div>
    ${rows.length ? `<ul>${rows.slice(0, 5).map((row) => row.html).join("")}</ul>` : `<p class="hint">${esc(t("upNextEmpty"))}</p>`}
  </div>`;
}

// Today from midnight: base programme and inserts, with the current time.
function renderToday() {
  const kiosk = state.config?.kiosk || {};
  const pages = normalizePages(kiosk.pages, kiosk.urls);
  const pageName = (id) => pages.find((page) => page.page_id === id)?.name || id;
  const timeline = state.timelineToday;
  const today = timeline ? renderTimeline({ timeline, kiosk, pageName }) : null;
  return `<section class="card today-card" aria-labelledby="today-title">
    <div class="head"><div><h3 id="today-title">${esc(t("today"))}</h3><span class="section-kicker">${esc(today ? t("todayHint").replace("{percent}", Math.round((1 - today.share) * 100)) : t("loading"))}</span></div><button data-view="program">${esc(t("openProgram"))}</button></div>
    <div class="body">${today ? today.html : ""}</div>
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
