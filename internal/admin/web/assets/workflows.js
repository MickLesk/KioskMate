import { checked, cloneConfig, closeModal, esc, field, formatDate, getJSON, isDirty, markDirty, modalRoot, openModal, postJSON, renderApp, selectHtml, state, switchHtml, t, toast, val } from "./app-core.js";
import { normalizePages } from "./renderers.js";

// Workflows are event rules on top of the page sequence: a trigger (MQTT
// message, time of day, webhook), optional conditions and a list of actions.
// They live in config.kiosk.rules and are saved with the rest of the kiosk.

const DAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];
const TRIGGERS = ["mqtt", "time", "webhook", "touch", "idle"];
const ACTIONS = ["show_page", "resume_playlist", "display", "brightness", "reload", "overlay", "script", "click", "scroll", "sound"];

function rules() {
  return state.config?.kiosk?.rules || [];
}

function pages() {
  return normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls);
}

function pageName(pageID) {
  const page = pages().find((item) => item.page_id === pageID);
  return page ? page.name || page.url : pageID || "-";
}

function randomHex(bytes) {
  const data = new Uint8Array(bytes);
  globalThis.crypto.getRandomValues(data);
  return [...data].map((value) => value.toString(16).padStart(2, "0")).join("");
}

function webhookURL(rule) {
  return `${location.origin}/api/rules/${encodeURIComponent(rule.id)}/trigger?token=${encodeURIComponent(rule.trigger?.token || "")}`;
}

function triggerSummary(trigger = {}) {
  if (trigger.type === "mqtt") return `MQTT ${trigger.topic || "?"}${trigger.payload ? ` = ${trigger.payload}` : ""}`;
  if (trigger.type === "time") return `${t("trigger_time")} ${trigger.at || "--:--"}${trigger.days?.length ? ` (${trigger.days.map((day) => t(`dayShort_${day}`)).join(", ")})` : ""}`;
  if (trigger.type === "idle") return t("idleSummary").replace("{seconds}", trigger.idle_seconds || 0);
  return t(`trigger_${trigger.type || "webhook"}`);
}

function actionSummary(action = {}) {
  const label = t(`action_${action.type}`);
  if (action.type === "show_page") return `${label}: ${pageName(action.page_id)}${action.until_resume ? ` (${t("untilResume")})` : action.duration_seconds ? ` (${action.duration_seconds}s)` : ""}`;
  if (action.type === "display") return `${label}: ${action.value === "OFF" ? t("off") : t("on")}${action.duration_seconds ? ` (${action.duration_seconds}s)` : ""}`;
  if (["click", "scroll"].includes(action.type)) return `${label}: ${action.value}`;
  if (action.type === "brightness") return `${label}: ${action.value}%`;
  if (action.type === "overlay") return `${label}: “${action.value}”`;
  return label;
}

export function renderWorkflows() {
  const items = rules();
  const dirty = isDirty();
  return `<section class="data-section workflow-section">
    <div class="data-section-head">
      <div><h3>${esc(t("workflows"))}</h3><span class="section-kicker">${esc(t("workflowsHint"))}</span></div>
      <div class="actions"><button class="primary" data-action="workflow-new">+ ${esc(t("newWorkflow"))}</button></div>
    </div>
    <div class="data-section-body">
      ${items.length ? `<div class="workflow-list">${items.map((rule, index) => `
        <article class="workflow-row ${rule.disabled ? "disabled" : ""}">
          <div class="workflow-copy">
            <strong>${esc(rule.name || rule.id)}${rule.disabled ? ` <small class="muted">${esc(t("disabled"))}</small>` : ""}</strong>
            <span>${esc(t("workflowSummary").replace("{trigger}", triggerSummary(rule.trigger)).replace("{actions}", (rule.actions || []).map(actionSummary).join(" → ")))}</span>
          </div>
          <div class="actions">
            <button data-workflow-test="${index}" ${rule.disabled ? "disabled" : ""} title="${esc(dirty ? t("workflowTestSaveFirst") : t("testWorkflowHint"))}">${esc(t("testWorkflow"))}</button>
            <button data-workflow-edit="${index}">${esc(t("edit"))}</button>
            <button class="danger-ghost" data-workflow-remove="${index}" title="${esc(t("remove"))}" aria-label="${esc(t("remove"))}">&times;</button>
          </div>
        </article>`).join("")}</div>` : `<div class="empty">${esc(t("noWorkflows"))}</div>`}
      <details class="workflow-history" data-workflow-history ${state.workflowHistoryOpen ? "open" : ""}>
        <summary>${esc(t("recentRuns"))}</summary>
        <div id="workflow-history-body">${renderHistory()}</div>
      </details>
    </div>
  </section>`;
}

