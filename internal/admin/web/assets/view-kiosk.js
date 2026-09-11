"use strict";

function renderKiosk() {
        const kiosk = state.config?.kiosk || {};
        const browser = state.status?.browser || {};
        const pages = normalizePages(kiosk.pages, kiosk.urls);
        const enabledPages = pages.filter((page) => !page.disabled && page.url).length;
        const selected = selectedKioskPageIndex(pages);
        const current = pages[selected] || {};
		const override = browser.override || {};
        const workflowIssues = state.status?.config?.workflow_issues || [];
        return `
          <div class="page-stack kiosk-workspace">
            ${workflowIssues.map((issue) => stateBanner(issue.severity === "error" ? "bad" : "warn", t("workflowIssue"), workflowIssueMessage(issue))).join("")}
            <section class="kiosk-commandbar">
              <div class="section-summary">
                <strong>${esc(t("kioskSequence"))}</strong>
                <span>${esc(t("kioskSequenceHint"))}</span>
              </div>
              <div class="actions">
                <div class="segmented" role="group" aria-label="${esc(t("editorView"))}">
                  <button data-kiosk-mode="storybook" class="${state.kioskEditorMode === "storybook" ? "active" : ""}">${esc(t("storybookView"))}</button>
                  <button data-kiosk-mode="flow" class="${state.kioskEditorMode === "flow" ? "active" : ""}">${esc(t("flowView"))}</button>
                </div>
                <button class="primary" data-action="page-wizard-new">+ ${esc(t("newPage"))}</button>
              </div>
            </section>
            ${!kiosk.scheduler?.enabled && (kiosk.time_rules || []).length ? stateBanner("warn", t("scheduler"), t("scheduleNeedsEnable"), "") : ""}
            <section class="kiosk-overview" aria-label="${esc(t("workflowStatus"))}">
              <div><span>${esc(t("pages"))}</span><strong>${enabledPages} / ${pages.length}</strong></div>
              <div><span>${esc(t("scheduler"))}</span><strong class="${kiosk.scheduler?.enabled ? "ok-text" : "warn-text"}">${esc(kiosk.scheduler?.enabled ? t("active") : t("disabled"))}</strong></div>
              <div><span>${esc(t("currentPage"))}</span><strong>${esc(browser.page_name || current.name || "-")}</strong></div>
              <div><span>${esc(t("nextSwitch"))}</span><strong>${esc(formatDate(browser.scheduler?.next_switch))}</strong></div>
			  <div><span>${esc(t("temporaryOverride"))}</span><strong class="${override.active ? "warn-text" : ""}">${esc(override.active ? `${pages[override.page]?.name || override.page + 1} · ${formatDate(override.until)}` : t("inactive"))}</strong></div>
            </section>
            <section class="sequence-canvas ${state.kioskEditorMode === "flow" ? "flow-mode" : "storybook-mode"}">
              <div class="sequence-canvas-head">
                <div><h3>${esc(state.kioskEditorMode === "flow" ? t("visualWorkflow") : t("linearSequence"))}</h3><span>${esc(state.kioskEditorMode === "flow" ? t("flowViewHint") : t("storybookViewHint"))}</span></div>
                ${switchHtml("scheduler-enabled", t("runAutomatically"), !!kiosk.scheduler?.enabled)}
              </div>
              ${state.kioskEditorMode === "flow" ? renderKioskFlow(pages, selected) : renderKioskStorybook(pages, selected)}
            </section>
            <section class="selected-page-bar">
              <div>
                <span>${esc(t("selectedPage"))}</span>
                <strong>${esc(current.name || t("noPagesConfigured"))}</strong>
                <small>${esc(current.url || t("addFirstPageHint"))}</small>
              </div>
              <div class="actions">
                <button data-action="page-activate" ${pages.length ? "" : "disabled"} title="${esc(t("activatePageHint"))}">${esc(t("activatePage"))}</button>
                <button data-action="page-check" ${pages.length ? "" : "disabled"} title="${esc(t("checkPageHint"))}">${esc(t("checkPage"))}</button>
                <button data-action="page-wizard-edit" ${pages.length ? "" : "disabled"}>${esc(t("editPage"))}</button>
              </div>
            </section>
            <div id="page-check-output" class="action-feedback">${esc(current.url || browser.url || "-")}</div>
			<section class="override-bar">
			  <div><strong>${esc(t("temporaryOverride"))}</strong><span>${esc(t("temporaryOverrideHint"))}</span></div>
			  <label><span>${esc(t("durationMinutes"))}</span><input id="override-duration" type="number" min="1" max="1440" value="60"></label>
			  <button class="primary" data-action="override-apply" ${pages.length ? "" : "disabled"}>${esc(t("showSelectedTemporarily"))}</button>
			  ${override.active ? `<button data-action="override-clear">${esc(t("endOverride"))}</button>` : ""}
			</section>
            <details class="card disclosure advanced-settings">
              <summary>${esc(t("sequenceAdvanced"))}</summary>
              <div class="disclosure-body advanced-sequence-grid">
                <div class="form-grid">
                  ${field("scheduler-tick", t("tickInterval"), "number", "", secondsToDuration(kiosk.scheduler?.tick_interval, 15))}
                  <div class="readonly-field"><span>${esc(t("schedulerMode"))}</span><strong>${esc(t(kiosk.scheduler?.mode === "time" ? "timeMode" : kiosk.scheduler?.mode === "hybrid" ? "mixedMode" : "rotationMode"))}</strong></div>
                </div>
                <div class="actions">
                  ${button("renderCheck", "render-check")}
                  ${button("checkAllPages", "page-check-all")}
                  ${button("importPages", "pages-import")}
                  ${button("exportPages", "pages-export")}
                </div>
                <input id="pages-import-file" type="file" accept="application/json,.json" class="hidden" />
                <div id="page-check-all-output" class="check-panel span-2"></div>
              </div>
            </details>
            <div class="save-bar">
              <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
              <div class="actions">${button("save", "kiosk-save")}${button("saveStartKiosk", "kiosk-save-restart", "primary")}</div>
            </div>
          </div>`;
      }

      function workflowIssueMessage(issue) {
        const key = `workflowIssue_${issue?.code || "unknown"}`;
        const translated = t(key);
        const message = translated === key ? (issue?.message || issue?.code || t("unknownError")) : translated;
        return Array.isArray(issue?.pages) && issue.pages.length ? `${message} (${issue.pages.join(", ")})` : message;
      }

      function selectedKioskPageIndex(pages) {
        // state.status.browser.active is in enabled-page index space (matches backend
        // SetActive/PageCount), so it must be converted to an absolute index before it
        // can be used to index into the full (including disabled) pages array.
        const fallback = absolutePageIndexFromEnabled(pages, Number(state.status?.browser?.active || 0));
        const selected = state.kioskSelectedPageIndex === null ? fallback : Number(state.kioskSelectedPageIndex);
        return Math.max(0, Math.min(pages.length - 1, Number.isFinite(selected) ? selected : 0));
      }

      // Converts an absolute index into `pages` (which includes disabled/url-less
      // pages) into the enabled-page index space expected by /api/browser/page and
      // /api/browser/override. Returns -1 if the page at absoluteIndex is not enabled.
      function enabledPageIndexFromAbsolute(pages, absoluteIndex) {
        let enabledIndex = 0;
        for (let i = 0; i < pages.length; i++) {
          const isEnabled = !pages[i].disabled && pages[i].url;
          if (i === absoluteIndex) return isEnabled ? enabledIndex : -1;
          if (isEnabled) enabledIndex++;
        }
        return -1;
      }

      // Converts an enabled-page index (as reported by the backend, e.g. browser.active)
      // back into the absolute index used by the UI's pages array. Returns -1 if no such
      // enabled page exists.
      function absolutePageIndexFromEnabled(pages, enabledIndex) {
        let counted = 0;
        for (let i = 0; i < pages.length; i++) {
          if (!pages[i].disabled && pages[i].url) {
            if (counted === enabledIndex) return i;
            counted++;
          }
        }
        return -1;
      }

      function pageTimingLabel(page) {
        if (page.display_mode === "schedule") return `${page.schedule?.start || "--:--"} - ${page.schedule?.end || "--:--"}`;
        if (page.display_mode === "mqtt") return t("mqttTrigger");
        return formatDuration(Number(page.duration_seconds || 3600));
      }

      function renderKioskStorybook(pages, selected) {
        if (!pages.length) return `<div class="sequence-empty"><strong>${esc(t("noPagesConfigured"))}</strong><span>${esc(t("addFirstPageHint"))}</span><button class="primary" data-action="page-wizard-new">+ ${esc(t("newPage"))}</button></div>`;
        return `<div class="storybook-track" data-sequence-track>
          ${pages.map((page, index) => `${index ? `<button class="insert-page" data-page-insert="${index}" title="${esc(t("insertPageHere"))}">+</button>` : ""}${renderSequenceCard(page, index, selected)}`).join("")}
          <button class="add-page-card" data-page-insert="${pages.length}"><span>+</span><strong>${esc(t("addPage"))}</strong><small>${esc(t("addPageAtEnd"))}</small></button>
        </div>`;
      }

      function renderSequenceCard(page, index, selected) {
        let host = "";
        try { host = new URL(page.url).host; } catch { host = page.url || t("notConfigured"); }
        return `<article class="sequence-card ${selected === index ? "selected" : ""} ${page.disabled ? "disabled" : ""}" draggable="true" data-sequence-index="${index}" data-page-select="${index}">
          <div class="sequence-card-top"><span class="step-number">${index + 1}</span><span class="duration-badge">${esc(pageTimingLabel(page))}</span><button class="icon-button drag-handle" data-page-drag-handle title="${esc(t("dragToReorder"))}" aria-label="${esc(t("dragToReorder"))}">&#8942;&#8942;</button></div>
          <button class="page-preview-tile" data-page-edit="${index}" title="${esc(t("editPage"))}">
            <span class="source-mark">${page.source_type === "home_assistant" ? "HA" : "WEB"}</span>
            <strong>${esc(host)}</strong>
            <small>${esc(page.display_mode === "schedule" ? t("fixedSchedule") : page.display_mode === "mqtt" ? t("triggerBased") : t("customDuration"))}</small>
          </button>
          <div class="sequence-card-copy"><strong>${esc(page.name || `${t("pages")} ${index + 1}`)}</strong><span>${esc(page.url || t("notConfigured"))}</span></div>
          <div class="sequence-card-actions">
            <button data-page-move="${index}" data-direction="-1" ${index === 0 ? "disabled" : ""} title="${esc(t("moveUp"))}" aria-label="${esc(t("moveUp"))}">&larr;</button>
            <button data-page-move="${index}" data-direction="1" ${index === normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls).length - 1 ? "disabled" : ""} title="${esc(t("moveDown"))}" aria-label="${esc(t("moveDown"))}">&rarr;</button>
            <button data-page-edit="${index}">${esc(t("edit"))}</button>
            <button class="danger-ghost" data-page-remove="${index}" title="${esc(t("remove"))}">&times;</button>
          </div>
        </article>`;
      }

      function renderKioskFlow(pages, selected) {
        return `<div class="flow-canvas">
          <div class="flow-track">
            <div class="flow-terminal">${esc(t("flowStart"))}</div>
            ${pages.map((page, index) => `<span class="flow-arrow">&rarr;</span><article class="flow-node ${selected === index ? "selected" : ""} ${page.disabled ? "disabled" : ""}" data-page-select="${index}" data-page-edit="${index}"><span>${index + 1}</span><strong>${esc(page.name || `${t("pages")} ${index + 1}`)}</strong><small>${esc(pageTimingLabel(page))}</small></article>`).join("")}
            <span class="flow-arrow">&rarr;</span><div class="flow-terminal">${esc(kioskLoops() ? t("loop") : t("flowEnd"))}</div>
            <button class="flow-add" data-page-insert="${pages.length}">+ ${esc(t("newPage"))}</button>
          </div>
        </div>`;
      }

      function kioskLoops() {
        return (state.config?.kiosk?.rotation || []).length > 1;
      }

      function renderScheduler() {
        const kiosk = state.config?.kiosk || {};
        const browser = state.status?.browser || {};
        return `
          <div class="page-stack">
            <section class="status-strip">
              ${statusTile(t("scheduler"), kiosk.scheduler?.enabled ? t("enabled") : t("disabled"), kiosk.scheduler?.enabled ? "ok" : "", t(kiosk.scheduler?.mode === "time" ? "timeMode" : kiosk.scheduler?.mode === "hybrid" ? "mixedMode" : "rotationMode"))}
              ${statusTile(t("currentPage"), browser.page_name || "-", "", formatSchedulerReason(browser.scheduler?.reason))}
              ${statusTile(t("nextSwitch"), formatDate(browser.scheduler?.next_switch), "", browser.scheduler?.active_rule || t("noActiveRule"))}
            </section>
            <div class="card">
              <div class="head"><div><h3>${esc(t("schedulerSettings"))}</h3><span class="section-kicker">${esc(t("scheduler"))}</span></div></div>
              <div class="body schedule-settings">
                ${switchHtml("scheduler-enabled", t("enabled"), !!kiosk.scheduler?.enabled)}
                ${selectHtml("scheduler-mode", t("mode"), kiosk.scheduler?.mode || "rotation", [["rotation", t("rotationMode")], ["time", t("timeMode")], ["hybrid", t("mixedMode")]])}
                ${field("scheduler-tick", t("tickInterval"), "number", "", secondsToDuration(kiosk.scheduler?.tick_interval, 15))}
              </div>
            </div>
            <div class="schedule-columns">
              <section class="data-section">
                <div class="data-section-head"><div><h3>${esc(t("rotation"))}</h3><span class="section-kicker">${esc(t("rotationSummary"))}</span></div><div class="actions">${button("buildRotation", "rotation-build")}${button("clearRotation", "rotation-clear")}${button("addRotation", "rotation-add", "primary")}</div></div>
                <div class="data-section-body" id="rotation-list">${renderRotation(kiosk.rotation || [])}</div>
              </section>
              <section class="data-section">
                <div class="data-section-head"><div><h3>${esc(t("timeRules"))}</h3><span class="section-kicker">${esc(t("timeRuleSummary"))}</span></div><div class="actions">${button("clearRules", "rules-clear")}${button("addRule", "rule-add", "primary")}</div></div>
                <div class="data-section-body" id="rules-list">${renderRules(kiosk.time_rules || [])}</div>
              </section>
            </div>
            <div class="save-bar"><span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span><button class="primary" data-busy="scheduler-save" data-action="scheduler-save">${esc(t("save"))}</button></div>
          </div>`;
      }
