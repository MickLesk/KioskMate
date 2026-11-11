import { bindKiosk, bindScheduler, loadBrowserDiagnostics } from "./actions-kiosk.js";
import { bindPages } from "./view-pages.js";
import { bindProgram } from "./view-program.js";
import { bindDeviceHealth } from "./view-device.js";
import { bindMQTT } from "./actions-mqtt.js";
import { bindSettings } from "./actions-settings.js";
import { bindHardware, bindLogs, bindSystem, bindSystemMaintenance } from "./actions-system.js";
import { closeModal, esc, getJSON, openModal, postJSON, refreshCore, renderApp, request, runAction, state, t, toast } from "./app-core.js";
import { button } from "./renderers.js";

export function bindView() {
        switch (state.view) {
          case "dashboard":
            bindDashboard();
            break;
          case "program":
            bindKiosk();
            bindScheduler();
            bindProgram();
            break;
          case "pages":
            bindKiosk();
            bindPages();
            break;
          case "automations":
            bindKiosk();
            break;
          case "automations-mqtt":
            bindMQTT();
            break;
          case "device":
            bindHardware();
            bindSystem();
            bindDeviceHealth();
            break;
          case "device-maintenance":
            bindSystemMaintenance();
            bindSettings();
            break;
          case "device-logs":
            bindLogs();
            break;
          case "device-display":
          case "device-settings":
            bindSettings();
            break;
        }
      }

      function bindDashboard() {
        bindBrowserButtons();
        bindHardware();
        document.querySelector('[data-action="dashboard-page-check"]')?.addEventListener("click", checkDashboardPage);
        document.querySelector('[data-action="dashboard-render-check"]')?.addEventListener("click", renderCheckDashboardPage);
        document.querySelector('[data-action="dashboard-preview-open"]')?.addEventListener("click", openDashboardPreview);
        document.querySelector('[data-action="dashboard-diagnostics"]')?.addEventListener("click", loadBrowserDiagnostics);
        document.querySelector('[data-action="dashboard-snapshot-refresh"]')?.addEventListener("click", refreshDashboardSnapshot);
        document.getElementById("snapshot-auto")?.addEventListener("change", (event) => {
          state.snapshotAuto = event.target.checked;
          try { localStorage.setItem("kioskmate.snapshotAuto", state.snapshotAuto ? "1" : "0"); } catch (_) {}
          if (state.snapshotAuto) scheduleAutoSnapshot();
          renderApp();
        });
        if (state.snapshotAuto) scheduleAutoSnapshot();
        document.querySelector('[data-action="browser-doctor"]')?.addEventListener("click", loadBrowserDoctor);
        document.querySelector('[data-action="browser-recover"]')?.addEventListener("click", recoverBrowser);
		document.querySelector('[data-action="browser-auto-recover"]')?.addEventListener("click", recoverBrowserQuick);
        document.querySelector('[data-action="overview-show"]')?.addEventListener("click", () => runAction("overview-show", async () => {
          // A temporary override, so the programme does not switch straight back.
          await postJSON("/api/browser/override", { page: Number(document.getElementById("overview-page")?.value || 0), duration_seconds: 3600, source: "admin-activate" });
          clearSnapshot();
          await refreshCore();
          renderApp();
        }, t("overrideApplied")));
        const displayButton = document.querySelector('[data-action="overview-display"]');
        displayButton?.addEventListener("click", () => runAction("overview-display", async () => {
          await postJSON("/api/hardware/display", { value: displayButton.dataset.power || "ON" });
          state.hardware = await getJSON("/api/hardware");
          renderApp();
        }));
        document.querySelector('[data-action="hardware-refresh"]')?.addEventListener("click", async () => {
          state.hardware = await getJSON("/api/hardware");
          renderApp();
        });
      }

      export function bindBrowserButtons() {
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

      export function clearSnapshot() {
        if (state.snapshotURL) URL.revokeObjectURL(state.snapshotURL);
        state.snapshotURL = "";
        state.snapshotTime = "";
      }

      export function formatThemeStatus(status) {
        if (!status || !status.state) return "-";
        if (status.state === "pending") return t("pending");
        if (status.state === "failed") return `${t("failed")}: ${status.error || "-"}`;
        const mode = status.applied_dark ? t("dark") : t("light");
        return `${t("applied")}: ${status.selected_theme || status.requested_theme || "default"} / ${mode}`;
      }

      export function formatSchedulerReason(reason) {
        const key = {
          disabled: "disabled",
          time: "timeMode",
          rotation: "rotationMode",
          insert: "reasonInsert",
          "manual override": "temporaryOverride",
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

      async function fetchSnapshot() {
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
      }

      async function refreshDashboardSnapshot() {
        await runAction("dashboard-snapshot-refresh", async () => {
          await fetchSnapshot();
          renderApp();
        }, t("refreshSnapshot"));
      }

      // With "automatic" on, the overview takes a new screenshot every 30 s
      // while it is open and the tab is visible. A capture costs the Pi a
      // little CPU, so it never runs in the background.
      const SNAPSHOT_INTERVAL = 30000;
      let snapshotTimer = null;
      let snapshotBusy = false;

      function scheduleAutoSnapshot() {
        if (snapshotTimer) return;
        const tick = async () => {
          snapshotTimer = null;
          const browser = state.status?.browser || {};
          if (state.view !== "dashboard" || !state.snapshotAuto || !state.auth?.authenticated) return;
          const age = state.snapshotTime ? Date.now() - new Date(state.snapshotTime).getTime() : Infinity;
          if (!document.hidden && browser.running && browser.devtools && !snapshotBusy && age >= SNAPSHOT_INTERVAL - 1000) {
            snapshotBusy = true;
            try {
              await fetchSnapshot();
              if (state.view === "dashboard") renderApp({ background: true });
            } catch (_) {
              // The manual button shows errors; the automatic refresh stays quiet.
            } finally {
              snapshotBusy = false;
            }
          }
          snapshotTimer = setTimeout(tick, SNAPSHOT_INTERVAL);
        };
        const age = state.snapshotTime ? Date.now() - new Date(state.snapshotTime).getTime() : Infinity;
        snapshotTimer = setTimeout(tick, age >= SNAPSHOT_INTERVAL ? 300 : SNAPSHOT_INTERVAL - age);
      }

      // Doctor findings in plain language; the raw detail stays available
      // under "Technical details".
      export function doctorMessage(check) {
        const detail = check.detail || {};
        if (check.id === "video_decode" && detail.finding) {
          return t(`videoDecode_${detail.finding}`)
            .replace("{count}", detail.cma_failures || 0)
            .replace("{mb}", detail.cma_total_mb || 0)
            .replace("{gpu}", detail.gpu_mem_mb || 0)
            .replace("{file}", detail.boot_config || "config.txt");
        }
        if (check.id === "swap" && detail.finding) {
          return t(`swap_${detail.finding}`).replace("{used}", detail.swap_used_mb || 0);
        }
        if (check.id === "page_cpu" && detail.finding) {
          return t(`pageCpu_${detail.finding}`).replace("{page}", detail.page || "").replace("{share}", Math.round(Number(detail.share_percent || 0)));
        }
        if (check.id === "page_video" && detail.finding) {
          return t(`pageVideo_${detail.finding}`)
            .replace("{count}", detail.videos || 0)
            .replace("{playing}", detail.playing || 0)
            .replace("{webrtc}", detail.webrtc || 0)
            .replace("{largest}", (detail.largest || "-").replace("x", "×"));
        }
        const key = `doctor_${check.id}_${check.level}`;
        const text = t(key);
        return text === key ? check.message || check.id || "-" : text;
      }

      // Technical details as a readable list: "swap_used_mb" becomes "Swap
      // used (MB)", true/false become yes/no, lists are joined. Known keys
      // have a translation (detailKey_<key>); nesting stays as a sub-list.
      export function detailList(detail, depth = 0) {
        if (detail == null || detail === "") return "";
        if (typeof detail !== "object") return `<code>${esc(detailValue(detail))}</code>`;
        if (Array.isArray(detail)) {
          if (detail.every((item) => item == null || typeof item !== "object")) return `<code>${esc(detail.map(detailValue).join(", ") || "–")}</code>`;
          return `<ol class="detail-list">${detail.map((item) => `<li>${detailList(item, depth + 1)}</li>`).join("")}</ol>`;
        }
        const rows = Object.entries(detail).filter(([, value]) => value !== undefined && value !== null && value !== "" && !(Array.isArray(value) && !value.length));
        if (!rows.length) return "";
        if (depth > 3) return `<code>${esc(JSON.stringify(detail))}</code>`;
        return `<dl class="detail-list">${rows.map(([key, value]) => `<div><dt>${esc(detailLabel(key))}</dt><dd>${typeof value === "object" ? detailList(value, depth + 1) : esc(detailValue(value))}</dd></div>`).join("")}</dl>`;
      }

      function detailLabel(key) {
        const translated = t(`detailKey_${key}`);
        if (translated !== `detailKey_${key}`) return translated;
        const words = String(key).replace(/([a-z])([A-Z])/g, "$1 $2").split(/[_\s]+/).filter(Boolean);
        const units = { mb: "(MB)", gb: "(GB)", ms: "(ms)", c: "(°C)", percent: "(%)" };
        const last = words[words.length - 1]?.toLowerCase();
        const unit = words.length > 1 && units[last] ? ` ${units[last]}` : "";
        const text = (unit ? words.slice(0, -1) : words).join(" ").toLowerCase();
        return text.charAt(0).toUpperCase() + text.slice(1) + unit;
      }

      function detailValue(value) {
        if (value === true) return t("yes");
        if (value === false) return t("no");
        if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/.test(value)) {
          const date = new Date(value);
          if (!Number.isNaN(date.getTime())) return date.toLocaleString(state.lang || undefined);
        }
        return String(value);
      }

      // The Clipboard API only exists in secure contexts (https, localhost);
      // the Admin UI usually runs on plain http in the LAN, so fall back to
      // a hidden text field and the copy command.
      export async function copyText(text) {
        try {
          if (navigator.clipboard && window.isSecureContext) {
            await navigator.clipboard.writeText(text);
            return true;
          }
        } catch {}
        const area = document.createElement("textarea");
        area.value = text;
        area.setAttribute("readonly", "");
        area.className = "offscreen";
        document.body.append(area);
        area.select();
        let ok = false;
        try { ok = document.execCommand("copy"); } catch {}
        area.remove();
        return ok;
      }

      export async function loadBrowserDoctor() {
        await runAction("browser-doctor", async () => {
          const report = await getJSON("/api/browser/doctor", { timeout: 30000 });
          state.doctor = report;
          const order = { error: 0, warn: 1, ok: 2 };
          const sorted = [...(report.checks || [])].sort((a, b) => (order[a.level] ?? 3) - (order[b.level] ?? 3));
          const problems = sorted.filter((check) => check.level !== "ok").length;
          // Software rasterization: offer hardware mode; the telemetry then
          // compares both modes in its own rows.
          const offerHardware = sorted.some((check) => check.id === "gpu" && check.level === "warn") && state.config?.performance?.gpu_mode !== "hardware";
          const checks = sorted.map((check) => `
            <li class="health-check ${check.level === "error" ? "bad" : check.level === "warn" ? "warn" : "ok"}">
              <span class="health-mark" aria-hidden="true">${check.level === "ok" ? "✓" : "!"}</span>
              <div><strong>${esc(t(`doctorCheck_${check.id}`) === `doctorCheck_${check.id}` ? check.id : t(`doctorCheck_${check.id}`))}</strong><span>${esc(doctorMessage(check))}</span>
              ${check.detail && check.level !== "ok" && typeof check.detail !== "string" ? `<details class="tech-details"><summary>${esc(t("technicalDetails"))}</summary>${detailList(check.detail)}</details>` : ""}</div>
            </li>`).join("");
          openModal(`
            <div class="modal doctor-dialog" role="dialog" aria-modal="true" aria-labelledby="doctor-title">
              <div class="modal-head">
                <div>
                  <h3 id="doctor-title">${esc(t("browserDoctor"))}</h3>
                  <div class="hint ${problems ? "warn-text" : "ok-text"}">${esc(problems ? t("healthProblems").replace("{count}", problems) : t("healthAllGood"))}</div>
                </div>
                <button data-modal-close>${esc(t("close"))}</button>
              </div>
              <div class="modal-body">
                <ul class="health-checks">${checks}</ul>
              </div>
              <div class="modal-foot">${offerHardware ? `<button class="primary" data-gpu-hardware>${esc(t("gpuTryHardware"))}</button>` : ""}<button data-doctor-copy>${esc(t("copyReport"))}</button><button data-modal-close>${esc(t("close"))}</button></div>
            </div>`);
          document.querySelector("[data-doctor-copy]")?.addEventListener("click", async () => {
            const text = [`KioskMate ${state.auth?.version || ""} · ${state.hardware?.device?.model || ""}`, ...sorted.map((check) => `[${check.level}] ${check.id}: ${check.message || ""}${check.detail && typeof check.detail === "object" ? `\n    ${JSON.stringify(check.detail)}` : check.detail ? `\n    ${check.detail}` : ""}`)].join("\n");
            toast(await copyText(text) ? t("copied") : t("copyFailed"), "", "ok");
          });
          document.querySelector("[data-gpu-hardware]")?.addEventListener("click", () => runAction("gpu-mode", async () => {
            await request("/api/browser/gpu-mode", { method: "POST", body: JSON.stringify({ mode: "hardware" }), timeout: 70000 });
            if (state.config?.performance) state.config.performance.gpu_mode = "hardware";
            closeModal();
            await refreshCore();
          }, t("gpuHardwareStarted")));
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
