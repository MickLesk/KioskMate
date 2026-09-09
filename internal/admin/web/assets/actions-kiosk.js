"use strict";

function bindKiosk() {
        bindBrowserButtons();
        document.querySelectorAll("[data-kiosk-mode]").forEach((button) => button.addEventListener("click", () => {
          state.kioskEditorMode = button.dataset.kioskMode === "flow" ? "flow" : "storybook";
          localStorage.setItem("kioskmate.kioskEditorMode", state.kioskEditorMode);
          renderApp();
        }));
        document.querySelectorAll("[data-page-select]").forEach((item) => item.addEventListener("click", (event) => {
          if (event.target.closest("[data-page-edit],[data-page-remove],[data-page-move]")) return;
          state.kioskSelectedPageIndex = Number(item.dataset.pageSelect);
          renderApp();
        }));
        document.querySelectorAll("[data-page-edit]").forEach((button) => button.addEventListener("click", (event) => {
          event.stopPropagation();
          openPageWizard(Number(button.dataset.pageEdit));
        }));
        document.querySelector('[data-action="page-wizard-edit"]')?.addEventListener("click", () => openPageWizard(selectedKioskPageIndex(collectPages())));
        document.querySelector('[data-action="page-wizard-new"]')?.addEventListener("click", () => openPageWizard(null, collectPages().length));
        document.querySelectorAll("[data-page-insert]").forEach((button) => button.addEventListener("click", () => openPageWizard(null, Number(button.dataset.pageInsert))));
        bindSequenceDragAndDrop();
        document.querySelectorAll('[data-action="page-add"]').forEach((button) => button.addEventListener("click", () => addKioskPage(false)));
        document.querySelectorAll('[data-action="page-add-ha"]').forEach((button) => button.addEventListener("click", () => addKioskPage(true)));
        document.querySelector('[data-action="kiosk-save"]')?.addEventListener("click", () => saveKiosk(false));
        document.querySelector('[data-action="kiosk-save-restart"]')?.addEventListener("click", () => saveKiosk(true));
        document.querySelector('[data-action="page-check"]')?.addEventListener("click", checkSelectedPage);
        document.querySelector('[data-action="render-check"]')?.addEventListener("click", renderCheckSelectedPage);
        document.querySelector('[data-action="page-check-all"]')?.addEventListener("click", checkAllPages);
        document.querySelector('[data-action="page-enable-all"]')?.addEventListener("click", () => bulkSetPagesDisabled(false));
        document.querySelector('[data-action="page-disable-all"]')?.addEventListener("click", () => bulkSetPagesDisabled(true));
        document.querySelector('[data-action="pages-export"]')?.addEventListener("click", exportPages);
        document.querySelector('[data-action="pages-import"]')?.addEventListener("click", () => document.getElementById("pages-import-file")?.click());
        document.getElementById("pages-import-file")?.addEventListener("change", importPages);
        document.querySelector('[data-action="page-activate"]')?.addEventListener("click", activateSelectedPage);
		document.querySelector('[data-action="override-apply"]')?.addEventListener("click", applyPageOverride);
		document.querySelector('[data-action="override-clear"]')?.addEventListener("click", clearPageOverride);
        document.querySelector('[data-action="preview-open"]')?.addEventListener("click", openPreview);
        document.getElementById("page-filter")?.addEventListener("input", (event) => {
          state.pageFilter = event.target.value || "";
          applyPageFilter(state.pageFilter);
        });
        document.querySelectorAll("[data-page-remove]").forEach((button) => {
          button.addEventListener("click", () => {
            if (!confirm(t("confirmRemovePage"))) return;
            const cfg = cloneConfig();
            cfg.kiosk.pages = collectPages();
            removePage(cfg.kiosk, Number(button.dataset.pageRemove));
            synchronizeKioskWorkflow(cfg.kiosk);
            state.config = cfg;
            state.kioskSelectedPageIndex = Math.max(0, Math.min(Number(button.dataset.pageRemove), cfg.kiosk.pages.length - 1));
            markDirty();
            renderApp();
          });
        });
        document.querySelectorAll("[data-page-duplicate]").forEach((button) => {
          button.addEventListener("click", () => {
            const cfg = cloneConfig();
            cfg.kiosk = cfg.kiosk || {};
            const index = Number(button.dataset.pageDuplicate);
            cfg.kiosk.pages = collectPages();
            const source = cfg.kiosk.pages[index];
            if (source) cfg.kiosk.pages.splice(index + 1, 0, { ...source, page_id: createPageID(), name: `${source.name || t("pages")} ${t("copy")}` });
            synchronizeKioskWorkflow(cfg.kiosk);
            state.config = cfg;
            markDirty();
            renderApp();
          });
        });
        document.querySelectorAll("[data-page-move]").forEach((button) => {
          button.addEventListener("click", () => {
            const cfg = cloneConfig();
            cfg.kiosk = cfg.kiosk || {};
            cfg.kiosk.pages = collectPages();
            movePage(cfg.kiosk, Number(button.dataset.pageMove), Number(button.dataset.direction));
            synchronizeKioskWorkflow(cfg.kiosk);
            state.config = cfg;
            state.kioskSelectedPageIndex = Math.max(0, Number(button.dataset.pageMove) + Number(button.dataset.direction));
            markDirty();
            renderApp();
          });
        });
      }

      function bindSequenceDragAndDrop() {
        let dragged = null;
        document.querySelectorAll("[data-sequence-index]").forEach((card) => {
          card.addEventListener("dragstart", (event) => {
            dragged = Number(card.dataset.sequenceIndex);
            card.classList.add("dragging");
            event.dataTransfer.effectAllowed = "move";
          });
          card.addEventListener("dragend", () => card.classList.remove("dragging"));
          card.addEventListener("dragover", (event) => {
            event.preventDefault();
            card.classList.add("drag-over");
          });
          card.addEventListener("dragleave", () => card.classList.remove("drag-over"));
          card.addEventListener("drop", (event) => {
            event.preventDefault();
            card.classList.remove("drag-over");
            const target = Number(card.dataset.sequenceIndex);
            if (dragged === null || dragged === target) return;
            const cfg = cloneConfig();
            cfg.kiosk.pages = collectPages();
            const [page] = cfg.kiosk.pages.splice(dragged, 1);
            cfg.kiosk.pages.splice(target, 0, page);
            synchronizeKioskWorkflow(cfg.kiosk);
            state.config = cfg;
            state.kioskSelectedPageIndex = target;
            markDirty();
            renderApp();
          });
        });
      }

      function openPageWizard(index = null, insertAt = null, sourceType = "url") {
        const pages = collectPages();
        const editing = index !== null && pages[index];
        const page = editing ? { ...pages[index], schedule: { ...(pages[index].schedule || {}) }, trigger: { ...(pages[index].trigger || {}) }, display_options: { ...(pages[index].display_options || {}) } } : {
          page_id: createPageID(),
          name: sourceType === "home_assistant" ? "Home Assistant" : `${t("pages")} ${pages.length + 1}`,
          url: sourceType === "home_assistant" ? "http://homeassistant.local:8123" : "https://",
          source_type: sourceType,
          display_mode: "duration",
          duration_seconds: 60,
          schedule: { start: "08:00", end: "18:00", days: [] },
          trigger: { topic: "", payload: "ON" },
          display_options: { power_off_after: false, screensaver: false, brightness: 100 },
          disabled: false,
        };
        state.pageWizard = { step: 1, index: editing ? Number(index) : null, insertAt: insertAt === null ? pages.length : Number(insertAt), page };
        renderPageWizard();
      }

      function createPageID() {
        if (globalThis.crypto?.randomUUID) return `page_${crypto.randomUUID().replaceAll("-", "").slice(0, 10)}`;
        return `page_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`;
      }

      function renderPageWizard() {
        const wizard = state.pageWizard;
        if (!wizard) return;
        if (modalRoot.innerHTML) closeModal();
        const page = wizard.page;
        openModal(`
          <div class="modal page-wizard" role="dialog" aria-modal="true" aria-labelledby="page-wizard-title">
            <div class="modal-head"><div><h3 id="page-wizard-title">${esc(wizard.index === null ? t("createKioskPage") : t("editKioskPage"))}</h3><div class="hint">${esc(t("wizardHint"))}</div></div><button data-modal-close aria-label="${esc(t("close"))}">&times;</button></div>
            <div class="wizard-progress" aria-label="${esc(t("wizardProgress"))}">${[1, 2, 3].map((step) => `<div class="${wizard.step === step ? "active" : wizard.step > step ? "done" : ""}"><span>${wizard.step > step ? "&#10003;" : step}</span><strong>${esc(t(`wizardStep${step}`))}</strong></div>`).join("")}</div>
            <div class="modal-body wizard-body">${renderPageWizardStep(wizard)}</div>
            <div class="modal-foot"><button data-modal-close>${esc(t("cancel"))}</button><div class="actions">${wizard.step > 1 ? `<button data-wizard-back>${esc(t("back"))}</button>` : ""}${wizard.step < 3 ? `<button class="primary" data-wizard-next>${esc(t("next"))}</button>` : `<button data-wizard-finish>${esc(t("finish"))}</button><button class="primary" data-wizard-start>${esc(t("finishAndStart"))}</button>`}</div></div>
          </div>`);
        document.querySelector("[data-wizard-back]")?.addEventListener("click", () => { readPageWizardStep(); state.pageWizard.step--; renderPageWizard(); });
        document.querySelector("[data-wizard-next]")?.addEventListener("click", () => {
          try { readPageWizardStep(true); state.pageWizard.step++; renderPageWizard(); } catch (error) { toast(t("validationFailed"), error.message, "error"); }
        });
        document.querySelector("[data-wizard-finish]")?.addEventListener("click", () => commitPageWizard(false));
        document.querySelector("[data-wizard-start]")?.addEventListener("click", () => commitPageWizard(true));
        document.getElementById("wizard-display-mode")?.addEventListener("change", () => { readPageWizardStep(false); renderPageWizard(); });
        document.getElementById("wizard-source-type")?.addEventListener("change", (event) => {
          readPageWizardStep(false);
          state.pageWizard.page.source_type = event.target.value;
          if (event.target.value === "home_assistant" && (!state.pageWizard.page.url || state.pageWizard.page.url === "https://")) state.pageWizard.page.url = "http://homeassistant.local:8123";
          renderPageWizard();
        });
        document.getElementById("wizard-brightness")?.addEventListener("input", (event) => {
          const output = document.getElementById("wizard-brightness-value");
          if (output) output.textContent = `${event.target.value}%`;
        });
      }

      function renderPageWizardStep(wizard) {
        const page = wizard.page;
        if (wizard.step === 1) return `<div class="wizard-step-grid">
          ${field("wizard-page-name", t("pageName"), "text", "", page.name || "")}
          ${selectHtml("wizard-source-type", t("pageSource"), page.source_type || "url", [["url", t("webAddress")], ["home_assistant", t("homeAssistantDashboard")]])}
          <div class="span-2">${field("wizard-page-url", t("pageUrl"), "url", "", page.url || "")}</div>
          <div class="span-2">${switchHtml("wizard-page-disabled", t("disablePage"), !!page.disabled)}</div>
          <div class="span-2 wizard-callout"><strong>${esc(t("validation"))}</strong><span>${esc(t("urlValidationHint"))}</span></div>
        </div>`;
        if (wizard.step === 2) {
          const mode = page.display_mode || "duration";
          return `<div class="wizard-step-grid">
            <div class="span-2">${selectHtml("wizard-display-mode", t("displayMode"), mode, [["duration", t("customDuration")], ["schedule", t("fixedSchedule")], ["mqtt", t("triggerBased")]])}</div>
            ${mode === "duration" ? field("wizard-duration", t("durationSeconds"), "number", "", page.duration_seconds || 60) : ""}
            ${mode === "schedule" ? `${field("wizard-schedule-start", t("start"), "time", "", page.schedule?.start || "08:00")}${field("wizard-schedule-end", t("end"), "time", "", page.schedule?.end || "18:00")}<div class="span-2">${renderWizardDayPicker(page.schedule?.days || [])}</div>` : ""}
            ${mode === "mqtt" ? `${field("wizard-trigger-topic", t("mqttTopic"), "text", "", page.trigger?.topic || "")}${field("wizard-trigger-payload", t("mqttPayload"), "text", "", page.trigger?.payload || "ON")}` : ""}
            <details class="span-2 wizard-options" open><summary>${esc(t("advancedDisplayActions"))}</summary><div class="wizard-option-grid">
              ${switchHtml("wizard-power-off", t("powerOffAfterExpiry"), mode === "schedule" ? page.display_options?.power_off_after !== false : !!page.display_options?.power_off_after)}
              <label class="range-field"><span>${esc(t("brightness"))}</span><input id="wizard-brightness" type="range" min="0" max="100" value="${esc(page.display_options?.brightness ?? 100)}"><strong id="wizard-brightness-value">${esc(page.display_options?.brightness ?? 100)}%</strong></label>
            </div></details>
          </div>`;
        }
        const pages = collectPages();
        const nextIndex = wizard.index === null ? Math.min(wizard.insertAt, pages.length - 1) : (wizard.index + 1) % Math.max(1, pages.length);
        const next = pages[nextIndex];
        return `<div class="wizard-review">
          <div class="review-visual"><span>${esc(page.source_type === "home_assistant" ? "HA" : "WEB")}</span><strong>${esc(page.name)}</strong><small>${esc(page.url)}</small></div>
          <dl><div><dt>${esc(t("displayMode"))}</dt><dd>${esc(page.display_mode === "schedule" ? t("fixedSchedule") : page.display_mode === "mqtt" ? t("triggerBased") : t("customDuration"))}</dd></div><div><dt>${esc(t("timing"))}</dt><dd>${esc(pageTimingLabel(page))}</dd></div><div><dt>${esc(t("nextPage"))}</dt><dd>${esc(next?.name || (pages.length ? pages[0]?.name : t("loop")))}</dd></div><div><dt>${esc(t("brightness"))}</dt><dd>${esc(page.display_options?.brightness ?? 100)}%</dd></div></dl>
          <div class="wizard-summary">${esc(t("workflowReview").replace("{name}", page.name).replace("{timing}", pageTimingLabel(page)).replace("{next}", next?.name || t("loop")))}</div>
        </div>`;
      }

      function renderWizardDayPicker(selected) {
        const active = new Set((selected || []).map((day) => String(day).slice(0, 3).toLowerCase()));
        return `<fieldset class="day-picker"><legend>${esc(t("days"))}</legend><div>${["mon", "tue", "wed", "thu", "fri", "sat", "sun"].map((day) => `<label><input id="wizard-day-${day}" type="checkbox" ${active.has(day) ? "checked" : ""}><span>${esc(t(`dayShort_${day}`))}</span></label>`).join("")}</div></fieldset>`;
      }

      function readPageWizardStep(validate = false) {
        const wizard = state.pageWizard;
        if (!wizard) return;
        const page = wizard.page;
        if (wizard.step === 1) {
          page.name = val("wizard-page-name") || page.name;
          page.source_type = val("wizard-source-type") || page.source_type;
          page.url = val("wizard-page-url") || page.url;
          page.disabled = checked("wizard-page-disabled");
          if (validate && !String(page.name || "").trim()) validationError("wizard-page-name", t("validationPageName"));
          if (validate) {
            try { const parsed = new URL(page.url); if (!["http:", "https:", "file:"].includes(parsed.protocol)) throw new Error("protocol"); }
            catch { validationError("wizard-page-url", t("validationPageUrl")); }
          }
        }
        if (wizard.step === 2) {
          page.display_mode = val("wizard-display-mode") || page.display_mode || "duration";
          page.duration_seconds = Number(val("wizard-duration") || page.duration_seconds || 60);
          page.schedule = { start: val("wizard-schedule-start") || page.schedule?.start || "08:00", end: val("wizard-schedule-end") || page.schedule?.end || "18:00", days: ["mon", "tue", "wed", "thu", "fri", "sat", "sun"].filter((day) => checked(`wizard-day-${day}`)) };
          page.trigger = { topic: val("wizard-trigger-topic") || page.trigger?.topic || "", payload: val("wizard-trigger-payload") || page.trigger?.payload || "ON" };
          page.display_options = {
            power_off_after: page.display_mode === "schedule" ? (document.getElementById("wizard-power-off") ? checked("wizard-power-off") : true) : checked("wizard-power-off"),
            screensaver: !!page.display_options?.screensaver,
            brightness: Number(val("wizard-brightness") || page.display_options?.brightness || 100),
          };
          if (validate && page.display_mode === "duration" && page.duration_seconds < 5) validationError("wizard-duration", t("validationDuration"));
          if (validate && page.display_mode === "schedule" && (!page.schedule.start || !page.schedule.end)) throw new Error(t("validationTime"));
          if (validate && page.display_mode === "mqtt" && !page.trigger.topic.trim()) validationError("wizard-trigger-topic", t("validationMQTTTopic"));
        }
      }

      async function commitPageWizard(startKiosk) {
        try {
          const wizard = state.pageWizard;
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.kiosk.pages = collectPages();
          const page = { ...wizard.page, schedule: { ...(wizard.page.schedule || {}) }, trigger: { ...(wizard.page.trigger || {}) }, display_options: { ...(wizard.page.display_options || {}) } };
          if (wizard.index === null) cfg.kiosk.pages.splice(Math.max(0, Math.min(wizard.insertAt, cfg.kiosk.pages.length)), 0, page);
          else cfg.kiosk.pages[wizard.index] = page;
          synchronizeKioskWorkflow(cfg.kiosk);
          state.config = cfg;
          state.kioskSelectedPageIndex = wizard.index === null ? Math.max(0, Math.min(wizard.insertAt, cfg.kiosk.pages.length - 1)) : wizard.index;
          state.pageWizard = null;
          closeModal();
          markDirty();
          renderApp();
          if (startKiosk) await saveKiosk(true, true);
          else toast(t("unsavedChanges"), t("pageWizardSaveReminder"), "warn");
        } catch (error) {
          toast(t("validationFailed"), error.message, "error");
        }
      }

      function addKioskPage(homeAssistant) {
        const cfg = cloneConfig();
        cfg.kiosk = cfg.kiosk || {};
        cfg.kiosk.pages = collectPages();
        cfg.kiosk.pages.push({
          name: homeAssistant ? "Home Assistant" : `Kiosk ${cfg.kiosk.pages.length + 1}`,
          url: "http://homeassistant.local:8123",
          disabled: false,
        });
        syncKioskURLs(cfg.kiosk);
        state.config = cfg;
        markDirty();
        renderApp();
        requestAnimationFrame(() => document.getElementById(`page-name-${cfg.kiosk.pages.length - 1}`)?.focus());
      }

      function applyPageFilter(query) {
        const filter = String(query || "").trim().toLowerCase();
        let visible = 0;
        document.querySelectorAll("[data-page-index]").forEach((item) => {
          const index = Number(item.dataset.pageIndex);
          const text = `${val(`page-name-${index}`)} ${val(`page-url-${index}`)}`.toLowerCase();
          const matches = !filter || text.includes(filter);
          item.hidden = !matches;
          if (matches) visible++;
        });
        const counter = document.getElementById("visible-pages-count");
        if (counter) counter.textContent = `${t("visiblePages")}: ${visible}`;
      }

      async function applySafeMode() {
        await runAction("safe-mode", async () => {
          const result = await postJSON("/api/browser/safe-mode", { restart: true });
          state.config = result.config || state.config;
          await refreshCore();
          renderApp();
        }, t("saved"));
      }

      async function loadBrowserDiagnostics() {
        await runAction("browser-diagnostics", async () => {
          state.diagnostics = await getJSON("/api/browser/diagnostics");
          if (state.view === "dashboard") {
            openModal(`
              <div class="modal" role="dialog" aria-modal="true" aria-labelledby="diag-title">
                <div class="modal-head">
                  <div>
                    <h3 id="diag-title">${esc(t("browserDiagnostics"))}</h3>
                    <div class="hint">${esc(state.diagnostics?.command || "")}</div>
                  </div>
                  <button data-modal-close>${esc(t("close"))}</button>
                </div>
                <div class="modal-body">
                  <pre class="logbox">${esc(JSON.stringify(state.diagnostics, null, 2))}</pre>
                </div>
                <div class="modal-foot"><button data-modal-close>${esc(t("close"))}</button></div>
              </div>`);
          } else {
            renderApp();
          }
        }, t("testBrowser"));
      }

      async function saveKiosk(restart, skipConfirm = false) {
        if (restart && !skipConfirm && !confirm(t("confirmRestart"))) return;
        await runAction(restart ? "kiosk-save-restart" : "kiosk-save", async () => {
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.kiosk.pages = collectPages();
          validatePages(cfg.kiosk.pages);
          cfg.kiosk.scheduler = cfg.kiosk.scheduler || {};
          cfg.kiosk.scheduler.tick_interval = durationToNs(val("scheduler-tick") || secondsToDuration(cfg.kiosk.scheduler.tick_interval, 15));
          synchronizeKioskWorkflow(cfg.kiosk);
          // Honor an explicit disable only when the checkbox is present and unchecked
          // after sync. Sync enables the scheduler whenever schedule/rotation pages exist,
          // so first-save of a Zeitplan no longer leaves enabled=false.
          const schedulerToggle = document.getElementById("scheduler-enabled");
          if (schedulerToggle && !schedulerToggle.checked && !(cfg.kiosk.time_rules || []).length && !(cfg.kiosk.rotation || []).length) {
            cfg.kiosk.scheduler.enabled = false;
          } else if (schedulerToggle && !schedulerToggle.checked && state.persistedConfig?.kiosk?.scheduler?.enabled) {
            cfg.kiosk.scheduler.enabled = false;
          }
          validateScheduler(cfg.kiosk);
          await postJSON("/api/config", cfg);
          if (restart) await postJSON(state.status?.browser?.running ? "/api/browser/restart" : "/api/browser/start");
          await refreshCore();
          clearDirty("kiosk-pages");
          renderApp();
        }, t("saved"));
      }

      async function checkSelectedPage() {
        const pages = collectPages();
        const active = Number(document.querySelector("[data-page-active]:checked")?.value || 0);
        const out = document.getElementById("page-check-output");
        await runAction("page-check", async () => {
          const result = await postJSON("/api/browser/check-page", { index: active, url: pages[active]?.url || "" });
          const detail = result.hint || (result.statusCode === 403 ? t("haForbiddenHint") : (result.error || result.status || ""));
          out.textContent = result.ok ? `${t("pageReachable")}: ${result.status || result.url}` : `${t("pageFailed")}: ${detail}`;
        }, t("checkPage"));
      }

      async function renderCheckSelectedPage() {
        const pages = collectPages();
        const active = Number(document.querySelector("[data-page-active]:checked")?.value || 0);
        const out = document.getElementById("page-check-output");
        await runAction("render-check", async () => {
          if (out) out.textContent = `${t("loading")}...`;
          const result = await postJSON("/api/browser/render-check", { index: active, url: pages[active]?.url || "" });
          const ratio = result.analysis?.blank_ratio !== undefined ? ` (${Math.round(result.analysis.blank_ratio * 1000) / 10}% blank)` : "";
          if (out) out.textContent = result.ok ? `${t("pageVisible")}${ratio}` : `${t("pageBlank")}${ratio}: ${result.error || result.output_tail || ""}`;
          if (!result.ok) throw new Error(result.error || t("pageBlank"));
        }, t("renderCheck"));
      }

      async function checkAllPages() {
        const pages = collectPages();
        const output = document.getElementById("page-check-all-output");
        await runAction("page-check-all", async () => {
          if (output) output.innerHTML = "";
          for (let index = 0; index < pages.length; index++) {
            const page = pages[index];
            if (!page.url || page.disabled) continue;
            const result = await postJSON("/api/browser/check-page", { index, url: page.url });
            if (output) {
              output.insertAdjacentHTML("beforeend", `
                <div class="check-row">
                  <strong>${esc(page.name || `${t("pages")} ${index + 1}`)}</strong>
                  <span class="status-url">${esc(result.url || page.url)}</span>
                  <span class="chip ${result.ok ? "ok" : "bad"}">${esc(result.hint || (result.statusCode === 403 ? t("haForbiddenHint") : result.ok ? (result.status || t("success")) : (result.error || result.status || t("failed"))))}</span>
                </div>`);
            }
          }
        }, t("checkAllPages"));
      }

      function bulkSetPagesDisabled(disabled) {
        const cfg = cloneConfig();
        cfg.kiosk = cfg.kiosk || {};
        cfg.kiosk.pages = collectPages().map((page) => ({ ...page, disabled }));
        syncKioskURLs(cfg.kiosk);
        state.config = cfg;
        markDirty();
        renderApp();
      }

      function exportPages() {
        const payload = JSON.stringify({ pages: collectPages() }, null, 2);
        const blob = new Blob([payload + "\n"], { type: "application/json" });
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = url;
        link.download = "kioskmate-pages.json";
        link.click();
        URL.revokeObjectURL(url);
        recordAction(t("exportPages"), "kioskmate-pages.json", "ok");
      }

      async function importPages(event) {
        const file = event.target.files?.[0];
        if (!file) return;
        await runAction("pages-import", async () => {
          const data = JSON.parse(await file.text());
          const pages = Array.isArray(data) ? data : data.pages;
          if (!Array.isArray(pages)) throw new Error("Invalid pages file");
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.kiosk.pages = normalizePages(pages.map((page, index) => ({ ...page, name: String(page.name || `Kiosk ${index + 1}`), url: String(page.url || "") })), [])
            .filter((page) => page.name || page.url);
          synchronizeKioskWorkflow(cfg.kiosk);
          state.config = cfg;
          markDirty();
          renderApp();
        }, t("importPages"));
      }

      function buildRotationFromPages() {
        const cfg = cloneConfig();
        cfg.kiosk = cfg.kiosk || {};
        cfg.kiosk.pages = collectPages();
        cfg.kiosk.rotation = cfg.kiosk.pages
          .map((page, index) => ({ page, index }))
          .filter((item) => !item.page.disabled && item.page.url)
          .map((item) => ({ page: item.index, duration_seconds: 3600 }));
        state.config = cfg;
        markDirty("kiosk-pages");
        renderApp();
      }

      function clearKioskList(key) {
        const cfg = cloneConfig();
        cfg.kiosk = cfg.kiosk || {};
        cfg.kiosk.pages = collectPages();
        cfg.kiosk[key] = [];
        state.config = cfg;
        markDirty("kiosk-pages");
        renderApp();
      }

      async function activateSelectedPage() {
		const pages = collectPages();
		const active = enabledPageIndexFromAbsolute(pages, selectedKioskPageIndex(pages));
        await runAction("page-activate", async () => {
          if (active < 0) throw new Error(t("validationPageDisabledForActivation"));
          // Temporary override so the scheduler does not immediately steal the page back.
          await postJSON("/api/browser/override", { page: active, duration_seconds: 3600, source: "admin-activate" });
          await refreshCore();
          renderApp();
        });
      }

	  async function applyPageOverride() {
		const pages = collectPages();
		const page = enabledPageIndexFromAbsolute(pages, selectedKioskPageIndex(pages));
		const durationMinutes = Math.max(1, Math.min(1440, Number(val("override-duration") || 60)));
		await runAction("override-apply", async () => {
		  if (page < 0) throw new Error(t("validationPageDisabledForActivation"));
		  await postJSON("/api/browser/override", { page, duration_seconds: durationMinutes * 60, source: "admin" });
		  clearSnapshot();
		  await refreshCore();
		  renderApp();
		}, t("overrideApplied"));
	  }

	  async function clearPageOverride() {
		await runAction("override-clear", async () => {
		  await request("/api/browser/override", { method: "DELETE" });
		  await refreshCore();
		  renderApp();
		}, t("overrideEnded"));
	  }

      function openPreview() {
        const pages = collectPages();
        const active = Number(document.querySelector("[data-page-active]:checked")?.value || 0);
        const url = pages[active]?.url || "";
        if (!url) return;
        window.open(url, "_blank", "noopener");
        const out = document.getElementById("page-check-output");
        if (out) out.textContent = url;
      }

      function syncKioskURLs(kiosk) {
        kiosk.urls = (kiosk.pages || []).filter((p) => !p.disabled && p.url).map((p) => p.url);
      }

      function synchronizeKioskWorkflow(kiosk) {
        kiosk.pages = normalizePages(kiosk.pages, kiosk.urls);
        syncKioskURLs(kiosk);
        const rotation = [];
        const rules = [];
        let enabledIndex = 0;
        kiosk.pages.forEach((page) => {
          if (page.disabled || !page.url) return;
          if (page.display_mode === "schedule" && page.display_options?.power_off_after == null) {
            page.display_options = { ...(page.display_options || {}), power_off_after: true };
          }
          if ((page.display_mode || "duration") === "duration") rotation.push({ page: enabledIndex, duration_seconds: Math.max(5, Number(page.duration_seconds || 60)) });
          if (page.display_mode === "schedule") rules.push({ name: page.name || `Page ${enabledIndex + 1}`, page: enabledIndex, start: normalizeClock(page.schedule?.start || "08:00"), end: normalizeClock(page.schedule?.end || "18:00"), days: [...(page.schedule?.days || [])], disabled: false });
          enabledIndex++;
        });
        kiosk.rotation = rotation;
        kiosk.time_rules = rules;
        kiosk.scheduler = kiosk.scheduler || {};
        kiosk.scheduler.mode = rules.length && rotation.length ? "hybrid" : rules.length ? "time" : "rotation";
        // Schedule/rotation workflows are inert unless the scheduler is enabled.
        if (rules.length > 0 || rotation.length > 0) {
          kiosk.scheduler.enabled = true;
        }
        if (!kiosk.scheduler.tick_interval) {
          kiosk.scheduler.tick_interval = durationToNs(15);
        }
      }

      function normalizeClock(value) {
        const parts = String(value || "").trim().split(":");
        if (parts.length < 2) return "08:00";
        const hour = Math.max(0, Math.min(23, Number(parts[0]) || 0));
        const minute = Math.max(0, Math.min(59, Number(parts[1]) || 0));
        return `${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`;
      }

      function movePage(kiosk, index, direction) {
        const pages = kiosk.pages || [];
        const next = index + direction;
        if (index < 0 || next < 0 || index >= pages.length || next >= pages.length) return;
        const order = pages.map((_, i) => i);
        [order[index], order[next]] = [order[next], order[index]];
        kiosk.pages = order.map((oldIndex) => pages[oldIndex]);
        remapPageIndexes(kiosk, order, pages);
        syncKioskURLs(kiosk);
      }

      function removePage(kiosk, index) {
        const pages = kiosk.pages || [];
        const order = pages.map((_, i) => i).filter((oldIndex) => oldIndex !== index);
        kiosk.pages = order.map((oldIndex) => pages[oldIndex]);
        remapPageIndexes(kiosk, order, pages);
        syncKioskURLs(kiosk);
      }

      // rotation[].page and time_rules[].page are stored in enabled-page index space
      // (see synchronizeKioskWorkflow), not absolute pages-array position. `order` maps
      // new absolute position -> old absolute position; `previousPages` is the pages
      // array as it was before the reorder/removal was applied. Rebuild an
      // old-enabled-index -> new-enabled-index map from that before touching rotation
      // or time_rules, otherwise moving/removing a page silently repoints unrelated rules.
      function remapPageIndexes(kiosk, order, previousPages) {
        const oldPages = previousPages || [];
        const oldEnabledByAbsolute = new Map();
        let oldEnabled = 0;
        oldPages.forEach((page, absoluteIndex) => {
          if (!page.disabled && page.url) {
            oldEnabledByAbsolute.set(absoluteIndex, oldEnabled);
            oldEnabled++;
          }
        });
        const newPages = kiosk.pages || [];
        let newEnabled = 0;
        const enabledMap = new Map();
        newPages.forEach((page, newAbsoluteIndex) => {
          const oldAbsoluteIndex = order[newAbsoluteIndex];
          const isEnabled = !page.disabled && page.url;
          if (isEnabled) {
            if (oldEnabledByAbsolute.has(oldAbsoluteIndex)) {
              enabledMap.set(oldEnabledByAbsolute.get(oldAbsoluteIndex), newEnabled);
            }
            newEnabled++;
          }
        });
        const remap = (page) => enabledMap.has(Number(page)) ? enabledMap.get(Number(page)) : 0;
        kiosk.rotation = (kiosk.rotation || []).map((item) => ({ ...item, page: remap(item.page) }));
        kiosk.time_rules = (kiosk.time_rules || []).map((rule) => ({ ...rule, page: remap(rule.page) }));
      }

      function bindScheduler() {
        document.querySelector('[data-action="scheduler-save"]')?.addEventListener("click", saveScheduler);
        document.querySelector('[data-action="rotation-build"]')?.addEventListener("click", buildRotationFromPages);
        document.querySelector('[data-action="rotation-clear"]')?.addEventListener("click", () => clearKioskList("rotation"));
        document.querySelector('[data-action="rules-clear"]')?.addEventListener("click", () => clearKioskList("time_rules"));
        document.querySelector('[data-action="rotation-add"]')?.addEventListener("click", () => {
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.kiosk.rotation = collectRotation();
          cfg.kiosk.rotation.push({ page: 0, duration_seconds: 3600 });
          state.config = cfg;
          markDirty();
          renderApp();
        });
        document.querySelector('[data-action="rule-add"]')?.addEventListener("click", () => {
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.kiosk.time_rules = collectRules();
          cfg.kiosk.time_rules.push({ name: "Dashboard", page: 0, start: "13:00", end: "14:00", days: [], disabled: false });
          state.config = cfg;
          markDirty();
          renderApp();
        });
        document.querySelectorAll("[data-rotation-remove]").forEach((button) => button.addEventListener("click", () => {
          const cfg = cloneConfig();
          cfg.kiosk.rotation = collectRotation().filter((_, i) => i !== Number(button.dataset.rotationRemove));
          state.config = cfg;
          markDirty();
          renderApp();
        }));
        document.querySelectorAll("[data-rule-remove]").forEach((button) => button.addEventListener("click", () => {
          const cfg = cloneConfig();
          cfg.kiosk.time_rules = collectRules().filter((_, i) => i !== Number(button.dataset.ruleRemove));
          state.config = cfg;
          markDirty();
          renderApp();
        }));
      }

      async function saveScheduler() {
        await runAction("scheduler-save", async () => {
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.kiosk.scheduler = { enabled: checked("scheduler-enabled"), mode: val("scheduler-mode"), tick_interval: durationToNs(val("scheduler-tick")) };
          cfg.kiosk.rotation = collectRotation();
          cfg.kiosk.time_rules = collectRules();
          // Pages stay the source of truth for schedule windows: mirror the edited
          // rules back onto their matching pages instead of calling
          // synchronizeKioskWorkflow (which would rebuild rotation/time_rules from the
          // pages and wipe out any Zeitplan-only edits made here).
          mirrorRulesToSchedulePages(cfg.kiosk);
          validateScheduler(cfg.kiosk);
          await postJSON("/api/config", cfg);
          await refreshCore();
          clearDirty("kiosk-schedule");
          renderApp();
        }, t("saved"));
      }

      // Applies each time_rules entry's start/end/days back onto the enabled kiosk page
      // it targets, as long as that page is still in "schedule" display mode. Rules are
      // keyed by enabled-page index; mirroring keeps Pages authoritative for schedule
      // windows even when they were edited from the Zeitplan/scheduler view.
      function mirrorRulesToSchedulePages(kiosk) {
        const pages = normalizePages(kiosk.pages, kiosk.urls);
        (kiosk.time_rules || []).forEach((rule) => {
          const absoluteIndex = absolutePageIndexFromEnabled(pages, Number(rule.page));
          const page = absoluteIndex >= 0 ? pages[absoluteIndex] : null;
          if (page && page.display_mode === "schedule") {
            page.schedule = { start: normalizeClock(rule.start), end: normalizeClock(rule.end), days: [...(rule.days || [])] };
          }
        });
        kiosk.pages = pages;
        syncKioskURLs(kiosk);
      }