function renderHistory() {
  const runs = state.workflowHistory;
  if (!runs) return `<div class="empty">${esc(t("loading"))}</div>`;
  if (!runs.length) return `<div class="empty">${esc(t("noRuns"))}</div>`;
  return `<ul class="workflow-runs">${runs.map((run) => `<li class="${run.error ? "error-text" : run.skipped ? "muted" : ""}">
    <time>${esc(formatDate(run.at))}</time>
    <strong>${esc(run.rule || run.rule_id)}</strong>
    <span>${esc(run.source)}</span>
    <span>${esc(run.error || (run.skipped ? `${t("skipped")}: ${run.skipped}` : run.result || "ok"))}</span>
  </li>`).join("")}</ul>`;
}

async function loadHistory() {
  try {
    const data = await getJSON("/api/rules/history");
    state.workflowHistory = data.runs || [];
  } catch (error) {
    state.workflowHistory = [];
    toast(t("failed"), error.message, "error");
  }
  const body = document.getElementById("workflow-history-body");
  if (body) body.innerHTML = renderHistory();
}

export function bindWorkflows() {
  document.querySelector('[data-action="workflow-new"]')?.addEventListener("click", () => openWorkflowEditor(null));
  document.querySelectorAll("[data-workflow-edit]").forEach((button) => button.addEventListener("click", () => openWorkflowEditor(Number(button.dataset.workflowEdit))));
  document.querySelectorAll("[data-workflow-remove]").forEach((button) => button.addEventListener("click", () => {
    const index = Number(button.dataset.workflowRemove);
    const rule = rules()[index];
    if (!rule || !confirm(t("confirmRemoveWorkflow").replace("{name}", rule.name || rule.id))) return;
    const cfg = cloneConfig();
    cfg.kiosk.rules.splice(index, 1);
    state.config = cfg;
    markDirty();
    renderApp();
  }));
  document.querySelectorAll("[data-workflow-test]").forEach((button) => button.addEventListener("click", () => testWorkflow(Number(button.dataset.workflowTest))));
  document.querySelector("[data-workflow-history]")?.addEventListener("toggle", (event) => {
    state.workflowHistoryOpen = event.target.open;
    if (event.target.open) loadHistory();
  });
}

async function testWorkflow(index) {
  const rule = rules()[index];
  if (!rule) return;
  const saved = (state.persistedConfig?.kiosk?.rules || []).find((item) => item.id === rule.id);
  if (isDirty() || !saved) {
    toast(t("unsavedChanges"), t("workflowTestSaveFirst"), "warn");
    return;
  }
  try {
    await postJSON(`/api/rules/${encodeURIComponent(rule.id)}/test`);
    toast(t("workflowTested"), rule.name || rule.id, "ok");
  } catch (error) {
    toast(t("failed"), error.message, "error");
  }
  if (state.workflowHistoryOpen) loadHistory();
}

function newRule() {
  return {
    id: `rule_${randomHex(5)}`,
    name: `${t("workflow")} ${rules().length + 1}`,
    disabled: false,
    trigger: { type: "mqtt", topic: "", payload: "" },
    conditions: {},
    actions: [{ type: "show_page", page_id: pages()[0]?.page_id || "", duration_seconds: 30 }],
    cooldown_seconds: 0,
  };
}

function openWorkflowEditor(index) {
  const source = index === null ? newRule() : rules()[index];
  if (!source) return;
  state.workflowEditor = { index, rule: JSON.parse(JSON.stringify(source)) };
  state.workflowEditor.rule.conditions = state.workflowEditor.rule.conditions || {};
  state.workflowEditor.rule.actions = state.workflowEditor.rule.actions || [];
  renderWorkflowEditor();
}

