// Device health: one readable checklist (power, touch, browser, network,
// memory) with a fix for every warning, above the hardware controls.
import { loadBrowserDoctor } from "./actions-core.js";
import { esc, formatValue, getJSON, formatMQTTState, renderApp, runAction, state, t } from "./app-core.js";
import { cpuCoresUsed, cpuShare, cpuShareValue, humanizeMessage } from "./renderers.js";
import { renderHardware } from "./view-system.js";

function check(level, title, detail, fix = "", value = "") {
  return { level, title, detail, fix, value };
}

export function healthChecks() {
  const hw = state.hardware || {};
  const system = hw.system || {};
  const browser = state.status?.browser || {};
  const mqtt = state.status?.mqtt || {};
  const checks = [];
  const power = system.power;
  if (power) {
    if (power.undervoltage_now) checks.push(check("bad", t("checkPower"), t("undervoltageNow"), t("undervoltageHint"), power.throttled || ""));
    else if (power.undervoltage_since_boot) checks.push(check("warn", t("checkPower"), t("undervoltageSinceBoot"), t("undervoltageHint"), power.throttled || ""));
    else checks.push(check("ok", t("checkPower"), t("powerOk")));
  }
  if (Array.isArray(system.touch_devices)) {
    checks.push(system.touch_devices.length
      ? check("ok", t("touchInput"), system.touch_devices.join(", "))
      : check("warn", t("touchInput"), t("noTouchDevice"), t("noTouchDeviceHint"), t("none")));
  }
  if (state.status?.browser) {
    const share = cpuShareValue(browser.stats?.cpu_percent);
    const detail = browser.ready ? t("checkBrowserReady").replace("{page}", browser.page_name || "-") : browser.running ? t("displayConnecting") : (humanizeMessage(browser.last_error) || t("displayStoppedHint"));
    checks.push(check(browser.ready ? (Number(share || 0) > 75 ? "warn" : "ok") : browser.running ? "warn" : "bad", t("checkBrowser"), detail, Number(share || 0) > 75 ? t("checkBrowserLoadHint") : "", browser.running ? `${cpuShare(browser.stats?.cpu_percent)} · ${cpuCoresUsed(browser.stats?.cpu_percent)}` : ""));
  }
  if (browser.auth_guard?.tripped) checks.push(check("bad", t("authGuard"), browser.auth_guard.reason || t("blocked"), t("checkAuthGuardHint")));
  const recovery = browser.recovery || {};
  if (recovery.state && !["healthy", "idle"].includes(recovery.state)) checks.push(check(recovery.state === "failed" ? "bad" : "warn", t("recoveryState"), humanizeMessage(recovery.last_result || recovery.reason || recovery.state)));
  if (state.config?.mqtt?.enabled) {
    checks.push(mqtt.connected ? check("ok", "MQTT", formatMQTTState(mqtt.state)) : check("bad", "MQTT", mqtt.last_error || formatMQTTState(mqtt.state), t("checkMQTTHint")));
  }
  const memory = system.memory_usage_percent == null ? NaN : Number(system.memory_usage_percent);
  const temperature = system.processor_temperature_c == null ? NaN : Number(system.processor_temperature_c);
  if (Number.isFinite(memory) || Number.isFinite(temperature)) {
    const hot = temperature >= 75;
    const full = memory >= 85;
    checks.push(check(hot || full ? "warn" : "ok", t("checkMemoryTemperature"), Number.isFinite(temperature) ? t("checkMemoryTemperatureDetail").replace("{memory}", formatValue(system.memory_usage_percent, " %")).replace("{temperature}", formatValue(system.processor_temperature_c, " °C")) : t("checkMemoryDetail").replace("{memory}", formatValue(system.memory_usage_percent, " %")), hot ? t("checkHotHint") : full ? t("checkMemoryHint") : ""));
  }
  const disk = Number(system.disk_usage?.percent);
  if (Number.isFinite(disk)) checks.push(check(disk >= 90 ? "warn" : "ok", t("storage"), `${formatValue(system.disk_usage.available_gb, " GB")} ${t("available")}`, disk >= 90 ? t("checkDiskHint") : "", formatValue(disk, " %")));
  if (state.time) checks.push(check(state.time.synchronized ? "ok" : "warn", t("time"), state.time.synchronized ? t("timeSynchronized") : t("timeNotSynchronized"), state.time.synchronized ? "" : t("checkTimeHint")));
  return checks;
}

export function renderDeviceHealth() {
  const checks = healthChecks();
  const problems = checks.filter((item) => item.level !== "ok").length;
  const device = state.hardware?.device || {};
  return `
    <div class="page-stack device-view">
      <section class="card health-card" aria-labelledby="health-title">
        <div class="head">
          <div><h3 id="health-title">${esc(t("deviceHealth"))}</h3><span class="section-kicker ${problems ? "warn-text" : "ok-text"}">${esc(problems ? t("healthProblems").replace("{count}", problems) : t("healthAllGood"))}${device.model ? ` · ${esc(device.model)}` : ""}</span></div>
          <div class="actions"><button data-busy="health-refresh" data-action="health-refresh">${esc(t("checkAgain"))}</button><button data-busy="browser-doctor" data-action="browser-doctor">${esc(t("browserDoctor"))}</button></div>
        </div>
        <ul class="health-checks">
          ${checks.map((item) => `<li class="health-check ${item.level}">
            <span class="health-mark" aria-label="${esc(t(item.level === "ok" ? "healthOk" : item.level === "bad" ? "healthBad" : "healthWarn"))}">${item.level === "ok" ? "✓" : "!"}</span>
            <div><strong>${esc(item.title)}</strong><span>${esc(item.detail)}</span>${item.fix ? `<p class="health-fix">${esc(item.fix)}</p>` : ""}</div>
            ${item.value ? `<code>${esc(item.value)}</code>` : ""}
          </li>`).join("")}
        </ul>
      </section>
      ${renderHardware()}
    </div>`;
}

export function bindDeviceHealth() {
  document.querySelector('[data-action="health-refresh"]')?.addEventListener("click", () => runAction("health-refresh", async () => {
    state.hardware = await getJSON("/api/hardware");
    renderApp();
  }, t("checkAgain")));
  document.querySelector('[data-action="browser-doctor"]')?.addEventListener("click", loadBrowserDoctor);
}
