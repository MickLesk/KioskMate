"use strict";

function bindView() {
        if (state.view === "dashboard") bindDashboard();
        if (state.view === "kiosk" || state.view === "kiosk-pages") { bindKiosk(); bindScheduler(); }
        if (state.view === "kiosk-schedule") bindScheduler();
        if (state.view === "mqtt") bindMQTT();
        if (state.view === "system" || state.view === "system-maintenance") bindSystemMaintenance();
        if (state.view === "system-device") {
          bindHardware();
          bindSystem();
        }
        if (state.view === "system-terminal") bindTerminal();
        if (state.view === "system-logs") bindLogs();
        if (state.view.startsWith("settings") || state.view === "kiosk-display") bindSettings();
      }

      function bindDashboard() {
        bindBrowserButtons();
        bindHardware();
		loadDashboardSoakReport();
        document.querySelector('[data-action="dashboard-page-check"]')?.addEventListener("click", checkDashboardPage);
        document.querySelector('[data-action="dashboard-render-check"]')?.addEventListener("click", renderCheckDashboardPage);
        document.querySelector('[data-action="dashboard-preview-open"]')?.addEventListener("click", openDashboardPreview);
        document.querySelector('[data-action="dashboard-diagnostics"]')?.addEventListener("click", loadBrowserDiagnostics);
        document.querySelector('[data-action="dashboard-snapshot-refresh"]')?.addEventListener("click", refreshDashboardSnapshot);
        document.querySelector('[data-action="browser-doctor"]')?.addEventListener("click", loadBrowserDoctor);
        document.querySelector('[data-action="browser-recover"]')?.addEventListener("click", recoverBrowser);
		document.querySelector('[data-action="browser-auto-recover"]')?.addEventListener("click", recoverBrowserQuick);
        document.querySelector('[data-action="hardware-refresh"]')?.addEventListener("click", async () => {
          state.hardware = await getJSON("/api/hardware");
          renderApp();
        });
      }

	  function loadDashboardSoakReport() {
		const lastLoad = Number(state.loaded.dashboardSoakAt || 0);
		if (Date.now() - lastLoad < 60_000) return;
		state.loaded.dashboardSoakAt = Date.now();
		getJSON("/api/browser/soak-report").then((report) => {
		  state.soakReport = report;
		  if (state.view === "dashboard") renderAppIfIdle();
		}).catch(() => {
		  state.loaded.dashboardSoakAt = 0;
		});
	  }

      function bindBrowserButtons() {
        const actions = {
          "browser-start": "start",
          "browser-stop": "stop",
          "browser-restart": "restart",
          "browser-reload": "reload",
          "browser-next": "next",
          "browser-previous": "previous",
          "browser-reset-session": "reset-session",
        };
        for (const [key, action] of Object.entries(actions)) {
          document.querySelectorAll(`[data-action="${key}"]`).forEach((button) => button.addEventListener("click", () => browserAction(action, key)));
        }
        document.querySelectorAll('[data-action="browser-repair-ha"]').forEach((button) => button.addEventListener("click", repairHASession));
      }

      async function browserAction(action, key) {
        await runAction(key, async () => {
          await postJSON("/api/browser/" + action);
          clearSnapshot();
          await refreshCore();
          renderApp();
        });
      }

      function clearSnapshot() {
        if (state.snapshotURL) URL.revokeObjectURL(state.snapshotURL);
        state.snapshotURL = "";
        state.snapshotTime = "";
      }

      function formatThemeStatus(status) {
        if (!status || !status.state) return "-";
        if (status.state === "pending") return t("pending");
        if (status.state === "failed") return `${t("failed")}: ${status.error || "-"}`;
        const mode = status.applied_dark ? t("dark") : t("light");
        return `${t("applied")}: ${status.selected_theme || status.requested_theme || "default"} / ${mode}`;
      }

      function formatSchedulerReason(reason) {
        const key = {
          disabled: "disabled",
          time: "timeMode",
          rotation: "rotationMode",
          "no active time rule": "noActiveRule",
          "no rotation items": "noRotationItems",
          "unsupported mode": "unsupportedMode",
        }[String(reason || "").toLowerCase()];
        return key ? t(key) : (reason || "-");
      }

      async function repairHASession() {
        await runAction("browser-repair-ha", async () => {
          await postJSON("/api/browser/reset-session");
          await refreshCore();
          const browser = state.status?.browser || {};
          const result = await postJSON("/api/browser/check-page", { index: browser.active || 0, url: browser.url || "" });
          const detail = result.hint || (result.statusCode === 403 ? t("haForbiddenHint") : (result.error || result.status || result.url || ""));
          const output = document.getElementById("page-check-output");
          if (output) {
            output.textContent = result.ok ? `${t("pageReachable")}: ${result.status || result.url}` : `${t("pageFailed")}: ${detail}`;
          }
          renderApp();
          if (!result.ok) throw new Error(detail || t("pageFailed"));
        }, t("repairHA"));
      }

      async function checkDashboardPage() {
        const browser = state.status?.browser || {};
        const active = Number(browser.active || 0);
        const out = document.getElementById("page-check-output");
        await runAction("dashboard-page-check", async () => {
          const result = await postJSON("/api/browser/check-page", { index: active, url: browser.url || "" });
          const detail = result.hint || (result.statusCode === 403 ? t("haForbiddenHint") : (result.error || result.status || ""));
          if (out) out.textContent = result.ok ? `${t("pageReachable")}: ${result.status || result.url}` : `${t("pageFailed")}: ${detail}`;
          if (!result.ok) throw new Error(detail || t("pageFailed"));
        }, t("checkPage"));
      }

      async function renderCheckDashboardPage() {
        const browser = state.status?.browser || {};
        const active = Number(browser.active || 0);
        const out = document.getElementById("page-check-output");
        await runAction("dashboard-render-check", async () => {
          if (out) out.textContent = `${t("loading")}...`;
          const result = await postJSON("/api/browser/render-check", { index: active, url: browser.url || "" });
          const ratio = result.analysis?.blank_ratio !== undefined ? ` (${Math.round(result.analysis.blank_ratio * 1000) / 10}% blank)` : "";
          if (out) out.textContent = result.ok ? `${t("pageVisible")}${ratio}` : `${t("pageBlank")}${ratio}: ${result.error || result.output_tail || ""}`;
          if (!result.ok) throw new Error(result.error || t("pageBlank"));
        }, t("renderCheck"));
      }

      function openDashboardPreview() {
        const url = state.status?.browser?.url || "";
        if (!url) return;
        window.open(url, "_blank", "noopener");
        const out = document.getElementById("page-check-output");
        if (out) out.textContent = url;
      }

      async function refreshDashboardSnapshot() {
        await runAction("dashboard-snapshot-refresh", async () => {
          const response = await fetch("/api/browser/snapshot?refresh=1", { credentials: "same-origin" });
          if (!response.ok) {
            let detail = response.statusText;
            try { detail = (await response.json()).error || detail; } catch (_) {}
            throw new Error(detail || `HTTP ${response.status}`);
          }
          const blob = await response.blob();
          if (state.snapshotURL) URL.revokeObjectURL(state.snapshotURL);
          state.snapshotURL = URL.createObjectURL(blob);
          state.snapshotTime = response.headers.get("X-KioskMate-Snapshot-Time") || new Date().toISOString();
          renderApp();
        }, t("refreshSnapshot"));
      }

      async function loadBrowserDoctor() {
        await runAction("browser-doctor", async () => {
          const report = await getJSON("/api/browser/doctor");
          const checks = (report.checks || []).map((check) => `
            <div class="recovery-step">
              <strong>${esc(check.level || "-")} · ${esc(check.message || check.id || "-")}</strong>
              <span class="hint">${esc(typeof check.detail === "string" ? check.detail : JSON.stringify(check.detail || ""))}</span>
            </div>`).join("");
          openModal(`
            <div class="modal" role="dialog" aria-modal="true" aria-labelledby="doctor-title">
              <div class="modal-head">
                <div>
                  <h3 id="doctor-title">${esc(t("browserDoctor"))}</h3>
                  <div class="hint">${esc(t("doctorHint"))}</div>
                </div>
                <button data-modal-close>${esc(t("close"))}</button>
              </div>
              <div class="modal-body grid">
                <div class="recovery-list">${checks}</div>
                <div><label>${esc(t("diagnostics"))}</label><pre class="logbox">${esc(JSON.stringify(report.advice || [], null, 2))}</pre></div>
              </div>
              <div class="modal-foot"><button data-modal-close>${esc(t("close"))}</button></div>
            </div>`);
        }, t("browserDoctor"));
      }

      async function recoverBrowser() {
        await runAction("browser-recover", async () => {
          const report = await postJSON("/api/browser/recover", {});
          await refreshCore();
          const steps = (report.steps || []).map((step) => `
            <div class="recovery-step">
              <strong>${esc(step.level || "-")} · ${esc(step.name || "-")}</strong>
              <span class="hint">${esc(typeof step.detail === "string" ? step.detail : JSON.stringify(step.detail || ""))}</span>
            </div>`).join("");
          openModal(`
            <div class="modal" role="dialog" aria-modal="true" aria-labelledby="recover-title">
              <div class="modal-head">
                <div>
                  <h3 id="recover-title">${esc(t("recoveryWorkflow"))}</h3>
                  <div class="hint">${esc(t("recoveryWorkflowHint"))}</div>
                </div>
                <button data-modal-close>${esc(t("close"))}</button>
              </div>
              <div class="modal-body"><div class="recovery-list">${steps}</div></div>
              <div class="modal-foot"><button data-modal-close>${esc(t("close"))}</button></div>
            </div>`);
          renderApp();
        }, t("recoveryWorkflow"));
      }

	  async function recoverBrowserQuick() {
		await runAction("browser-auto-recover", async () => {
		  await postJSON("/api/browser/recovery", { reason: "requested from Admin UI" });
		  clearSnapshot();
		  await refreshCore();
		  renderApp();
		}, t("recoveryStarted"));
	  }