function dayPicker(prefix, selected) {
  const active = new Set(selected || []);
  return `<fieldset class="day-picker"><legend>${esc(t("days"))}</legend><div>${DAYS.map((day) => `<label><input id="${prefix}-${day}" type="checkbox" ${active.has(day) ? "checked" : ""}><span>${esc(t(`dayShort_${day}`))}</span></label>`).join("")}</div></fieldset>`;
}

function renderTriggerFields(rule) {
  const trigger = rule.trigger;
  if (trigger.type === "mqtt") return `${field("wf-topic", t("mqttTopic"), "text", "", trigger.topic || "", "home/doorbell")}${field("wf-payload", t("payloadAny"), "text", "", trigger.payload || "")}`;
  if (trigger.type === "time") return `${field("wf-at", t("triggerAt"), "time", "", trigger.at || "07:00")}<div class="span-2">${dayPicker("wf-trigger-day", trigger.days)}</div>`;
  if (trigger.type === "idle") return `${field("wf-idle", t("idleSeconds"), "number", "", trigger.idle_seconds || 300)}<p class="hint">${esc(t("idleTriggerHint"))}</p>`;
  if (trigger.type === "touch") return `<p class="hint span-2">${esc(t("touchTriggerHint"))}</p>`;
  return `<div class="span-2 readonly-field workflow-webhook">
    <span>${esc(t("webhookUrl"))}</span>
    <code>${esc(webhookURL(rule))}</code>
    <small>${esc(t("webhookHint"))}</small>
    <div class="actions"><button data-wf-copy-url>${esc(t("copy"))}</button><button data-wf-new-token>${esc(t("regenerateToken"))}</button></div>
  </div>`;
}

function renderActionRow(action, index) {
  const pageOptions = pages().map((page) => [page.page_id, page.name || page.url]);
  let fields = "";
  if (action.type === "show_page") fields = `${selectHtml(`wf-action-page-${index}`, t("workflowPage"), action.page_id || "", pageOptions)}${field(`wf-action-duration-${index}`, t("showPageDuration"), "number", "", action.duration_seconds || 0)}<div class="span-2">${switchHtml(`wf-action-until-${index}`, t("untilResume"), !!action.until_resume)}</div>`;
  if (action.type === "display") fields = `${selectHtml(`wf-action-value-${index}`, t("action_display"), action.value || "ON", [["ON", t("on")], ["OFF", t("off")]])}${field(`wf-action-duration-${index}`, t("displayRevertSeconds"), "number", "", action.duration_seconds || "", "0")}`;
  if (action.type === "click") fields = field(`wf-action-value-${index}`, t("cssSelector"), "text", "", action.value || "", "button.primary");
  if (action.type === "scroll") fields = field(`wf-action-value-${index}`, t("scrollTarget"), "text", "", action.value || "400");
  if (action.type === "sound") fields = `<div class="span-2">${field(`wf-action-value-${index}`, t("soundUrl"), "url", "", action.value || "", "https://")}</div>`;
  if (action.type === "brightness") fields = field(`wf-action-value-${index}`, t("brightness"), "number", "", action.value || "80");
  if (action.type === "overlay") fields = `${field(`wf-action-value-${index}`, t("overlayText"), "text", "", action.value || "")}${field(`wf-action-duration-${index}`, t("durationSeconds"), "number", "", action.duration_seconds || 10)}`;
  if (action.type === "script") fields = `<div class="span-2"><label for="wf-action-value-${index}">JavaScript</label><textarea id="wf-action-value-${index}" rows="3" spellcheck="false">${esc(action.value || "")}</textarea></div>`;
  return `<div class="workflow-action">
    <span class="step-number">${index + 1}</span>
    <div class="workflow-action-fields">
      ${selectHtml(`wf-action-type-${index}`, t("workflowAction"), action.type, ACTIONS.map((type) => [type, t(`action_${type}`)]))}
      ${fields}
    </div>
    <button class="danger-ghost" data-wf-action-remove="${index}" title="${esc(t("remove"))}" aria-label="${esc(t("remove"))}">&times;</button>
  </div>`;
}

