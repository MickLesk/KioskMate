// First-run setup: on a fresh install (only the demo page, nothing saved)
// three short steps give a working display: the first page, when the screen
// is on, and optionally MQTT for Home Assistant.
import { synchronizeKioskWorkflow } from "./actions-kiosk.js";
import { checked, cloneConfig, closeModal, esc, field, navigate, openModal, postJSON, refreshCore, renderApp, saveConfig, state, switchHtml, t, toast, val } from "./app-core.js";
import { clockField, describeClock, readClock } from "./clock.js";
import { icon } from "./icons.js";
import { validationError } from "./renderers.js";

const DISMISSED = "kioskmate.setupDismissed";
const DEMO = ["https://demo.home-assistant.io", "http://homeassistant.local:8123"];

function isDemo(url) {
  return DEMO.includes(String(url || "").trim().toLowerCase().replace(/\/+$/, ""));
}

// Fresh install: no page other than the demo page.
export function needsSetup(cfg = state.config) {
  const pages = (cfg?.kiosk?.pages || []).filter((page) => !page.disabled);
  return !pages.length || pages.every((page) => !page.source_type?.match(/^(clock|slideshow|html)$/) && isDemo(page.url));
}

export function maybeOpenSetupWizard() {
  let dismissed = false;
  try { dismissed = localStorage.getItem(DISMISSED) === "1"; } catch {}
  if (!dismissed && needsSetup() && !document.querySelector(".modal")) openSetupWizard();
}

const draft = { step: 1, name: "Home Assistant", url: "", always: true, start: "07:00", end: "22:00", mqtt: false, mqttURL: "", mqttUser: "", mqttPassword: "" };

function stepBody() {
  if (draft.step === 1) return `
    <p class="hint">${esc(t("setupPageHint"))}</p>
    <div class="form-grid">
      ${field("setup-name", t("pageName"), "text", "", draft.name)}
      ${field("setup-url", t("pageUrl"), "url", "", draft.url, "http://homeassistant.local:8123/lovelace/kiosk")}
    </div>
    <p class="hint">${esc(t("setupPageHaTip"))}</p>`;
  if (draft.step === 2) return `
    <p class="hint">${esc(t("setupScreenHint"))}</p>
    <div class="segmented setup-choice" role="radiogroup">
      <label class="${draft.always ? "active" : ""}"><input type="radio" name="setup-screen" value="always" ${draft.always ? "checked" : ""}><span>${esc(t("setupScreenAlways"))}</span></label>
      <label class="${draft.always ? "" : "active"}"><input type="radio" name="setup-screen" value="window" ${draft.always ? "" : "checked"}><span>${esc(t("setupScreenWindow"))}</span></label>
    </div>
    <div class="form-grid" data-setup-window ${draft.always ? "hidden" : ""}>
      ${clockField("setup-start", t("setupScreenOn"), draft.start, "07:00")}
      ${clockField("setup-end", t("setupScreenOff"), draft.end, "22:00")}
    </div>`;
  if (draft.step === 3) return `
    <p class="hint">${esc(t("setupMqttHint"))}</p>
    ${switchHtml("setup-mqtt", t("setupMqttEnable"), draft.mqtt)}
    <div class="form-grid" data-setup-mqtt ${draft.mqtt ? "" : "hidden"}>
      <div class="span-2">${field("setup-mqtt-url", t("mqttUrl"), "text", "", draft.mqttURL, "mqtt://homeassistant.local:1883")}</div>
      ${field("setup-mqtt-user", t("username"), "text", "username", draft.mqttUser)}
      ${field("setup-mqtt-password", t("password"), "password", "new-password", draft.mqttPassword)}
    </div>`;
  return `
    <div class="wizard-summary setup-summary">
      <p>${icon("check", 16)} ${esc(t("setupSummaryPage").replace("{name}", draft.name).replace("{url}", draft.url))}</p>
      <p>${icon("clock", 16)} ${esc(draft.always ? t("setupSummaryAlways") : t("setupSummaryWindow").replace("{start}", describeClock(draft.start, false)).replace("{end}", describeClock(draft.end, false)))}</p>
      <p>${icon("link", 16)} ${esc(draft.mqtt ? t("setupSummaryMqtt").replace("{url}", draft.mqttURL) : t("setupSummaryNoMqtt"))}</p>
    </div>
    <p class="hint">${esc(t("setupSummaryHint"))}</p>`;
}

