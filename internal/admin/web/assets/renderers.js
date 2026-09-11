"use strict";

function renderPages(pages) {
        if (!pages.length) {
          return `<div class="empty empty-action"><strong>${esc(t("noPagesConfigured"))}</strong><button class="primary" data-action="page-add-ha">${esc(t("addHomeAssistant"))}</button></div>`;
        }
        const filter = String(state.pageFilter || "").trim().toLowerCase();
        return `<div class="page-list">${pages
          .map((page, index) => {
            const matches = !filter || String(page.name || "").toLowerCase().includes(filter) || String(page.url || "").toLowerCase().includes(filter);
            return `
            <article class="page-item ${state.status?.browser?.active === index ? "active" : ""}" data-page-index="${index}" ${matches ? "" : "hidden"}>
              <div class="page-item-index">${index + 1}</div>
              <div class="page-item-content">
                <div class="page-item-fields">
                  ${field(`page-name-${index}`, t("pageName"), "text", "", page.name || "")}
                  ${field(`page-url-${index}`, t("pageUrl"), "url", "", page.url || "")}
                </div>
                <div class="page-item-footer">
                  <div class="page-selectors">
                    <label class="inline-choice"><input data-page-active value="${index}" name="active-page" type="radio" ${state.status?.browser?.active === index ? "checked" : ""}><span>${esc(t("selectPage"))}</span></label>
                    <label class="inline-choice"><input id="page-disabled-${index}" type="checkbox" ${page.disabled ? "checked" : ""}><span>${esc(t("disabled"))}</span></label>
                  </div>
                  <div class="actions page-order-actions">
                    <button title="${esc(t("moveUp"))}" aria-label="${esc(t("moveUp"))}" data-page-move="${index}" data-direction="-1" ${index === 0 ? "disabled" : ""}>↑</button>
                    <button title="${esc(t("moveDown"))}" aria-label="${esc(t("moveDown"))}" data-page-move="${index}" data-direction="1" ${index === pages.length - 1 ? "disabled" : ""}>↓</button>
                    <button data-page-duplicate="${index}">${esc(t("duplicate"))}</button>
                    <button data-page-remove="${index}" class="danger-ghost">${esc(t("remove"))}</button>
                  </div>
                </div>
              </div>
            </article>`;
          })
          .join("")}</div>`;
      }

      function renderSchedulerStatus(scheduler) {
        return `
          <div class="status-grid">
            ${metric(t("schedulerStatus"), scheduler.enabled ? t("enabled") : t("disabled"), formatSchedulerReason(scheduler.reason))}
            ${metric(t("nextSwitch"), formatDate(scheduler.next_switch), scheduler.active_rule ? `${t("activeRule")}: ${scheduler.active_rule}` : `${t("mode")}: ${scheduler.mode || "-"}`)}
          </div>`;
      }

      function renderActionLog() {
		const persisted = (state.operations || []).slice(-4).reverse().map((item) => ({
		  title: item.action || "browser",
		  detail: item.error || item.state || "",
		  type: item.state === "failed" ? "error" : item.state === "running" ? "warn" : "ok",
		  at: item.finished || item.started,
		}));
		const journal = (state.events || []).slice(-6).map((item) => ({
		  title: [item.component, item.action].filter(Boolean).join("/") || "runtime",
		  detail: item.message || item.status || "",
		  type: item.status === "error" || item.status === "failed" || item.status === "blocked" ? "error" : item.status === "running" ? "warn" : "ok",
		  at: item.at,
		}));
		const entries = [...state.actionLog, ...persisted, ...journal].sort((a, b) => new Date(b.at) - new Date(a.at)).slice(0, 4);
		if (!entries.length) return "";
        return `
          <div class="grid">
            <strong>${esc(t("recentActions"))}</strong>
            <div class="workflow-line">
			  ${entries.map((item) => `<span class="chip ${item.type === "error" ? "bad" : item.type === "warn" ? "warn" : "ok"}" title="${esc(item.detail)}">${esc(item.title)} - ${esc(new Date(item.at).toLocaleTimeString())}</span>`).join("")}
            </div>
          </div>`;
      }

      function renderRotation(items) {
        const pages = normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls);
        if (!items.length) return `<div class="empty">${esc(t("noData"))}</div>`;
        return items.map((item, index) => `
          <div class="rotation-row">
            ${selectHtml(`rotation-page-${index}`, t("pages"), String(item.page || 0), pages.map((p, i) => [String(i), p.name || p.url || String(i)]))}
            ${field(`rotation-duration-${index}`, t("duration"), "number", "", item.duration_seconds || 3600)}
            <button data-rotation-remove="${index}" class="danger">${esc(t("remove"))}</button>
          </div>`).join("");
      }

      function renderRules(items) {
        const pages = normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls);
        if (!items.length) return `<div class="empty">${esc(t("noData"))}</div>`;
        return items.map((item, index) => `
          <div class="rule-row">
            ${field(`rule-name-${index}`, t("ruleName"), "text", "", item.name || "")}
            ${selectHtml(`rule-page-${index}`, t("pages"), String(item.page || 0), pages.map((p, i) => [String(i), p.name || p.url || String(i)]))}
            ${field(`rule-start-${index}`, t("start"), "time", "", item.start || "13:00")}
            ${field(`rule-end-${index}`, t("end"), "time", "", item.end || "14:00")}
            ${renderDayPicker(index, item.days || [])}
            <label class="switch"><span>${esc(t("disabled"))}</span><input id="rule-disabled-${index}" type="checkbox" ${item.disabled ? "checked" : ""}></label>
            <button data-rule-remove="${index}" class="danger">${esc(t("remove"))}</button>
          </div>`).join("");
      }

      function renderDayPicker(index, selected) {
        const active = new Set((selected || []).map((day) => String(day).slice(0, 3).toLowerCase()));
        const days = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];
        return `
          <fieldset class="day-picker">
            <legend>${esc(t("days"))}</legend>
            <div>${days.map((day) => `<label title="${esc(t(`day_${day}`))}"><input id="rule-day-${index}-${day}" type="checkbox" ${active.has(day) ? "checked" : ""}><span>${esc(t(`dayShort_${day}`))}</span></label>`).join("")}</div>
          </fieldset>`;
      }

      function renderWorkflowPreview(kiosk) {
        const pages = normalizePages(kiosk?.pages, kiosk?.urls);
        const rotations = kiosk?.rotation || [];
        const rules = kiosk?.time_rules || [];
        const pageLabel = (index) => {
          const page = pages[Number(index) || 0];
          return page?.name || page?.url || `${t("pages")} ${index}`;
        };
        const rotationHtml = rotations.length
          ? rotations.map((item, index) => `
              <div class="workflow-step">
                <strong>${esc(index + 1)}. ${esc(pageLabel(item.page))}</strong>
                <span class="hint">${esc(formatDuration(item.duration_seconds || 0))}</span>
              </div>`).join("")
          : `<div class="empty">${esc(t("noData"))}</div>`;
        const ruleHtml = rules.length
          ? rules.map((rule) => `<span class="chip ${rule.disabled ? "" : "ok"}">${esc(rule.name || pageLabel(rule.page))}: ${esc(rule.start || "--:--")} - ${esc(rule.end || "--:--")}</span>`).join("")
          : `<span class="hint">${esc(t("noData"))}</span>`;
        return `
          <div class="workflow-strip">
            <div class="workflow-line">${rotationHtml}</div>
            <div class="workflow-line">${ruleHtml}</div>
          </div>`;
      }

      function renderRecoveryHints(browser) {
        const hints = [
          [t("reloadBrowser"), t("reloadBrowserHint")],
          [t("repairHA"), t("repairHAHint")],
          [t("resetSession"), t("resetSessionHint")],
          [t("restartBrowser"), t("restartBrowserHint")],
        ];
        if (browser?.last_error) hints.unshift([t("lastError"), browser.last_error]);
        return `
          <div class="recovery-list">
            ${hints.map(([title, text]) => `<div class="recovery-step"><strong>${esc(title)}</strong><span class="hint">${esc(text)}</span></div>`).join("")}
          </div>`;
      }

      function renderSessions() {
        if (!state.sessions.length) return `<div class="empty">${esc(t("noData"))}</div>`;
        return table([t("device"), t("start"), t("lastExit")], state.sessions.map((s) => [s.remote || "-", formatDate(s.created), formatDate(s.last_seen)]));
      }

      function renderBackups() {
        if (!state.backups.length) return `<div class="empty">${esc(t("noData"))}</div>`;
        return table([t("configFile"), t("lastExit"), ""], state.backups.map((b) => [b.name || b.path, formatDate(b.modified), `<button data-restore="${esc(b.path)}">${esc(t("restore"))}</button>`]), true);
      }

      function renderRepair() {
        const issues = (state.repair?.issues || []).filter((issue) => issue.id !== "ok");
        if (!issues.length) return `<div class="empty">${esc(t("noConfigIssues"))}</div>`;
        return table([t("diagnostics"), t("status")], issues.map((issue) => [
          issue.message || issue.id || "-",
          issue.fixed ? t("repairChanged") : t("noRepairIssues"),
        ]));
      }

      function renderJobs() {
        if (!state.jobs.length) return t("noData");
        return state.jobs.map((job) => {
          const result = job.finished ? `exit ${job.exit_code}` : t("jobRunning");
          return `$ ${job.name} (${result})\n${(job.output || []).join("\n")}`;
        }).join("\n\n");
      }

      function renderJobsHTML() {
        if (!state.jobs.length) return `<div class="empty empty-action"><strong>${esc(t("noJobsYet"))}</strong><span>${esc(t("noJobsYetHint"))}</span></div>`;
        return renderJobList(state.jobs);
      }

      function renderUpdateJobsHTML() {
        if (!state.updateJobs.length) return `<div class="empty">${esc(t("noUpdateJobs"))}</div>`;
        return renderJobList(state.updateJobs, true);
      }

      function renderUpdatePreflight() {
        const report = state.updatePreflight;
        if (!report) return "";
        return `<div class="preflight ${report.ok ? "ok" : "bad"}">
          <strong>${esc(report.ok ? t("updatePreflightPassed") : t("updatePreflightFailed"))}</strong>
          <div>${(report.checks || []).map((check) => `<span class="${check.ok ? "ok" : "bad"}"><b>${check.ok ? "✓" : "×"}</b>${esc(t("preflight_" + check.id))}: ${esc(formatPreflightCheck(check, report))}</span>`).join("")}</div>
          <details><summary>${esc(t("privilegeScope"))} · ${esc(report.privilege_mode || "-")}</summary><ul>${(report.privilege_scope || []).map((scope) => `<li>${esc(formatPrivilegeScope(scope))}</li>`).join("")}</ul></details>
        </div>`;
      }

      function formatPrivilegeScope(scope) {
        const key = {
          "apt-get install local verified package": "privilegeScopePackage",
          "package maintainer scripts": "privilegeScopeScripts",
          "systemctl --user daemon-reload and service restart": "privilegeScopeService",
        }[scope];
        return key ? t(key) : scope;
      }

      function formatPreflightCheck(check, report) {
        const result = check.ok ? "ok" : "failed";
        if (check.id === "release") return check.ok ? t("preflightResult_release_ok").replace("{version}", report.target_version || "-") : t("preflightResult_release_failed");
        if (check.id === "platform") return check.ok ? t("preflightResult_platform_ok").replace("{platform}", check.message || "Linux") : t("preflightResult_platform_failed").replace("{platform}", check.message || "-");
        if (check.id === "asset") return check.ok ? check.message || t("ready") : t("preflightResult_asset_failed");
        if (check.id === "disk") return check.ok
          ? t("preflightResult_disk_ok").replace("{available}", formatBytes(report.available_bytes)).replace("{required}", formatBytes(report.required_bytes))
          : t("preflightResult_disk_failed").replace("{available}", formatBytes(report.available_bytes)).replace("{required}", formatBytes(report.required_bytes));
        if (check.id === "privilege") return t("preflightResult_privilege_" + result);
        if (check.id.startsWith("command_")) return t("preflightResult_command_" + result);
        return check.message || "-";
      }

      function formatBytes(value) {
        const bytes = Number(value || 0);
        if (!Number.isFinite(bytes) || bytes <= 0) return "0 MiB";
        return `${Math.ceil(bytes / (1 << 20))} MiB`;
      }

      function renderUpdateHistory() {
        const entries = state.updateHistory?.entries || [];
        if (!entries.length) return `<div class="empty">${esc(t("noUpdateHistory"))}</div>`;
        return `<div class="history-list">${entries.map((entry) => `<article>
          <div><strong>${esc(entry.action === "rollback" ? t("rollback") : t("update"))}: ${esc(entry.from_version || "-")} → ${esc(entry.target_version || "-")}</strong><span>${esc(formatDate(entry.started))}</span></div>
          <span class="chip ${entry.status === "installed" ? "ok" : entry.status === "failed" ? "bad" : "warn"}">${esc(t("updateHistory_" + entry.status))}</span>
          ${entry.error ? `<small>${esc(entry.error)}</small>` : ""}
        </article>`).join("")}</div>`;
      }

      function renderJobList(jobs, updateJob = false) {
        if (!jobs.length) return `<div class="empty">${esc(t("noData"))}</div>`;
        return jobs.map((job) => {
          const running = !job.finished;
          const duration = Math.max(0, Math.round(((job.finished ? new Date(job.finished) : new Date()) - new Date(job.started)) / 1000));
          return `<article class="job-item">
            <div class="job-head"><div><strong>${esc(updateJob ? job.name === "update-rollback" ? t("rollback") : t("installUpdate") : job.name || "-")}</strong><span>${esc(updateJob && job.stage ? t("updateStage_" + job.stage) : formatDate(job.started))} · ${esc(formatDuration(duration))}</span></div><span class="chip ${running ? "warn" : job.exit_code === 0 ? "ok" : "bad"}">${esc(running ? t("jobRunning") : job.exit_code === 0 ? t("success") : t("failed"))}</span></div>
            <pre class="logbox compact-log">${esc((job.output || []).join("\n") || t("waitingForOutput"))}</pre>
          </article>`;
        }).join("");
      }

      function validationError(id, message) {
        const input = document.getElementById(id);
        if (input) {
          input.setCustomValidity(message);
          input.reportValidity();
          input.focus();
          input.addEventListener("input", () => input.setCustomValidity(""), { once: true });
        }
        throw new Error(message);
      }

      function validatePages(pages) {
        const enabled = pages.filter((page) => !page.disabled);
        if (!enabled.length) throw new Error(t("validationEnabledPage"));
        pages.forEach((page, index) => {
          if (page.disabled && !page.name && !page.url) return;
          if (!page.name.trim()) validationError(`page-name-${index}`, t("validationPageName"));
          if (!page.url.trim()) validationError(`page-url-${index}`, t("validationPageUrl"));
          try {
            const parsed = new URL(page.url);
            if (!["http:", "https:", "file:"].includes(parsed.protocol)) throw new Error("protocol");
          } catch {
            validationError(`page-url-${index}`, t("validationPageUrl"));
          }
          if (page.display_mode === "mqtt" && !String(page.trigger?.topic || "").trim()) {
            throw new Error(t("validationMQTTTopic"));
          }
        });
      }

      function validateScheduler(kiosk) {
        const pages = normalizePages(kiosk.pages, kiosk.urls);
        const mode = kiosk.scheduler?.mode || "rotation";
        const rotation = kiosk.rotation || [];
        const rules = kiosk.time_rules || [];
        if (secondsToDuration(kiosk.scheduler?.tick_interval, 0) < 1) validationError("scheduler-tick", t("validationTick"));
        if (mode !== "time") {
          rotation.forEach((item, index) => {
            if (item.page < 0 || item.page >= pages.length) validationError(`rotation-page-${index}`, t("validationPageReference"));
            if (Number(item.duration_seconds) < 5) validationError(`rotation-duration-${index}`, t("validationDuration"));
          });
        }
        if (mode !== "rotation") {
          rules.forEach((rule, index) => {
            if (rule.disabled) return;
            if (!rule.name.trim()) validationError(`rule-name-${index}`, t("validationRuleName"));
            if (rule.page < 0 || rule.page >= pages.length) validationError(`rule-page-${index}`, t("validationPageReference"));
            if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(rule.start) || !/^([01]\d|2[0-3]):[0-5]\d$/.test(rule.end)) {
              validationError(`rule-start-${index}`, t("validationTime"));
            }
          });
        }
        if (!kiosk.scheduler?.enabled) return;
        const activeRules = rules.filter((rule) => !rule.disabled);
        const mqttTriggers = pages.filter((page) => !page.disabled && page.display_mode === "mqtt" && page.trigger?.topic).length;
        if (!mqttTriggers && ((mode === "rotation" && !rotation.length) || (mode === "time" && !activeRules.length) || (mode === "hybrid" && !rotation.length && !activeRules.length))) {
          throw new Error(t("validationScheduleEmpty"));
        }
      }

      function validateMQTT(mqtt) {
        if (!mqtt.enabled) return;
        try {
          const parsed = new URL(mqtt.url);
		  if (!["mqtt:", "mqtts:"].includes(parsed.protocol)) throw new Error("protocol");
        } catch {
          validationError("mqtt-url", t("validationBrokerUrl"));
        }
        if (!String(mqtt.node || "").trim()) validationError("mqtt-node", t("validationMQTTNode"));
        if (!String(mqtt.base_topic || "").trim()) validationError("mqtt-base-topic", t("validationMQTTTopic"));
		if (!!String(mqtt.cert_file || "").trim() !== !!String(mqtt.key_file || "").trim()) {
		  validationError("mqtt-cert-file", t("validationMQTTCertificatePair"));
		}
		const maximum = Number(mqtt.maximum_packet_size || 1048576);
		if (!Number.isFinite(maximum) || maximum < 1024 || maximum > 268435456) {
		  validationError("mqtt-max-packet", t("validationMQTTMaximumPacket"));
		}
      }

      function collectPages() {
        const existing = normalizePages(state.config?.kiosk?.pages, state.config?.kiosk?.urls);
        if (!document.getElementById("page-name-0")) return existing.map((page) => ({ ...page }));
        return existing.map((_, index) => ({
          ...existing[index],
          name: val(`page-name-${index}`),
          url: val(`page-url-${index}`),
          disabled: checked(`page-disabled-${index}`),
        })).filter((page) => page.name || page.url);
      }

      function collectRotation() {
        const items = state.config?.kiosk?.rotation || [];
        if (!document.getElementById("rotation-page-0")) return items.map((item) => ({ ...item }));
        return items.map((_, index) => ({ page: Number(val(`rotation-page-${index}`) || 0), duration_seconds: Number(val(`rotation-duration-${index}`) || 3600) }));
      }

      function collectRules() {
        const items = state.config?.kiosk?.time_rules || [];
        if (!document.getElementById("rule-name-0")) return items.map((item) => ({ ...item, days: [...(item.days || [])] }));
        const days = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];
        return items.map((_, index) => ({
          name: val(`rule-name-${index}`),
          page: Number(val(`rule-page-${index}`) || 0),
          start: val(`rule-start-${index}`),
          end: val(`rule-end-${index}`),
          days: days.filter((day) => checked(`rule-day-${index}-${day}`)),
          disabled: checked(`rule-disabled-${index}`),
        }));
      }

      function normalizePages(pages, urls) {
        if (Array.isArray(pages) && pages.length) return pages.map((p, i) => ({
          ...p,
          page_id: p.page_id || `page_${i + 1}`,
          name: p.name || `Page ${i + 1}`,
          url: p.url || "",
          source_type: p.source_type || (String(p.url || "").includes(":8123") ? "home_assistant" : "url"),
          display_mode: p.display_mode || "duration",
          duration_seconds: Number(p.duration_seconds || 3600),
          schedule: { start: p.schedule?.start || "08:00", end: p.schedule?.end || "18:00", days: [...(p.schedule?.days || [])] },
          trigger: { topic: p.trigger?.topic || "", payload: p.trigger?.payload || "ON" },
          display_options: {
            power_off_after: (p.display_mode || "duration") === "schedule"
              ? p.display_options?.power_off_after !== false
              : !!p.display_options?.power_off_after,
            screensaver: !!p.display_options?.screensaver,
            brightness: Number(p.display_options?.brightness ?? 100),
          },
          disabled: !!p.disabled,
        }));
        return (urls || []).map((url, i) => ({ page_id: `page_${i + 1}`, name: `Page ${i + 1}`, url, source_type: String(url).includes(":8123") ? "home_assistant" : "url", display_mode: "duration", duration_seconds: 3600, schedule: { start: "08:00", end: "18:00", days: [] }, trigger: { topic: "", payload: "ON" }, display_options: { brightness: 100 }, disabled: false }));
      }

      function lines(id) {
        return val(id).split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
      }

      function commandTopic() {
        const base = String(val("mqtt-base-topic") || state.config?.mqtt?.base_topic || "kioskmate").replace(/^\/+|\/+$/g, "");
        const node = String(val("mqtt-node") || state.config?.mqtt?.node || "kioskmate").replace(/^\/+|\/+$/g, "");
        return `${base}/${node}/command`;
      }

      function browserPresetOptions() {
        return [
          ["chromium", t("chromium")],
          ["chromium-lite", t("chromiumLite")],
          ["firefox", t("firefox")],
          ["webkit-cog", t("webkitCog")],
          ["epiphany", t("epiphany")],
          ["midori", t("midori")],
          ["custom", t("custom")],
        ];
      }

      function objectEntries(obj) {
        return Object.entries(obj || {}).map(([key, value]) => [key.replaceAll("_", " "), formatValue(value)]);
      }

      function metric(title, value, hint = "") {
        return `<div class="metric"><span class="muted">${esc(title)}</span><strong>${esc(value)}</strong><span class="hint">${esc(hint)}</span></div>`;
      }

      function statusTile(title, value, stateClass = "", meta = "") {
        return `<div class="status-tile ${esc(stateClass)}"><span>${esc(title)}</span><strong>${esc(value)}</strong><small>${esc(meta)}</small></div>`;
      }

	  function renderTelemetryChart(samples) {
		const points = (samples || []).slice(-48);
		if (!points.length) return `<div class="telemetry-empty">${esc(t("telemetryCollecting"))}</div>`;
		const cpuMax = Math.max(1, ...points.map((item) => Number(item.cpu_percent || 0)));
		const rssMax = Math.max(1, ...points.map((item) => Number(item.rss_mb || 0)));
		return `<div class="telemetry-chart" role="img" aria-label="${esc(t("runtimeTelemetry"))}">${points.map((item) => {
		  const level = Math.max(4, Math.round(Math.max(Number(item.cpu_percent || 0) / cpuMax, Number(item.rss_mb || 0) / rssMax) * 100));
		  return `<i style="height:${level}%" title="${esc(`${formatDate(item.at)} · ${formatValue(item.cpu_percent, "% CPU")} · ${formatValue(item.rss_mb, " MB")}`)}"></i>`;
		}).join("")}</div>`;
	  }

      function stateBanner(tone, title, message, action = "") {
        return `<section class="state-banner ${esc(tone)}" role="status">
          <div class="state-indicator" aria-hidden="true"></div>
          <div><strong>${esc(title)}</strong><span>${esc(message)}</span></div>
          ${action ? `<div class="actions">${action}</div>` : ""}
        </section>`;
      }

      function readinessItem(label, ready, detail) {
        return `<div class="readiness-item ${ready ? "ok" : "warn"}">
          <span class="readiness-mark" aria-hidden="true">${ready ? "✓" : "!"}</span>
          <div><strong>${esc(label)}</strong><span>${esc(detail)}</span></div>
        </div>`;
      }

      function healthRow(title, value, stateClass = "") {
        return `<div class="health-row"><span>${esc(title)}</span><strong class="status-text ${esc(stateClass)}">${esc(value)}</strong></div>`;
      }

      function kvTable(rows) {
        return table(["", ""], rows.map(([a, b]) => [a, formatValue(b)]));
      }

      function table(headers, rows, raw = false) {
        return `<div class="table-wrap"><table><thead><tr>${headers.map((h) => `<th>${esc(h)}</th>`).join("")}</tr></thead><tbody>${rows
          .map((row) => `<tr>${row.map((cell) => `<td>${raw ? cell : esc(cell)}</td>`).join("")}</tr>`)
          .join("")}</tbody></table></div>`;
      }

      function unsupportedControl(label) {
        return `<div class="unsupported-control"><label>${esc(label)}</label><span>${esc(t("notSupportedOnDevice"))}</span></div>`;
      }

      function textarea(id, label, value = "") {
        return `<div><label for="${esc(id)}">${esc(label)}</label><textarea id="${esc(id)}">${esc(value)}</textarea></div>`;
      }

      function button(labelKey, action, cls = "") {
		return window.KioskMateUI.button(labelKey, action, cls, t);
      }