function renderWorkflowEditor() {
  const editor = state.workflowEditor;
  if (!editor) return;
  if (modalRoot.innerHTML) closeModal();
  const rule = editor.rule;
  const conditions = rule.conditions;
  openModal(`
    <div class="modal workflow-editor" role="dialog" aria-modal="true" aria-labelledby="workflow-editor-title">
      <div class="modal-head"><div><h3 id="workflow-editor-title">${esc(editor.index === null ? t("newWorkflow") : t("editWorkflow"))}</h3><div class="hint">${esc(t("workflowsHint"))}</div></div><button data-modal-close aria-label="${esc(t("close"))}">&times;</button></div>
      <div class="modal-body workflow-editor-body">
        <div class="wizard-step-grid">
          ${field("wf-name", t("workflowName"), "text", "", rule.name || "")}
          ${field("wf-cooldown", t("cooldownSeconds"), "number", "", rule.cooldown_seconds || 0)}
          <div class="span-2">${switchHtml("wf-disabled", t("disabled"), !!rule.disabled)}</div>
        </div>
        <h4>${esc(t("trigger"))}</h4>
        <div class="wizard-step-grid">
          <div class="span-2">${selectHtml("wf-trigger-type", t("trigger"), rule.trigger.type, TRIGGERS.map((type) => [type, t(`trigger_${type}`)]))}</div>
          ${renderTriggerFields(rule)}
        </div>
        <details class="wizard-options" ${conditions.start || conditions.days?.length || conditions.active_page_id ? "open" : ""}>
          <summary>${esc(t("conditions"))}</summary>
          <div class="wizard-step-grid">
            <div class="span-2 hint">${esc(t("conditionsHint"))}</div>
            ${field("wf-cond-start", t("start"), "time", "", conditions.start || "")}
            ${field("wf-cond-end", t("end"), "time", "", conditions.end || "")}
            <div class="span-2">${dayPicker("wf-cond-day", conditions.days)}</div>
            <div class="span-2">${selectHtml("wf-cond-page", t("onlyWhenPage"), conditions.active_page_id || "", [["", t("anyPage")], ...pages().map((page) => [page.page_id, page.name || page.url])])}</div>
          </div>
        </details>
        <h4>${esc(t("workflowActions"))}</h4>
        <div class="workflow-actions">${rule.actions.map(renderActionRow).join("")}</div>
        <button data-wf-action-add>+ ${esc(t("addAction"))}</button>
      </div>
      <div class="modal-foot"><button data-modal-close>${esc(t("cancel"))}</button><button class="primary" data-wf-apply>${esc(t("apply"))}</button></div>
    </div>`);
  modalRoot.querySelector("#wf-trigger-type")?.addEventListener("change", (event) => {
    readWorkflowEditor();
    rule.trigger = { type: event.target.value, days: [] };
    if (rule.trigger.type === "webhook") rule.trigger.token = randomHex(16);
    renderWorkflowEditor();
  });
  modalRoot.querySelectorAll('[id^="wf-action-type-"]').forEach((select) => select.addEventListener("change", () => {
    readWorkflowEditor();
    const index = Number(select.id.replace("wf-action-type-", ""));
    rule.actions[index] = { type: select.value, ...(select.value === "show_page" ? { page_id: pages()[0]?.page_id || "", duration_seconds: 30 } : {}) };
    renderWorkflowEditor();
  }));
  modalRoot.querySelector("[data-wf-action-add]")?.addEventListener("click", () => {
    readWorkflowEditor();
    rule.actions.push({ type: "resume_playlist" });
    renderWorkflowEditor();
  });
  modalRoot.querySelectorAll("[data-wf-action-remove]").forEach((button) => button.addEventListener("click", () => {
    readWorkflowEditor();
    rule.actions.splice(Number(button.dataset.wfActionRemove), 1);
    renderWorkflowEditor();
  }));
  modalRoot.querySelector("[data-wf-new-token]")?.addEventListener("click", () => {
    readWorkflowEditor();
    rule.trigger.token = randomHex(16);
    renderWorkflowEditor();
  });
  modalRoot.querySelector("[data-wf-copy-url]")?.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(webhookURL(rule));
      toast(t("copied"), "", "ok");
    } catch (error) {
      toast(t("failed"), error.message, "error");
    }
  });
  modalRoot.querySelector("[data-wf-apply]")?.addEventListener("click", applyWorkflowEditor);
}