function readStep(validate) {
  if (draft.step === 1) {
    draft.name = val("setup-name").trim() || "Dashboard";
    draft.url = val("setup-url").trim();
    if (validate) {
      try {
        const parsed = new URL(draft.url);
        if (!["http:", "https:"].includes(parsed.protocol)) throw new Error("protocol");
      } catch {
        validationError("setup-url", t("validationPageUrl"));
      }
    }
  }
  if (draft.step === 2) {
    draft.always = document.querySelector('input[name="setup-screen"]:checked')?.value !== "window";
    if (!draft.always) {
      draft.start = readClock("setup-start") || "07:00";
      draft.end = readClock("setup-end") || "22:00";
    }
  }
  if (draft.step === 3) {
    draft.mqtt = checked("setup-mqtt");
    draft.mqttURL = val("setup-mqtt-url").trim();
    draft.mqttUser = val("setup-mqtt-user").trim();
    draft.mqttPassword = val("setup-mqtt-password");
    if (validate && draft.mqtt && !/^mqtts?:\/\/[^/\s]+/.test(draft.mqttURL)) validationError("setup-mqtt-url", t("validationMqttUrl"));
  }
}

export function openSetupWizard() {
  const titles = ["setupStepPage", "setupStepScreen", "setupStepMqtt", "setupStepDone"];
  openModal(`
    <div class="modal setup-wizard" role="dialog" aria-modal="true" aria-labelledby="setup-title">
      <div class="modal-head">
        <div><span class="eyebrow">${esc(t("setupWelcome"))}</span><h3 id="setup-title">${esc(t(titles[draft.step - 1]))}</h3></div>
        <button data-setup="later">${esc(t("setupLater"))}</button>
      </div>
      <ol class="setup-steps" aria-hidden="true">${titles.map((title, index) => `<li class="${index + 1 === draft.step ? "active" : index + 1 < draft.step ? "done" : ""}">${index + 1}</li>`).join("")}</ol>
      <div class="modal-body setup-body">${stepBody()}</div>
      <div class="modal-foot">
        ${draft.step > 1 ? `<button data-setup="back">${esc(t("back"))}</button>` : ""}
        ${draft.step < 4 ? `<button class="primary" data-setup="next">${esc(t(draft.step === 3 && !draft.mqtt ? "setupSkip" : "next"))}</button>` : `<button class="primary" data-setup="finish" data-busy="setup-finish">${esc(t("setupFinish"))}</button>`}
      </div>
    </div>`);
  const modal = document.querySelector(".setup-wizard");
  modal?.querySelectorAll('input[name="setup-screen"]').forEach((radio) => radio.addEventListener("change", () => {
    modal.querySelector("[data-setup-window]").hidden = radio.value !== "window" || !radio.checked;
    modal.querySelectorAll(".setup-choice label").forEach((label) => label.classList.toggle("active", label.querySelector("input")?.checked));
  }));
  document.getElementById("setup-mqtt")?.addEventListener("change", (event) => {
    modal.querySelector("[data-setup-mqtt]").hidden = !event.target.checked;
    const next = modal.querySelector('[data-setup="next"]');
    if (next) next.textContent = t(event.target.checked ? "next" : "setupSkip");
  });
  modal?.querySelector('[data-setup="later"]')?.addEventListener("click", () => {
    try { localStorage.setItem(DISMISSED, "1"); } catch {}
    closeModal();
  });
  modal?.querySelector('[data-setup="back"]')?.addEventListener("click", () => {
    readStep(false);
    draft.step--;
    openSetupWizard();
  });
  modal?.querySelector('[data-setup="next"]')?.addEventListener("click", () => {
    try {
      readStep(true);
      draft.step++;
      openSetupWizard();
    } catch (error) {
      toast(t("validationFailed"), error.message, "error");
    }
  });
  modal?.querySelector('[data-setup="finish"]')?.addEventListener("click", finish);
}

async function finish() {
  const button = document.querySelector('[data-setup="finish"]');
  if (button) button.disabled = true;
  try {
    const cfg = cloneConfig();
    cfg.kiosk = cfg.kiosk || {};
    cfg.kiosk.pages = [{
      page_id: "dashboard",
      name: draft.name,
      url: draft.url,
      source_type: /:8123|home-?assistant/i.test(draft.url) ? "home_assistant" : "url",
      display_mode: draft.always ? "duration" : "schedule",
      duration_seconds: 3600,
      schedule: { start: draft.start, end: draft.end, days: [] },
      display_options: { power_off_after: !draft.always, brightness: 100 },
      disabled: false,
    }];
    cfg.kiosk.scheduler = { ...(cfg.kiosk.scheduler || {}), enabled: true };
    synchronizeKioskWorkflow(cfg.kiosk);
    if (draft.mqtt) {
      cfg.mqtt = { ...(cfg.mqtt || {}), enabled: true, url: draft.mqttURL, username: draft.mqttUser };
      if (draft.mqttPassword) cfg.mqtt.password = draft.mqttPassword;
    }
    await saveConfig(cfg);
    await postJSON(state.status?.browser?.running ? "/api/browser/restart" : "/api/browser/start").catch((error) => toast(t("failed"), error.message, "warn"));
    try { localStorage.setItem(DISMISSED, "1"); } catch {}
    closeModal();
    await refreshCore();
    navigate("program");
    renderApp();
    toast(t("setupDone"), t("setupDoneHint"));
  } catch (error) {
    if (button) button.disabled = false;
    toast(t("failed"), error.message, "error");
  }
}
