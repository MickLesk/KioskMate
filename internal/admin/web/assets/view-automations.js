// Automations: event rules (MQTT, time, webhook, touch, idle) that react on
// top of the programme and then hand back to it.
import { esc, isDirty, t } from "./app-core.js";
import { button } from "./renderers.js";
import { renderWorkflows } from "./workflows.js";

export function renderAutomations() {
  return `
    <div class="page-stack automations-view">
      <p class="view-lead">${esc(t("automationsLead"))}</p>
      ${renderWorkflows()}
      <div class="save-bar">
        <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
        <div class="actions">${button("save", "kiosk-save", "primary")}</div>
      </div>
    </div>`;
}