function selectedDays(prefix) {
  return DAYS.filter((day) => checked(`${prefix}-${day}`));
}

function readWorkflowEditor() {
  const rule = state.workflowEditor.rule;
  rule.name = val("wf-name").trim();
  rule.cooldown_seconds = Math.max(0, Number(val("wf-cooldown") || 0));
  rule.disabled = checked("wf-disabled");
  const trigger = rule.trigger;
  if (trigger.type === "mqtt") Object.assign(trigger, { topic: val("wf-topic").trim(), payload: val("wf-payload").trim() });
  if (trigger.type === "time") Object.assign(trigger, { at: val("wf-at"), days: selectedDays("wf-trigger-day") });
  if (trigger.type === "idle") trigger.idle_seconds = Number(val("wf-idle") || 0);
  const conditions = { start: val("wf-cond-start"), end: val("wf-cond-end"), days: selectedDays("wf-cond-day"), active_page_id: val("wf-cond-page") };
  rule.conditions = Object.fromEntries(Object.entries(conditions).filter(([, value]) => value && value.length));
  rule.actions = rule.actions.map((action, index) => {
    const next = { type: action.type };
    if (action.type === "show_page") Object.assign(next, { page_id: val(`wf-action-page-${index}`), duration_seconds: Number(val(`wf-action-duration-${index}`) || 0), until_resume: checked(`wf-action-until-${index}`) });
    if (["display", "brightness", "overlay", "script", "click", "scroll", "sound"].includes(action.type)) next.value = val(`wf-action-value-${index}`).trim();
    if (["overlay", "display"].includes(action.type)) next.duration_seconds = Number(val(`wf-action-duration-${index}`) || 0);
    return next;
  });
}

function validateWorkflow(rule) {
  if (!rule.name) return t("validationWorkflowName");
  const trigger = rule.trigger;
  if (trigger.type === "mqtt" && (!trigger.topic || /[+#]/.test(trigger.topic))) return t("validationWorkflowTopic");
  if (trigger.type === "time" && !trigger.at) return t("validationWorkflowTime");
  if (trigger.type === "idle" && !(trigger.idle_seconds >= 10 && trigger.idle_seconds <= 86400)) return t("validationIdleSeconds");
  if (trigger.type === "webhook" && (trigger.token || "").length < 16) trigger.token = randomHex(16);
  if (!!rule.conditions.start !== !!rule.conditions.end) return t("validationWorkflowWindow");
  if (!rule.actions.length) return t("validationWorkflowActions");
  for (const action of rule.actions) {
    if (action.type === "show_page" && !action.page_id) return t("validationWorkflowActions");
    if (action.type === "brightness" && !(Number(action.value) >= 1 && Number(action.value) <= 100)) return t("validationBrightness");
    if (["overlay", "script", "click", "scroll"].includes(action.type) && !String(action.value || "").trim()) return t("validationWorkflowActions");
    if (action.type === "sound" && action.value && !/^https?:\/\//.test(action.value)) return t("validationSoundUrl");
  }
  return "";
}

function applyWorkflowEditor() {
  readWorkflowEditor();
  const { index, rule } = state.workflowEditor;
  const problem = validateWorkflow(rule);
  if (problem) {
    toast(t("validationFailed"), problem, "error");
    return;
  }
  const cfg = cloneConfig();
  cfg.kiosk = cfg.kiosk || {};
  cfg.kiosk.rules = cfg.kiosk.rules || [];
  if (index === null) cfg.kiosk.rules.push(rule);
  else cfg.kiosk.rules[index] = rule;
  state.config = cfg;
  state.workflowEditor = null;
  closeModal();
  markDirty();
  renderApp();
  toast(t("unsavedChanges"), t("workflowSaveReminder"), "warn");
}
