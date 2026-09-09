"use strict";

      const I18N = window.KIOSKMATE_I18N || {};

      const NAV = [
        { id: "dashboard", label: "dashboard", hint: "overview" },
        {
          id: "kiosk",
          label: "kiosk",
          hint: "kioskStreaming",
          children: [
            { id: "kiosk-pages", label: "pagesAndWorkflow", hint: "kioskStreaming" },
            { id: "kiosk-display", label: "displayRendering", hint: "performance" },
          ],
        },
        { id: "mqtt", label: "mqtt", hint: "mqttSettings" },
        {
          id: "system",
          label: "system",
          hint: "systemTools",
          children: [
            { id: "system-device", label: "deviceAndTime", hint: "hardwareStatus" },
            { id: "system-maintenance", label: "systemMaintenance", hint: "privilegedActions" },
            { id: "system-logs", label: "logs", hint: "refreshLogs" },
          ],
        },
        {
          id: "settings",
          label: "settings",
          hint: "adminSettings",
          children: [
            { id: "settings-admin", label: "adminAccess", hint: "adminSettings" },
            { id: "settings-config", label: "configData", hint: "configFile" },
            { id: "settings-updates", label: "updatesAndRepair", hint: "update" },
          ],
        },
      ];

      const VIEW_ALIASES = {
        kiosk: "kiosk-pages",
        scheduler: "kiosk-pages",
        "kiosk-schedule": "kiosk-pages",
        system: "system-device",
        "system-actions": "system-maintenance",
        "system-hardware": "system-device",
        settings: "settings-admin",
        "settings-browser": "kiosk-display",
        "settings-maintenance": "settings-updates",
      };

      const root = document.getElementById("root");
      const toasts = document.getElementById("toasts");
      const modalRoot = document.getElementById("modal-root");
      const storedTheme = localStorage.getItem("kioskmate.theme");
      function storedList(key, fallback = []) {
        try {
          const value = JSON.parse(localStorage.getItem(key) || "null");
          return Array.isArray(value) ? value : fallback;
        } catch {
          return fallback;
        }
      }

      const state = {
        lang: localStorage.getItem("kioskmate.lang") || (((navigator.language || "en").toLowerCase().startsWith("de")) ? "de" : "en"),
        theme: storedTheme === "light" ? "light" : "dark",
        themeExplicit: localStorage.getItem("kioskmate.theme.explicit") === "1",
        view: VIEW_ALIASES[localStorage.getItem("kioskmate.view")] || localStorage.getItem("kioskmate.view") || "dashboard",
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
        pageFilter: "",
        kioskEditorMode: localStorage.getItem("kioskmate.kioskEditorMode") === "flow" ? "flow" : "storybook",
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
        navExpanded: new Set(storedList("kioskmate.navExpanded", [])),
        mobileNavOpen: false,
      };

      const DIRTY_PREFIXES = {
        "kiosk-pages": ["page-name-", "page-url-", "page-disabled-", "scheduler-", "rotation-", "rule-"],
        "kiosk-display": ["kiosk-", "perf-", "watchdog-"],
        mqtt: ["mqtt-"],
        "system-device": ["time-"],
        "settings-admin": ["admin-"],
        "settings-config": ["config-raw"],
      };

      const SAVE_ACTIONS = {
        "kiosk-pages": ["kiosk-save", "kiosk-save-restart"],
        "kiosk-display": ["browser-settings-save", "browser-settings-save-restart"],
        mqtt: ["mqtt-save"],
        "settings-admin": ["admin-save"],
        "settings-config": ["config-raw-save"],
      };

      document.documentElement.dataset.theme = state.theme;

      function t(key) {
        return (I18N[state.lang] && I18N[state.lang][key]) || I18N.en[key] || key;
      }

      function esc(value) {
        return String(value ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
      }

	  const field = window.KioskMateUI.field;
	  const selectHtml = window.KioskMateUI.selectHtml;
	  const switchHtml = window.KioskMateUI.switchHtml;

      function val(id) {
        return document.getElementById(id)?.value ?? "";
      }

      function checked(id) {
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

      function toast(title, message = "", type = "ok") {
        const item = document.createElement("div");
        item.className = "toast " + (type === "error" ? "error" : type === "warn" ? "warn" : "");
        item.setAttribute("role", type === "error" ? "alert" : "status");
        item.innerHTML = `<div><strong>${esc(title)}</strong>${message ? `<div class="muted">${esc(message)}</div>` : ""}</div><button class="toast-close" title="${esc(t("close"))}" aria-label="${esc(t("close"))}">&times;</button>`;
        item.querySelector(".toast-close")?.addEventListener("click", () => item.remove());
        toasts.appendChild(item);
        setTimeout(() => item.remove(), 5200);
      }

      let modalKeyHandler = null;

      function openModal(html) {
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

      function closeModal() {
        if (modalKeyHandler) document.removeEventListener("keydown", modalKeyHandler);
        modalKeyHandler = null;
        modalRoot.innerHTML = "";
      }

      function openMQTTTestDialog() {
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

      function appendMQTTEvent(event) {
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

      async function streamJSONLines(path, body, onEvent, signal) {
		return window.KioskMateAPI.streamJSONLines(path, body, { onEvent, signal, csrf: state.auth?.csrf });
      }

      function formatValue(value, suffix = "") {
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

      function formatDate(value) {
        if (!value) return t("never");
        const date = new Date(value);
        return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString();
      }

      function formatMQTTState(value) {
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

      function formatClock(value) {
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

      function secondsToDuration(value, fallback = 0) {
        const n = Number(value);
        if (!Number.isFinite(n) || n <= 0) return fallback;
        return Math.round(n / 1_000_000_000);
      }

      function formatDuration(seconds) {
        const total = Number(seconds || 0);
        if (total >= 3600 && total % 3600 === 0) return `${total / 3600} h`;
        if (total >= 60 && total % 60 === 0) return `${total / 60} min`;
        return `${total} s`;
      }

      function durationToNs(seconds) {
        return Math.max(0, Number(seconds || 0)) * 1_000_000_000;
      }

      function sleep(ms) {
        return new Promise((resolve) => setTimeout(resolve, ms));
      }

      function cloneConfig() {
        return JSON.parse(JSON.stringify(state.config || {}));
      }

      async function request(path, options = {}) {
		try {
		  return await window.KioskMateAPI.request(path, { ...options, csrf: state.auth?.csrf });
		} catch (error) {
		  if (error?.name === "AbortError") throw new Error(t("requestTimeout"));
		  throw error;
		}
      }

      const getJSON = (path, options = {}) => request(path, options);
      const postJSON = (path, body = {}) => request(path, { method: "POST", body: JSON.stringify(body) });
      const deleteJSON = (path) => request(path, { method: "DELETE" });

      async function runAction(name, fn, success = t("actionDone")) {
        setBusy(name, true);
        recordAction(t("actionStarted"), name, "warn");
        try {
          const result = await fn();
          toast(success, "", "ok");
          recordAction(success, name, "ok");
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

      function markDirty(view = state.view) {
        state.dirtyViews.add(view);
        updateDirtyUI();
      }

      function clearDirty(view = state.view) {
        state.dirtyViews.delete(view);
        updateDirtyUI();
      }

      function isDirty(view = state.view) {
        return state.dirtyViews.has(view);
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
        state.dirtyViews.delete(view);
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

      function recordAction(title, detail = "", type = "ok") {
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

      async function boot() {
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
		  refreshCore(false).then(() => {
			if (state.auth?.authenticated) renderAppIfIdle();
		  }).catch((error) => toast(t("backgroundLoadFailed"), error.message, "warn"));
        } catch (err) {
		  renderFatal(err);
        }
      }

	  async function refreshCore(fast = false) {
		if (fast) {
		  const cfg = await getJSON("/api/config");
		  applyCoreState({ cfg });
		  return;
		}
		try {
		  const quick = await getJSON("/api/status?fast=1", { timeout: 8000 });
		  applyCoreState({ status: quick });
		} catch (_) {}
		const requests = await Promise.allSettled([
		  getJSON("/api/config"), getJSON("/api/status", { timeout: 20000 }), getJSON("/api/privilege"),
		  getJSON("/api/time"), getJSON("/api/time/zones"), getJSON("/api/jobs?limit=25"),
		  getJSON("/api/update/history"),
		  getJSON("/api/browser/operations"),
		  getJSON("/api/events?limit=200"),
		]);
		const value = (index) => requests[index].status === "fulfilled" ? requests[index].value : undefined;
		applyCoreState({ cfg: value(0), status: value(1), privilege: value(2), timeInfo: value(3), zones: value(4), jobs: value(5), updateHistory: value(6), operations: value(7), events: value(8) });
		const failed = requests.filter((item) => item.status === "rejected");
		if (failed.length === requests.length) throw failed[0].reason;
      }

	  function applyCoreState({ cfg, status, privilege, timeInfo, zones, jobs, updateHistory, operations, events }) {
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
		syncThemeFromConfig();
	  }

	  function renderFatal(error) {
		root.innerHTML = `<main class="fatal-shell"><section class="fatal-card" role="alert">
		  <div class="mark">K</div><div><span class="eyebrow">KioskMate Admin</span><h1>${esc(t("adminLoadFailed"))}</h1><p>${esc(error?.message || t("unknownError"))}</p></div>
		  <div class="actions"><button class="primary" data-action="fatal-retry">${esc(t("retry"))}</button><button data-action="fatal-login">${esc(t("backToLogin"))}</button></div>
		</section></main>`;
		document.querySelector('[data-action="fatal-retry"]')?.addEventListener("click", () => boot());
		document.querySelector('[data-action="fatal-login"]')?.addEventListener("click", async () => {
		  try { await postJSON("/api/auth/logout"); } catch (_) {}
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
				<button class="primary login-submit" data-busy="auth">${esc(setup ? t("createPassword") : t("signIn"))}</button>
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
			refreshCore(false).then(() => { if (state.auth?.authenticated) renderAppIfIdle(); }).catch((error) => toast(t("backgroundLoadFailed"), error.message, "warn"));
		  } catch (error) {
			const currentError = document.getElementById("auth-error");
			const message = error.retryAfter > 0 ? t("loginRetryAfter").replace("{seconds}", String(error.retryAfter)) : (error.message || t("loginFailed"));
			if (currentError) { currentError.hidden = false; currentError.textContent = message; }
			const password = document.getElementById("auth-password");
			password?.focus();
		  } finally {
			setBusy("auth", false);
		  }
        });
		document.querySelector('[data-action="password-toggle"]')?.addEventListener("click", () => {
		  const input = document.getElementById("auth-password");
		  if (!input) return;
		  input.type = input.type === "password" ? "text" : "password";
		});
        document.getElementById("login-lang").addEventListener("change", (event) => setLanguage(event.target.value));
        document.getElementById("login-theme").addEventListener("change", (event) => setTheme(event.target.value));
      }

      function renderApp() {
        const browser = state.status?.browser || {};
        const mqtt = state.status?.mqtt || {};
        const timeInfo = state.time || {};
        root.innerHTML = `
          <div class="layout">
            <aside class="sidebar ${state.mobileNavOpen ? "mobile-open" : ""}">
              <div class="brand">
                <div class="brand-identity">
                  <div class="mark">K</div>
                  <div>
                    <h1>KioskMate</h1>
                    <p>${esc(t("appSubtitle"))}</p>
                  </div>
                </div>
                <button class="mobile-menu-toggle icon-command" data-action="nav-mobile-toggle" aria-expanded="${state.mobileNavOpen}" title="${esc(t("navigationMenu"))}" aria-label="${esc(t("navigationMenu"))}">☰</button>
              </div>
              <nav class="nav">
                ${renderNav()}
              </nav>
              <div class="sidebar-foot">
                <div class="sidebar-selects">
                  ${selectHtml("lang", t("language"), state.lang, [["en", "English"], ["de", "Deutsch"]])}
                  ${selectHtml("theme", t("theme"), state.theme, [["dark", t("dark")], ["light", t("light")]])}
                </div>
                <button data-action="logout">${esc(t("logout"))}</button>
              </div>
            </aside>
            <main class="main">
              <header class="topbar">
                <div class="title">
                  <h2>${esc(t(activeNavItem().label || "dashboard"))}</h2>
                  <p>${esc(t(activeNavItem().hint || "overview"))}</p>
                </div>
                <div class="chips">
                  ${state.update?.update_available ? `<button class="chip update-chip" data-view="settings-updates" title="${esc(t("openUpdateHint"))}">${esc(t("updateAvailable"))}: ${esc(state.update.latest_version || "")}</button>` : ""}
                  <button class="chip ${browser.running ? "ok" : "bad"}" data-view="dashboard" title="${esc(t("openDashboardStatusHint"))}">${esc(browser.running ? t("running") : t("stopped"))}</button>
                  <button class="chip ${mqtt.connected ? "ok" : mqtt.state === "auth_error" || mqtt.state === "error" ? "bad" : ""}" data-view="mqtt" title="${esc(mqtt.last_error || t("openMQTTStatusHint"))}">MQTT: ${esc(formatMQTTState(mqtt.state))}</button>
                  <button class="chip ${timeInfo.synchronized ? "ok" : "warn"}" data-view="system-device" title="${esc(timeInfo.synchronized ? t("timeSynchronized") : t("timeNotSynchronized"))}"><span id="kiosk-clock">${esc(formatClock(timeInfo.current_time))}</span></button>
                  <span class="chip">${esc(state.auth?.version || "dev")}</span>
                  <button class="icon-command" title="${esc(t("refresh"))}" aria-label="${esc(t("refresh"))}" data-busy="refresh" data-action="refresh">↻</button>
                </div>
              </header>
              <section class="content">${renderView()}</section>
            </main>
          </div>`;
        bindShell();
        bindView();
        bindDirtyTracking();
        startHeaderClock();
      }

	  function renderAppIfIdle() {
		const active = document.activeElement;
		const editing = active && ["INPUT", "SELECT", "TEXTAREA"].includes(active.tagName);
		if (state.dirtyViews.size || editing) return false;
		renderApp();
		return true;
	  }

      function renderNav() {
        return NAV.map((item) => {
          const children = item.children || [];
          const active = isNavActive(item);
          const expanded = children.length && (state.navExpanded.has(item.id) || active);
          const childHtml = children.length
            ? `<div class="nav-children" ${expanded ? "" : "hidden"}>${children.map((child) => `<button class="${state.view === child.id ? "active" : ""}" data-view="${child.id}"><span>${esc(t(child.label))}</span><small>${esc(t(child.hint))}</small></button>`).join("")}</div>`
            : "";
          return `
            <div class="nav-group">
              <button class="nav-parent ${active ? "active" : ""}" ${children.length ? `data-nav-toggle="${esc(item.id)}" aria-expanded="${expanded}"` : `data-view="${esc(item.id)}"`}>
                <span>${esc(t(item.label))}</span>
                <span class="nav-meta"><small>${esc(t(item.hint))}</small>${children.length ? `<span class="nav-chevron" aria-hidden="true">${expanded ? "−" : "+"}</span>` : ""}</span>
              </button>
              ${childHtml}
            </div>`;
        }).join("");
      }

      function isNavActive(item) {
        return state.view === item.id || (item.children || []).some((child) => child.id === state.view);
      }

      function activeNavItem() {
        for (const item of NAV) {
          if (state.view === item.id) return item;
          const child = (item.children || []).find((entry) => entry.id === state.view);
          if (child) return child;
        }
        return NAV[0];
      }

      function renderView() {
        switch (state.view) {
          case "kiosk":
          case "kiosk-pages":
            return renderKiosk();
          case "kiosk-schedule":
            return renderKiosk();
          case "kiosk-display":
            return renderSettingsBrowser();
          case "mqtt":
            return renderMQTT();
          case "system":
          case "system-maintenance":
            return renderSystemActions();
          case "system-device":
            return renderHardware();
          case "system-terminal":
            return renderTerminal();
          case "system-logs":
            return renderLogs();
          case "settings":
          case "settings-browser":
            return renderSettingsAdmin();
          case "settings-admin":
            return renderSettingsAdmin();
          case "settings-config":
            return renderSettingsConfig();
          case "settings-updates":
            return renderSettingsMaintenance();
          default:
            return renderDashboard();
        }
      }

      function bindShell() {
        document.querySelector('[data-action="nav-mobile-toggle"]')?.addEventListener("click", () => {
          state.mobileNavOpen = !state.mobileNavOpen;
          renderApp();
        });
        document.querySelectorAll("[data-nav-toggle]").forEach((button) => {
          button.addEventListener("click", () => {
            const id = button.dataset.navToggle;
            if (state.navExpanded.has(id)) state.navExpanded.delete(id);
            else state.navExpanded.add(id);
            localStorage.setItem("kioskmate.navExpanded", JSON.stringify([...state.navExpanded]));
            renderApp();
          });
        });
        document.querySelectorAll("[data-view]").forEach((button) => {
          button.addEventListener("click", () => {
            const nextView = button.dataset.view;
            if (nextView === state.view || !confirmDiscard()) return;
            state.view = nextView;
            state.mobileNavOpen = false;
            const parent = NAV.find((item) => (item.children || []).some((child) => child.id === nextView));
            if (parent) state.navExpanded.add(parent.id);
            localStorage.setItem("kioskmate.view", state.view);
            localStorage.setItem("kioskmate.navExpanded", JSON.stringify([...state.navExpanded]));
            renderApp();
          });
        });
        document.getElementById("lang")?.addEventListener("change", (event) => setLanguage(event.target.value));
        document.getElementById("theme")?.addEventListener("change", (event) => setTheme(event.target.value));
        document.querySelector('[data-action="logout"]')?.addEventListener("click", async () => {
          if (!confirmDiscard()) return;
          await postJSON("/api/auth/logout");
          state.auth = null;
          await boot();
        });
        document.querySelector('[data-action="refresh"]')?.addEventListener("click", () => {
          if (confirmDiscard()) refreshAndRender();
        });
      }

      function setLanguage(lang) {
        if (!confirmDiscard()) {
          const select = document.getElementById("lang");
          if (select) select.value = state.lang;
          return;
        }
        state.lang = lang === "de" ? "de" : "en";
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
