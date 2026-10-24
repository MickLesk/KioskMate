import { bindView } from "./actions-core.js";
import { bindScope, morph } from "./dom.js";
import { KioskMateAPI } from "./api.js";
import { I18N, loadLanguage } from "./i18n.js";
import { button, stateBanner } from "./renderers.js";
import { KioskMateUI } from "./ui-components.js";
import { icon } from "./icons.js";
import { renderAutomations } from "./view-automations.js";
import { renderDashboard } from "./view-dashboard.js";
import { renderDeviceHealth } from "./view-device.js";
import { renderMQTT } from "./view-mqtt.js";
import { renderPages } from "./view-pages.js";
import { renderProgram } from "./view-program.js";
import { renderSettingsAdmin, renderSettingsBrowser, renderSettingsConfig, renderSettingsMaintenance } from "./view-settings.js";
import { renderLogs, renderSystemActions } from "./view-system.js";
import { maybeOpenSetupWizard } from "./setup-wizard.js";

// Five areas. Automations and Device have sections, shown as tabs inside
// the area; the phone layout shows a bottom bar with the first three areas.
const NAV = [
        { id: "dashboard", label: "navOverview", hint: "navOverviewHint", icon: "overview" },
        { id: "program", label: "navProgram", hint: "navProgramHint", icon: "program" },
        { id: "pages", label: "navPages", hint: "navPagesHint", icon: "pages" },
        {
          id: "automations",
          label: "navAutomations",
          hint: "navAutomationsHint",
          icon: "automations",
          children: [
            { id: "automations", label: "workflows" },
            { id: "automations-mqtt", label: "mqttConnection" },
          ],
        },
        {
          id: "device",
          label: "navDevice",
          hint: "navDeviceHint",
          icon: "device",
          children: [
            { id: "device", label: "deviceHealth" },
            { id: "device-display", label: "displayRendering" },
            { id: "device-maintenance", label: "systemMaintenance" },
            { id: "device-logs", label: "logs" },
            { id: "device-settings", label: "settings" },
          ],
        },
      ];

      // Views of earlier versions, so a remembered view still opens.
      const VIEW_ALIASES = {
        kiosk: "program",
        "kiosk-pages": "program",
        scheduler: "program",
        "kiosk-schedule": "program",
        "kiosk-display": "device-display",
        mqtt: "automations-mqtt",
        system: "device",
        "system-device": "device",
        "system-hardware": "device",
        "system-actions": "device-maintenance",
        "system-maintenance": "device-maintenance",
        "system-logs": "device-logs",
        "system-terminal": "device-maintenance",
        settings: "device-settings",
        "settings-admin": "device-settings",
        "settings-config": "device-settings",
        "settings-browser": "device-display",
        "settings-maintenance": "device-maintenance",
        "settings-updates": "device-maintenance",
      };

      export function resolveView(view) {
        return VIEW_ALIASES[view] || view;
      }

      const root = document.getElementById("root");
      const toasts = document.getElementById("toasts");
      export const modalRoot = document.getElementById("modal-root");
      const storedTheme = localStorage.getItem("kioskmate.theme");
      function storedNumber(key, fallback) {
        try {
          const value = Number(localStorage.getItem(key));
          return Number.isFinite(value) && value > 0 ? value : fallback;
        } catch {
          return fallback;
        }
      }

      export const state = {
        lang: localStorage.getItem("kioskmate.lang") || (((navigator.language || "en").toLowerCase().startsWith("de")) ? "de" : "en"),
        theme: storedTheme === "light" ? "light" : "dark",
        themeExplicit: localStorage.getItem("kioskmate.theme.explicit") === "1",
        view: resolveView(localStorage.getItem("kioskmate.view") || "dashboard"),
        auth: null,
        config: null,
        persistedConfig: null,
        status: null,
        hardware: null,
        time: null,
        timezones: [],
        privilege: null,
        sessions: [],
        backups: [],
        update: null,
        updateHistory: { entries: [], rollback_available: false, rollback_target: "" },
        updatePreflight: null,
		telemetry: null,
        diagnostics: null,
        repair: null,
        timeline: null,
        timelineToday: null,
        programRange: storedNumber("kioskmate.programRange", 60),
        overrideMinutes: storedNumber("kioskmate.overrideMinutes", 60),
        snapshotAuto: localStorage.getItem("kioskmate.snapshotAuto") !== "0",
        pageFilter: "",
        pageKind: "all",
        kioskSelectedPageIndex: null,
        pageWizard: null,
        actionLog: [],
        operations: [],
        events: [],
        logs: [],
        logSource: localStorage.getItem("kioskmate.logSource") || "combined",
        logFilter: localStorage.getItem("kioskmate.logFilter") || "",
        logWarning: "",
        ssh: null,
        terminal: "",
        jobs: [],
        updateJobs: [],
        loaded: {},
        busy: new Set(),
        dirtyViews: new Set(),
        snapshotURL: "",
        snapshotTime: "",
        mobileNavOpen: false,
      };

	  let authCountdownTimer = null;

      const KIOSK_DIRTY = ["page-name-", "page-url-", "page-disabled-", "scheduler-", "rotation-", "rule-"];
      const DIRTY_PREFIXES = {
        program: KIOSK_DIRTY,
        pages: KIOSK_DIRTY,
        "device-display": ["kiosk-", "perf-", "watchdog-"],
        "automations-mqtt": ["mqtt-"],
        device: ["time-"],
        "device-settings": ["admin-", "config-raw"],
      };

      const KIOSK_SAVE = ["kiosk-save", "kiosk-save-restart"];
      const SAVE_ACTIONS = {
        program: KIOSK_SAVE,
        pages: KIOSK_SAVE,
        automations: KIOSK_SAVE,
        "device-display": ["browser-settings-save", "browser-settings-save-restart"],
        "automations-mqtt": ["mqtt-save"],
        "device-settings": ["admin-save", "config-raw-save"],
      };

      // Pages, programme and workflows are all saved with the kiosk config,
      // so unsaved changes in one of them belong to all three.
      export const KIOSK_VIEWS = ["program", "pages", "automations"];
      function dirtyKey(view) {
        return KIOSK_VIEWS.includes(view) ? "kiosk" : view;
      }

      document.documentElement.dataset.theme = state.theme;

      export function t(key) {
        return (I18N[state.lang] && I18N[state.lang][key]) || key;
      }

      export function esc(value) {
        return String(value ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
      }

	  export const field = KioskMateUI.field;
	  export const selectHtml = KioskMateUI.selectHtml;
	  export const switchHtml = KioskMateUI.switchHtml;

      export function val(id) {
        return document.getElementById(id)?.value ?? "";
      }

      export function checked(id) {
        return !!document.getElementById(id)?.checked;
      }

      function setBusy(name, on) {
        if (on) state.busy.add(name);
        else state.busy.delete(name);
        document.querySelectorAll(`[data-busy="${CSS.escape(name)}"]`).forEach((el) => {
          el.disabled = on;
          el.classList.toggle("busy", on);
          el.setAttribute("aria-busy", String(on));
          if (on) {
            el.dataset.idleLabel = el.textContent;
            el.textContent = t("working");
          } else if (el.dataset.idleLabel) {
            el.textContent = el.dataset.idleLabel;
            delete el.dataset.idleLabel;
          }
        });
        if (!on) updateDirtyUI();
      }

      // options.action = { label, run } adds a button (for example "Undo");
      // options.timeout overrides how long the toast stays.
      export function toast(title, message = "", type = "ok", options = {}) {
        const item = document.createElement("div");
        item.className = "toast " + (type === "error" ? "error" : type === "warn" ? "warn" : "") + (options.action ? " with-action" : "");
        item.setAttribute("role", type === "error" ? "alert" : "status");
        item.innerHTML = `<div><strong>${esc(title)}</strong>${message ? `<div class="muted">${esc(message)}</div>` : ""}</div>${options.action ? `<button class="toast-action">${esc(options.action.label)}</button>` : ""}<button class="toast-close" title="${esc(t("close"))}" aria-label="${esc(t("close"))}">&times;</button>`;
        item.querySelector(".toast-close")?.addEventListener("click", () => item.remove());
        item.querySelector(".toast-action")?.addEventListener("click", () => {
          item.remove();
          options.action.run();
        });
        toasts.appendChild(item);
        setTimeout(() => item.remove(), options.timeout || (options.action ? 8000 : 5200));
      }

      // Applies an unsaved removal right away and offers to undo it, instead
      // of asking first. before is the config as it was.
      export function offerUndo(before, title, message = "", after = () => {}) {
        toast(title, message, "ok", { action: { label: t("undo"), run: () => {
          state.config = before;
          markDirty();
          renderApp();
          after();
        } } });
      }

      let modalKeyHandler = null;

      export function openModal(html) {
        modalRoot.innerHTML = `<div class="modal-backdrop" data-modal-backdrop>${html}</div>`;
        modalRoot.querySelectorAll("[data-modal-close]").forEach((button) => button.addEventListener("click", closeModal));
        modalRoot.querySelector("[data-modal-backdrop]")?.addEventListener("click", (event) => {
          if (event.target?.dataset?.modalBackdrop !== undefined) closeModal();
        });
        modalKeyHandler = (event) => {
          if (event.key === "Escape") closeModal();
        };
        document.addEventListener("keydown", modalKeyHandler);
        requestAnimationFrame(() => modalRoot.querySelector("[data-modal-close], button, input, select, textarea")?.focus());
      }

      export function closeModal() {
        if (modalKeyHandler) document.removeEventListener("keydown", modalKeyHandler);
        modalKeyHandler = null;
        modalRoot.innerHTML = "";
      }

      export function openMQTTTestDialog() {
        openModal(`
          <div class="modal" role="dialog" aria-modal="true" aria-labelledby="mqtt-live-title">
            <div class="modal-head">
              <div>
                <h3 id="mqtt-live-title">${esc(t("mqttLiveTitle"))}</h3>
                <div class="hint">${esc(t("mqttLiveHint"))}</div>
              </div>
              <button data-modal-close>${esc(t("close"))}</button>
            </div>
            <div class="modal-body">
              <div id="mqtt-live-log" class="live-log"></div>
            </div>
            <div class="modal-foot">
              <span id="mqtt-live-summary" class="hint">${esc(t("loading"))}...</span>
              <button data-modal-close>${esc(t("close"))}</button>
            </div>
          </div>`);
      }

      export function appendMQTTEvent(event) {
        const log = document.getElementById("mqtt-live-log");
        const summary = document.getElementById("mqtt-live-summary");
        if (!log) return;
        const status = event.status || "running";
        const topic = event.topic || "";
        const topics = Array.isArray(event.published_topics) ? event.published_topics.join("\n") : "";
        const result = event.result ? JSON.stringify(event.result, null, 2) : "";
        const detail = topic || topics || result;
        const line = document.createElement("div");
        line.className = `live-line ${esc(status)}`;
        line.innerHTML = `
          <div class="status">${esc(status)}</div>
          <div><strong>${esc(event.step || "-")}</strong><div class="hint">${esc(event.elapsed_ms ?? 0)} ms</div></div>
          <div>${esc(event.message || "")}${detail ? `<code>${esc(detail)}</code>` : ""}</div>`;
        log.appendChild(line);
        line.scrollIntoView({ block: "end" });
        if (summary) summary.textContent = event.message || "";
      }

      export async function streamJSONLines(path, body, onEvent, signal) {
		return KioskMateAPI.streamJSONLines(path, body, { onEvent, signal, csrf: state.auth?.csrf });
      }

      export function formatValue(value, suffix = "") {
        if (value === null || value === undefined || value === "") return "-";
        if (typeof value === "number") {
          const rounded = Math.abs(value) >= 100 ? Math.round(value) : Math.round(value * 10) / 10;
          return String(rounded) + suffix;
        }
        if (typeof value === "boolean") return value ? t("yes") : t("no");
        if (Array.isArray(value)) return value.join(", ");
        if (typeof value === "object") return JSON.stringify(value);
        return String(value);
      }

      export function formatDate(value) {
        if (!value) return t("never");
        const date = new Date(value);
        return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString();
      }

      export function formatMQTTState(value) {
        const key = {
          connected: "connected",
          connecting: "connecting",
          auth_error: "authenticationFailed",
          error: "failed",
          disabled: "disabled",
          unavailable: "notAvailable",
        }[String(value || "disabled")];
        return t(key || "notConnected");
      }

      export function formatClock(value) {
        const date = value ? new Date(value) : new Date();
        return Number.isNaN(date.getTime()) ? "--:--:--" : date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
      }

      let clockTimer = null;
      function startHeaderClock() {
        if (clockTimer) clearInterval(clockTimer);
        const serverTime = new Date(state.time?.current_time || Date.now()).getTime();
        const offset = Number.isFinite(serverTime) ? serverTime - Date.now() : 0;
        clockTimer = setInterval(() => {
          const clock = document.getElementById("kiosk-clock");
          if (clock) clock.textContent = formatClock(Date.now() + offset);
        }, 1000);
      }

      export function secondsToDuration(value, fallback = 0) {
        const n = Number(value);
        if (!Number.isFinite(n) || n <= 0) return fallback;
        return Math.round(n / 1_000_000_000);
      }

      export function formatDuration(seconds) {
        const total = Number(seconds || 0);
        if (total >= 3600 && total % 3600 === 0) return `${total / 3600} h`;
        if (total >= 60 && total % 60 === 0) return `${total / 60} min`;
        return `${total} s`;
      }

      export function durationToNs(seconds) {
        return Math.max(0, Number(seconds || 0)) * 1_000_000_000;
      }

      export function sleep(ms) {
        return new Promise((resolve) => setTimeout(resolve, ms));
      }

      export function cloneConfig() {
        return JSON.parse(JSON.stringify(state.config || {}));
      }

      export async function request(path, options = {}) {
		try {
		  return await KioskMateAPI.request(path, { ...options, csrf: state.auth?.csrf });
		} catch (error) {
		  if (error?.name === "AbortError") throw new Error(t("requestTimeout"));
		  throw error;
		}
      }

      export const getJSON = (path, options = {}) => request(path, options);
      export const postJSON = (path, body = {}) => request(path, { method: "POST", body: JSON.stringify(body) });
      export const deleteJSON = (path) => request(path, { method: "DELETE" });

      export async function runAction(name, fn, success = t("actionDone")) {
        setBusy(name, true);
        recordAction(t("actionStarted"), name, "warn");
        try {
          const result = await fn();
          // null: fn shows its own, more specific result toast.
          if (success !== null) toast(success, "", "ok");
          recordAction(success ?? t("actionDone"), name, "ok");
          return result;
        } catch (err) {
          toast(t("failed"), err.message, "error");
          if (err.data?.browser_log || err.data?.core_log) showActionFailureDetails(err);
          recordAction(t("failed"), `${name}: ${err.message}`, "error");
          return undefined;
        } finally {
          setBusy(name, false);
        }
      }

      export function markDirty(view = state.view) {
        state.dirtyViews.add(dirtyKey(view));
        updateDirtyUI();
      }

      export function clearDirty(view = state.view) {
        state.dirtyViews.delete(dirtyKey(view));
        updateDirtyUI();
      }

      export function isDirty(view = state.view) {
        return state.dirtyViews.has(dirtyKey(view));
      }

      function updateDirtyUI() {
        const dirty = isDirty();
        document.querySelectorAll("[data-dirty-indicator]").forEach((element) => {
          element.textContent = t(dirty ? "unsavedChanges" : "allChangesSaved");
        });
        document.querySelectorAll(".save-bar").forEach((element) => element.classList.toggle("dirty", dirty));
        for (const action of SAVE_ACTIONS[state.view] || []) {
          document.querySelectorAll(`[data-action="${CSS.escape(action)}"]`).forEach((button) => {
            if (!state.busy.has(action)) button.disabled = !dirty;
          });
        }
      }

      function confirmDiscard(view = state.view) {
        if (!isDirty(view)) return true;
        if (!confirm(t("confirmDiscardChanges"))) return false;
        state.dirtyViews.delete(dirtyKey(view));
        if (state.persistedConfig) state.config = JSON.parse(JSON.stringify(state.persistedConfig));
        return true;
      }

      function bindDirtyTracking() {
        const prefixes = DIRTY_PREFIXES[state.view] || [];
        if (!prefixes.length) return;
        const content = document.querySelector(".content");
        const onChange = (event) => {
          const target = event.target;
          if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement || target instanceof HTMLTextAreaElement)) return;
          if (target.dataset.noDirty !== undefined || target.type === "file") return;
          if (prefixes.some((prefix) => target.id === prefix || target.id.startsWith(prefix))) markDirty();
        };
        content?.addEventListener("input", onChange);
        content?.addEventListener("change", onChange);
        updateDirtyUI();
      }

      window.addEventListener("beforeunload", (event) => {
        if (!state.dirtyViews.size) return;
        event.preventDefault();
        event.returnValue = "";
      });

      export function recordAction(title, detail = "", type = "ok") {
        state.actionLog.unshift({ title, detail, type, at: new Date().toISOString() });
        state.actionLog = state.actionLog.slice(0, 8);
      }

      function showActionFailureDetails(err) {
        const browserLog = Array.isArray(err.data?.browser_log) ? err.data.browser_log.join("\n") : "";
        const coreLog = Array.isArray(err.data?.core_log) ? err.data.core_log.join("\n") : "";
        const browser = err.data?.browser ? JSON.stringify(err.data.browser, null, 2) : "";
        openModal(`
          <div class="modal" role="dialog" aria-modal="true" aria-labelledby="failure-title">
            <div class="modal-head">
              <div>
                <h3 id="failure-title">${esc(t("actionFailedDetails"))}</h3>
                <div class="hint">${esc(err.message || t("failed"))}</div>
              </div>
              <button data-modal-close>${esc(t("close"))}</button>
            </div>
            <div class="modal-body grid">
              ${browser ? `<div><label>${esc(t("browserStatus"))}</label><pre class="logbox">${esc(browser)}</pre></div>` : ""}
              ${browserLog ? `<div><label>${esc(t("logBrowser"))}</label><pre class="logbox">${esc(browserLog)}</pre></div>` : ""}
              ${coreLog ? `<div><label>${esc(t("logCore"))}</label><pre class="logbox">${esc(coreLog)}</pre></div>` : ""}
            </div>
            <div class="modal-foot"><button data-modal-close>${esc(t("close"))}</button></div>
          </div>`);
      }

      export async function boot() {
        try {
          state.auth = await getJSON("/api/auth/status");
          if (!state.auth.authenticated) {
            renderLogin();
            return;
          }
		  if (state.auth.config) applyCoreState({ cfg: state.auth.config });
		  else await refreshCore(true);
          renderApp();
          startUpdateStatusPolling();
          startLiveUpdates();
		  refreshCore(false).then(() => {
			if (state.auth?.authenticated) renderAppIfIdle();
			if (state.auth?.authenticated && !isDirty()) maybeOpenSetupWizard();
		  }).catch((error) => toast(t("backgroundLoadFailed"), error.message, "warn"));
        } catch (err) {
		  renderFatal(err);
        }
      }

	  export async function refreshCore(fast = false) {
		if (fast) {
		  const cfg = await getJSON("/api/config");
		  applyCoreState({ cfg });
		  return;
		}
		try {
		  const quick = await getJSON("/api/status?fast=1", { timeout: 8000 });
		  applyCoreState({ status: quick });
		} catch (_) {}
		// Only what every view needs; the rest is loaded by ensureViewData when
		// a view that shows it is opened.
		const requests = await Promise.allSettled([
		  getJSON("/api/config"), getJSON("/api/status", { timeout: 20000 }), getJSON("/api/privilege"),
		  getJSON("/api/time"),
		]);
		const value = (index) => requests[index].status === "fulfilled" ? requests[index].value : undefined;
		applyCoreState({ cfg: value(0), status: value(1), privilege: value(2), timeInfo: value(3) });
		const failed = requests.filter((item) => item.status === "rejected");
		if (failed.length === requests.length) throw failed[0].reason;
		ensureViewData(state.view);
      }

	  // Saves the whole config and shows the conflicts the server found
	  // (overlapping time rules, workflows sharing a trigger) as warnings.
	  // A page's thumbnail image, or "" when there is none yet.
	  export function thumbnailImg(pageID, cls = "page-thumb") {
		const at = state.thumbnails?.[pageID];
		if (!at || !pageID) return "";
		return `<img class="${cls}" src="/api/pages/thumbnails/${encodeURIComponent(pageID)}?v=${encodeURIComponent(new Date(at).getTime())}" alt="" loading="lazy" decoding="async">`;
	  }

	  export async function saveConfig(cfg, force = false) {
		let result;
		try {
		  result = await postJSON(`/api/config${force ? "?force=1" : ""}`, { ...cfg, revision: state.persistedConfig?.revision || cfg.revision || "" });
		} catch (error) {
		  if (error.status !== 409 || error.data?.code !== "config_changed") throw error;
		  // Someone else saved in the meantime: let the user choose instead of
		  // overwriting their change silently.
		  if (!(await askOverwrite())) throw new Error(t("configChangedKept"));
		  return saveConfig(cfg, true);
		}
		if (result?.revision) {
		  if (state.persistedConfig) state.persistedConfig.revision = result.revision;
		  if (state.config) state.config.revision = result.revision;
		}
		for (const conflict of (result?.conflicts || []).slice(0, 3)) {
		  toast(t("configConflict"), t(`conflict_${conflict.kind}`).replace("{a}", conflict.first).replace("{b}", conflict.second), "warn");
		}
		return result;
	  }

	  function askOverwrite() {
		return new Promise((resolve) => {
		  openModal(`
			<div class="modal conflict-dialog" role="alertdialog" aria-modal="true" aria-labelledby="conflict-title">
			  <div class="modal-head"><div><h3 id="conflict-title">${esc(t("configChangedTitle"))}</h3><div class="hint">${esc(t("configChangedHint"))}</div></div></div>
			  <div class="modal-foot">
				<button data-conflict="reload">${esc(t("configChangedReload"))}</button>
				<button class="primary" data-conflict="overwrite">${esc(t("configChangedOverwrite"))}</button>
			  </div>
			</div>`);
		  document.querySelector('[data-conflict="overwrite"]')?.addEventListener("click", () => { closeModal(); resolve(true); });
		  document.querySelector('[data-conflict="reload"]')?.addEventListener("click", async () => {
			closeModal();
			state.dirtyViews.clear();
			await refreshCore(true).catch(() => {});
			renderApp();
			resolve(false);
		  });
		});
	  }

	  const VIEW_DATA = {
		dashboard: ["operations", "events", "timeline", "today", "thumbnails"],
		program: ["timeline", "thumbnails"],
		pages: ["thumbnails"],
		device: ["zones", "jobs"],
		"device-maintenance": ["jobs", "updateHistory"],
	  };
	  const VIEW_DATA_SOURCES = {
		operations: ["/api/browser/operations", (value) => ({ operations: value })],
		events: ["/api/events?limit=200", (value) => ({ events: value })],
		jobs: ["/api/jobs?limit=25", (value) => ({ jobs: value })],
		zones: ["/api/time/zones", (value) => ({ zones: value })],
		updateHistory: ["/api/update/history", (value) => ({ updateHistory: value })],
		timeline: [() => `/api/program/timeline?minutes=${Number(state.programRange || 60)}`, (value) => ({ timeline: value })],
		today: ["/api/program/timeline?day=1", (value) => ({ timelineToday: value })],
		thumbnails: ["/api/pages/thumbnails", (value) => ({ thumbnails: value })],
	  };
	  const viewDataLoadedAt = new Map();

	  // Loads data only some views show (journal, jobs, time zones, update
	  // history) when such a view opens, and again when it is older than a
	  // minute. The time zone list never changes while the Admin is open.
	  export async function ensureViewData(view = state.view) {
		const now = Date.now();
		const keys = (VIEW_DATA[view] || []).filter((key) => {
		  const loaded = viewDataLoadedAt.get(key);
		  return loaded === undefined || (key !== "zones" && now - loaded > (key === "timeline" ? 30000 : 60000));
		});
		if (!keys.length) return;
		keys.forEach((key) => viewDataLoadedAt.set(key, now));
		const results = await Promise.allSettled(keys.map((key) => getJSON(typeof VIEW_DATA_SOURCES[key][0] === "function" ? VIEW_DATA_SOURCES[key][0]() : VIEW_DATA_SOURCES[key][0])));
		const patch = {};
		results.forEach((result, index) => {
		  if (result.status === "fulfilled") Object.assign(patch, VIEW_DATA_SOURCES[keys[index]][1](result.value));
		  else viewDataLoadedAt.delete(keys[index]);
		});
		if (!Object.keys(patch).length) return;
		applyCoreState(patch);
		if (state.view === view && state.auth?.authenticated) renderApp({ background: true });
	  }

	  export function applyCoreState({ cfg, status, privilege, timeInfo, zones, jobs, updateHistory, operations, events, timeline, timelineToday, thumbnails }) {
		if (cfg) {
		  state.config = cfg;
		  state.persistedConfig = JSON.parse(JSON.stringify(cfg));
		}
		if (status) {
		  const previousHardware = state.hardware || {};
		  state.status = { ...(state.status || {}), ...status };
		  state.update = status.update || state.update;
		  state.hardware = Object.keys(status.hardware || {}).length ? status.hardware : previousHardware;
		}
		if (privilege) state.privilege = privilege;
		if (timeInfo) state.time = timeInfo;
		if (zones) state.timezones = zones.zones || [];
		if (jobs) state.jobs = jobs.jobs || [];
		if (updateHistory) state.updateHistory = updateHistory;
		if (operations) state.operations = operations;
		if (events) state.events = events.events || [];
		if (timeline) state.timeline = timeline;
		if (timelineToday) state.timelineToday = timelineToday;
		if (thumbnails) state.thumbnails = thumbnails;
		syncThemeFromConfig();
	  }

	  function renderFatal(error) {
		root.innerHTML = `<main class="fatal-shell"><section class="fatal-card" role="alert">
		  <div class="mark">K</div><div><span class="eyebrow">KioskMate Admin</span><h1>${esc(t("adminLoadFailed"))}</h1><p>${esc(error?.message || t("unknownError"))}</p></div>
		  <div class="actions"><button class="primary" data-action="fatal-retry">${esc(t("retry"))}</button><button data-action="fatal-login">${esc(t("backToLogin"))}</button></div>
		</section></main>`;
		document.querySelector('[data-action="fatal-retry"]')?.addEventListener("click", () => boot());
		document.querySelector('[data-action="fatal-login"]')?.addEventListener("click", async () => {
		  try { stopLiveUpdates();
          await postJSON("/api/auth/logout"); } catch (_) {}
		  state.auth = { authenticated: false, setupRequired: false };
		  renderLogin();
		});
	  }

	  window.addEventListener("unhandledrejection", (event) => {
		if (!root?.children?.length) renderFatal(event.reason || new Error(t("unknownError")));
	  });
	  window.addEventListener("error", (event) => {
		if (!root?.children?.length) renderFatal(event.error || new Error(event.message || t("unknownError")));
	  });

      async function refreshAndRender() {
        await runAction("refresh", async () => {
          await refreshCore();
          renderApp();
        }, t("refresh"));
      }

      let updateStatusTimer = null;
      let updateStatusPolling = false;

      function startUpdateStatusPolling() {
        if (updateStatusTimer) clearInterval(updateStatusTimer);
        pollStartupUpdateStatus().catch(() => {});
        updateStatusTimer = setInterval(() => refreshCachedUpdateStatus().catch(() => {}), 15 * 60 * 1000);
      }

      async function pollStartupUpdateStatus() {
        if (updateStatusPolling) return;
        updateStatusPolling = true;
        try {
          for (let attempt = 0; attempt < 12; attempt++) {
            await sleep(attempt === 0 ? 1500 : 2500);
            await refreshCachedUpdateStatus();
            if (state.update?.checked_at && !state.update?.checking) return;
          }
        } finally {
          updateStatusPolling = false;
        }
      }

      async function refreshCachedUpdateStatus() {
        const update = await getJSON("/api/update?cached=1");
        const changed = JSON.stringify(update) !== JSON.stringify(state.update);
        state.update = update;
		if (changed && state.auth?.authenticated) renderAppIfIdle();
      }

      function renderLogin() {
		stopLiveUpdates();
		if (authCountdownTimer) clearInterval(authCountdownTimer);
		authCountdownTimer = null;
        const setup = !!state.auth?.setupRequired;
        root.innerHTML = `
		  <main class="login-shell">
			<section class="login-frame">
			  <header class="login-header"><div class="mark">K</div><div><strong>KioskMate</strong><span>${esc(t("appSubtitle"))}</span></div></header>
			  <form class="login-card" id="auth-form">
			  <div class="login-title"><span class="eyebrow">${esc(t("adminAccess"))}</span><h1>${esc(setup ? t("setupTitle") : t("signInTitle"))}</h1><p>${esc(setup ? t("setupHint") : t("signInHint"))}</p></div>
			  <div class="login-fields">
				<input class="hidden" autocomplete="username" value="admin" />
				${setup ? field("setup-token", t("setupToken"), "text", "one-time-code") : ""}
				<div class="password-field">${field("auth-password", t("password"), "password", "current-password")}<button type="button" class="password-toggle" data-action="password-toggle" aria-label="${esc(t("showPassword"))}" title="${esc(t("showPassword"))}">◉</button></div>
				<div id="auth-error" class="login-error" role="alert" hidden></div>
				<button class="primary login-submit" data-busy="auth" data-login-label="${esc(setup ? t("createPassword") : t("signIn"))}">${esc(setup ? t("createPassword") : t("signIn"))}</button>
			  </div>
			  <footer class="login-footer">
				<div class="login-selects">
				  ${selectHtml("login-lang", t("language"), state.lang, [["en", "English"], ["de", "Deutsch"]])}
				  ${selectHtml("login-theme", t("theme"), state.theme, [["dark", t("dark")], ["light", t("light")]])}
				</div>
				<p>${esc(t("setupInfo"))}</p>
			  </footer>
			  </form>
			</section>
		  </main>`;
        document.getElementById("auth-form").addEventListener("submit", async (event) => {
          event.preventDefault();
		  let retryDelay = 0;
		  setBusy("auth", true);
		  const errorBox = document.getElementById("auth-error");
		  if (errorBox) { errorBox.hidden = true; errorBox.textContent = ""; }
		  try {
			const response = setup
			  ? await postJSON("/api/auth/setup", { token: val("setup-token"), password: val("auth-password") })
			  : await postJSON("/api/auth/login", { password: val("auth-password") });
			state.auth = { ...response, authenticated: response.authenticated !== false };
			if (!state.auth.authenticated) throw new Error(t("loginSessionFailed"));
			if (response.config) applyCoreState({ cfg: response.config });
			else await refreshCore(true);
			renderApp();
			toast(t("signedIn"), "", "ok");
			startUpdateStatusPolling();
          startLiveUpdates();
			refreshCore(false).then(() => {
			  if (state.auth?.authenticated) renderAppIfIdle();
			  if (state.auth?.authenticated && !isDirty()) maybeOpenSetupWizard();
			}).catch((error) => toast(t("backgroundLoadFailed"), error.message, "warn"));
		  } catch (error) {
			const currentError = document.getElementById("auth-error");
			retryDelay = Number(error.retryAfter || 0);
			const message = retryDelay > 0 ? t("loginRetryAfter").replace("{seconds}", String(retryDelay)) : (error.message || t("loginFailed"));
			if (currentError) { currentError.hidden = false; currentError.textContent = message; }
			const password = document.getElementById("auth-password");
			password?.focus();
		  } finally {
			setBusy("auth", false);
			if (retryDelay > 0) startAuthCountdown(retryDelay);
		  }
        });
		document.querySelector('[data-action="password-toggle"]')?.addEventListener("click", () => {
		  const input = document.getElementById("auth-password");
		  if (!input) return;
		  input.type = input.type === "password" ? "text" : "password";
		});
        document.getElementById("login-lang").addEventListener("change", (event) => setLanguage(event.target.value));
        document.getElementById("login-theme").addEventListener("change", (event) => setTheme(event.target.value));
		if (state.auth?.rateLimited && Number(state.auth?.retryAfterSeconds || 0) > 0) startAuthCountdown(Number(state.auth.retryAfterSeconds));
      }

	  function startAuthCountdown(seconds) {
		if (authCountdownTimer) clearInterval(authCountdownTimer);
		const until = Date.now() + Math.max(1, Number(seconds || 0)) * 1000;
		const update = () => {
		  const button = document.querySelector('[data-busy="auth"]');
		  const errorBox = document.getElementById("auth-error");
		  const remaining = Math.max(0, Math.ceil((until - Date.now()) / 1000));
		  if (remaining <= 0) {
			clearInterval(authCountdownTimer);
			authCountdownTimer = null;
			if (button) {
			  button.disabled = false;
			  button.textContent = button.dataset.loginLabel || t("signIn");
			}
			if (errorBox) errorBox.hidden = true;
			return;
		  }
		  if (button) {
			button.disabled = true;
			button.textContent = t("loginRetryButton").replace("{seconds}", String(remaining));
		  }
		  if (errorBox) {
			errorBox.hidden = false;
			errorBox.textContent = t("loginRetryAfter").replace("{seconds}", String(remaining));
		  }
		};
		update();
		authCountdownTimer = setInterval(update, 1000);
	  }

      // renderApp morphs the page to the current state. A background render
      // (live updates) keeps the value of the focused field and, while the
      // user is editing or a view has unsaved changes, leaves the content
      // area untouched and only refreshes the shell (status chips, nav).
      export function renderApp({ background = false } = {}) {
		if (authCountdownTimer) clearInterval(authCountdownTimer);
		authCountdownTimer = null;
        const browser = state.status?.browser || {};
        const timeInfo = state.time || {};
        const device = state.hardware?.device || {};
        const host = device.hostname || state.status?.hostname || "";
        const area = activeNavItem();
        if (!root.querySelector(":scope > .layout")) root.innerHTML = "";
        const running = !!browser.running;
        const html = `
          <div class="layout ${state.mobileNavOpen ? "nav-open" : ""}">
            <aside class="sidebar" id="sidebar" aria-label="${esc(t("navigationMenu"))}">
              <div class="brand">
                <div class="mark">K</div>
                <div class="brand-copy"><h1>KioskMate</h1><p>${esc(host || t("appSubtitle"))}</p></div>
                <button class="icon-command sidebar-close" data-action="nav-mobile-toggle" aria-label="${esc(t("close"))}" title="${esc(t("close"))}">${icon("close", 18)}</button>
              </div>
              <nav class="nav" aria-label="${esc(t("mainNavigation"))}">${renderNav()}</nav>
              <div class="sidebar-foot">
                <div class="sidebar-selects">
                  ${selectHtml("lang", t("language"), state.lang, [["en", "English"], ["de", "Deutsch"]])}
                  ${selectHtml("theme", t("theme"), state.theme, [["dark", t("dark")], ["light", t("light")]])}
                </div>
                <button data-action="logout">${esc(t("logout"))}</button>
              </div>
            </aside>
            <div class="nav-scrim" data-action="nav-mobile-toggle" aria-hidden="true"></div>
            <main class="main">
              <header class="topbar">
                <div class="title">
                  <h2>${esc(t(area.label))}</h2>
                  <p>${esc(t(area.hint || "overview"))}</p>
                </div>
                <div class="chips">
                  ${state.update?.update_available ? `<button class="chip update-chip" data-view="device-maintenance" title="${esc(t("openUpdateHint"))}">${esc(t("updateAvailable"))}: ${esc(state.update.latest_version || "")}</button>` : ""}
                  <button class="chip status-chip ${running ? "ok" : "bad"}" data-view="dashboard" title="${esc(t("openDashboardStatusHint"))}"><span class="dot" aria-hidden="true"></span>${esc(running ? t("running") : t("stopped"))}</button>
                  <button class="chip ${timeInfo.synchronized ? "" : "warn"}" data-view="device" title="${esc(timeInfo.synchronized ? t("timeSynchronized") : t("timeNotSynchronized"))}"><span id="kiosk-clock">${esc(formatClock(timeInfo.current_time))}</span></button>
                  <span class="chip version-chip">${esc(state.auth?.version || "dev")}</span>
                  <button class="icon-command" title="${esc(t("refresh"))}" aria-label="${esc(t("refresh"))}" data-busy="refresh" data-action="refresh">${icon("reload", 18)}</button>
                </div>
              </header>
              <section class="content">
                ${state.status?.config?.load_warning ? stateBanner("warn", t("configurationRecovered"), state.status.config.load_warning) : ""}
                ${area.children ? renderAreaTabs(area) : ""}
                ${renderView()}
              </section>
            </main>
            <nav class="bottom-nav" aria-label="${esc(t("mainNavigation"))}">
              ${NAV.slice(0, 3).map((item) => `<button class="${isNavActive(item) ? "active" : ""}" data-view="${esc(item.id)}" ${isNavActive(item) ? 'aria-current="page"' : ""}>${icon(item.icon)}<span>${esc(t(item.label))}</span></button>`).join("")}
              <button class="${NAV.slice(3).some(isNavActive) ? "active" : ""}" data-action="nav-mobile-toggle" aria-controls="sidebar" aria-expanded="${state.mobileNavOpen}">${icon("more")}<span>${esc(t("navMore"))}</span></button>
            </nav>
          </div>`;
        morph(root, html, {
          preserveFocused: background,
          preserveEdited: background,
          preserveExternal: background,
        });
        applyGeometry(root);
        bindScope(() => {
          bindShell();
          bindView();
          bindDirtyTracking();
        });
        startHeaderClock();
        if (!background) ensureViewData(state.view);
      }

      // The Content-Security-Policy forbids inline style attributes, so bars
      // and timelines carry their geometry as data-left/width/height
      // (percent) and get it here through the CSSOM, which the policy allows.
      export function applyGeometry(scope = document) {
        scope.querySelectorAll("[data-left], [data-width], [data-height]").forEach((element) => {
          for (const property of ["left", "width", "height"]) {
            const value = element.dataset[property];
            if (value !== undefined) element.style[property] = `${value}%`;
          }
        });
      }

      function renderAreaTabs(area) {
        return `<div class="area-tabs" role="tablist" aria-label="${esc(t(area.label))}">${area.children.map((child) => `<button role="tab" aria-selected="${state.view === child.id}" class="${state.view === child.id ? "active" : ""}" data-view="${esc(child.id)}">${esc(t(child.label))}</button>`).join("")}</div>`;
      }

	  // Background refreshes (loaded data, live updates) never discard input.
	  export function renderAppIfIdle() {
		renderApp({ background: true });
		return true;
	  }

      let liveSource = null;
      let liveRenderTimer = null;

      // Live updates over Server-Sent Events while the Admin tab is visible.
      // The server samples nothing for the stream when no tab is connected.
      export function startLiveUpdates() {
        stopLiveUpdates();
        if (typeof EventSource === "undefined" || document.hidden || !state.auth?.authenticated) return;
        liveSource = new EventSource("/api/stream");
        liveSource.addEventListener("status", (event) => {
          try {
            applyCoreState({ status: JSON.parse(event.data) });
          } catch {
            return;
          }
          // Keeps "up next" and the timeline current; a draft timeline of
          // unsaved programme changes is not replaced.
          if (["dashboard", "program"].includes(state.view) && !isDirty()) ensureViewData(state.view);
          scheduleLiveRender();
        });
        liveSource.addEventListener("events", (event) => {
          try {
            const fresh = JSON.parse(event.data) || [];
            const known = new Set((state.events || []).map((item) => item.id));
            state.events = [...(state.events || []), ...fresh.filter((item) => !known.has(item.id))].slice(-200);
          } catch {
            return;
          }
          scheduleLiveRender();
        });
        liveSource.addEventListener("auth", () => {
          stopLiveUpdates();
          state.auth = null;
          boot();
        });
      }

      export function stopLiveUpdates() {
        if (liveSource) liveSource.close();
        liveSource = null;
      }

      function scheduleLiveRender() {
        if (liveRenderTimer || !state.auth?.authenticated) return;
        liveRenderTimer = setTimeout(() => {
          liveRenderTimer = null;
          if (state.auth?.authenticated) renderApp({ background: true });
        }, 100);
      }

      document.addEventListener("visibilitychange", () => {
        if (document.hidden) stopLiveUpdates();
        else if (state.auth?.authenticated) startLiveUpdates();
      });

      function renderNav() {
        return NAV.map((item) => {
          const active = isNavActive(item);
          return `<button class="nav-item ${active ? "active" : ""}" data-view="${esc(item.id)}" ${active ? 'aria-current="page"' : ""}>${icon(item.icon)}<span>${esc(t(item.label))}</span></button>`;
        }).join("");
      }

      function isNavActive(item) {
        return state.view === item.id || (item.children || []).some((child) => child.id === state.view);
      }

      function activeNavItem() {
        return NAV.find(isNavActive) || NAV[0];
      }

      function renderView() {
        switch (state.view) {
          case "program":
            return renderProgram();
          case "pages":
            return renderPages();
          case "automations":
            return renderAutomations();
          case "automations-mqtt":
            return renderMQTT();
          case "device":
            return renderDeviceHealth();
          case "device-display":
            return renderSettingsBrowser();
          case "device-maintenance":
            return `<div class="page-stack">${renderSystemActions()}${renderSettingsMaintenance()}</div>`;
          case "device-logs":
            return renderLogs();
          case "device-settings":
            return `<div class="page-stack">${renderSettingsAdmin()}${renderSettingsConfig()}</div>`;
          default:
            return renderDashboard();
        }
      }

      export function navigate(nextView) {
        nextView = resolveView(nextView);
        if (nextView === state.view || !confirmDiscard()) return;
        state.view = nextView;
        state.mobileNavOpen = false;
        localStorage.setItem("kioskmate.view", state.view);
        renderApp();
        window.scrollTo({ top: 0 });
      }

      function bindShell() {
        document.querySelectorAll('[data-action="nav-mobile-toggle"]').forEach((button) => button.addEventListener("click", () => {
          state.mobileNavOpen = !state.mobileNavOpen;
          renderApp();
        }));
        document.querySelectorAll("[data-view]").forEach((button) => {
          button.addEventListener("click", () => navigate(button.dataset.view));
        });
        document.getElementById("lang")?.addEventListener("change", (event) => setLanguage(event.target.value));
        document.getElementById("theme")?.addEventListener("change", (event) => setTheme(event.target.value));
        document.querySelector('[data-action="logout"]')?.addEventListener("click", async () => {
          if (!confirmDiscard()) return;
          stopLiveUpdates();
          await postJSON("/api/auth/logout");
          state.auth = null;
          await boot();
        });
        document.querySelector('[data-action="refresh"]')?.addEventListener("click", () => {
          if (confirmDiscard()) refreshAndRender();
        });
      }

      async function setLanguage(lang) {
        if (!confirmDiscard()) {
          const select = document.getElementById("lang");
          if (select) select.value = state.lang;
          return;
        }
        try {
          state.lang = await loadLanguage(lang === "de" ? "de" : "en");
        } catch (error) {
          toast(t("actionFailed"), error.message, "error");
          return;
        }
        localStorage.setItem("kioskmate.lang", state.lang);
        document.documentElement.lang = state.lang;
        state.auth?.authenticated ? renderApp() : renderLogin();
      }

      function setTheme(theme) {
        if (!confirmDiscard()) {
          const select = document.getElementById("theme");
          if (select) select.value = state.theme;
          return;
        }
        state.theme = theme === "light" ? "light" : "dark";
        localStorage.setItem("kioskmate.theme", state.theme);
        localStorage.setItem("kioskmate.theme.explicit", "1");
        state.themeExplicit = true;
        document.documentElement.dataset.theme = state.theme;
        state.auth?.authenticated ? renderApp() : renderLogin();
      }

      function syncThemeFromConfig() {
        if (state.themeExplicit) return;
        const configured = state.config?.kiosk?.theme === "light" ? "light" : "dark";
        state.theme = configured;
        localStorage.setItem("kioskmate.theme", configured);
        document.documentElement.dataset.theme = configured;
      }
