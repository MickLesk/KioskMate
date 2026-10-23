// Automations: event rules (MQTT, time, webhook, touch, idle) that react on
// top of the programme and then hand back to it, with ready-made templates.
import { esc, formatMQTTState, isDirty, state, t } from "./app-core.js";
import { icon } from "./icons.js";
import { button } from "./renderers.js";
import { WORKFLOW_TEMPLATES, renderWorkflows } from "./workflows.js";

function renderMQTTStatus() {
  const mqtt = state.status?.mqtt || {};
  const enabled = !!state.config?.mqtt?.enabled;
  const tone = !enabled ? "" : mqtt.connected ? "ok" : "bad";
  const detail = !enabled ? t("mqttOffForAutomations") : mqtt.connected ? (state.config?.mqtt?.url || "") : (mqtt.last_error || formatMQTTState(mqtt.state));
  return `<section class="automation-status ${tone}">
    <span class="dot" aria-hidden="true"></span>
    <div><strong>MQTT · ${esc(enabled ? formatMQTTState(mqtt.state) : t("disabled"))}</strong><span>${esc(detail)}</span></div>
    <button data-view="automations-mqtt">${esc(t(enabled ? "mqttSettings" : "mqttSetUp"))}</button>
  </section>`;
}

export function renderAutomations() {
  const count = (state.config?.kiosk?.rules || []).length;
  return `
    <div class="page-stack automations-view">
      <p class="view-lead">${esc(t("automationsLead"))}</p>
      ${renderMQTTStatus()}
      ${renderWorkflows()}
      <section class="card" aria-labelledby="templates-title">
        <div class="head"><div><h3 id="templates-title">${esc(t(count ? "workflowTemplatesMore" : "workflowTemplates"))}</h3><span class="section-kicker">${esc(t("workflowTemplatesHint"))}</span></div></div>
        <div class="body template-grid wide">
          ${WORKFLOW_TEMPLATES.map(([id, glyph, hint]) => `<button class="template-tile" data-workflow-template="${id}">${icon(glyph, 20)}<strong>${esc(t(`tpl${id[0].toUpperCase()}${id.slice(1)}`))}</strong><small>${esc(t(hint))}</small></button>`).join("")}
        </div>
      </section>
      <div class="save-bar">
        <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
        <div class="actions">${button("save", "kiosk-save", "primary")}</div>
      </div>
    </div>`;
}
