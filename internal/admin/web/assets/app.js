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
        const response = await fetch(path, {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
          signal,
        });
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || response.statusText || "HTTP " + response.status);
        }
        if (!response.body) throw new Error("Streaming response is not available");
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() || "";
          for (const line of lines) {
            if (!line.trim()) continue;
            onEvent(JSON.parse(line));
          }
        }
        buffer += decoder.decode();
        if (buffer.trim()) onEvent(JSON.parse(buffer));
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
		const { timeout: timeoutMs, signal, headers, ...fetchOptions } = options;
		const controller = signal ? null : new AbortController();
		const timeout = controller ? setTimeout(() => controller.abort(), Number(timeoutMs || 12000)) : null;
		try {
		  const response = await fetch(path, {
			credentials: "same-origin",
			headers: { "Content-Type": "application/json", ...(headers || {}) },
			...fetchOptions,
			signal: signal || controller?.signal,
		  });
		  const text = await response.text();
		  let data = {};
		  if (text) {
			try { data = JSON.parse(text); }
			catch { data = { error: text }; }
		  }
		  if (!response.ok) {
			const error = new Error(data.error || response.statusText || "HTTP " + response.status);
			error.data = data;
			throw error;
		  }
		  return data;
		} catch (error) {
		  if (error?.name === "AbortError") throw new Error(t("requestTimeout"));
		  throw error;
		} finally {
		  if (timeout) clearTimeout(timeout);
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
			if (state.auth?.authenticated) renderApp();
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
		]);
		const value = (index) => requests[index].status === "fulfilled" ? requests[index].value : undefined;
		applyCoreState({ cfg: value(0), status: value(1), privilege: value(2), timeInfo: value(3), zones: value(4), jobs: value(5), updateHistory: value(6) });
		const failed = requests.filter((item) => item.status === "rejected");
		if (failed.length === requests.length) throw failed[0].reason;
      }

	  function applyCoreState({ cfg, status, privilege, timeInfo, zones, jobs, updateHistory }) {
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
        if (changed && state.auth?.authenticated) renderApp();
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
			refreshCore(false).then(() => { if (state.auth?.authenticated) renderApp(); }).catch((error) => toast(t("backgroundLoadFailed"), error.message, "warn"));
		  } catch (error) {
			const currentError = document.getElementById("auth-error");
			if (currentError) { currentError.hidden = false; currentError.textContent = error.message || t("loginFailed"); }
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

      function renderDashboard() {
        const browser = state.status?.browser || {};
        const browserKnown = !!state.status?.browser;
        const cfg = state.config || {};
        const mqtt = state.status?.mqtt || {};
        const stats = browser.stats || {};
        const watchdog = browser.watchdog || {};
		const recovery = browser.recovery || {};
		const telemetry = browser.telemetry || {};
		const override = browser.override || {};
        const pages = normalizePages(cfg.kiosk?.pages, cfg.kiosk?.urls);
        const activeIndex = Number(browser.active || 0);
        const activePage = pages[activeIndex] || {};
        const watchdogReason = watchdog.last_reason || (browser.last_error === "signal: killed" ? t("watchdogKilledHint") : "");
        const enabledPages = pages.filter((page) => !page.disabled && page.url);
        const browserMessage = !browserKnown
          ? t("loading")
          : (browser.running
          ? `${t("displayRunningHint")} ${browser.page_name || activePage.name || ""}`.trim()
          : (browser.last_error ? `${t("displayStoppedErrorHint")}: ${browser.last_error}` : t("displayStoppedHint")));
        const runningLabel = !browserKnown ? t("loading") : (browser.running ? t("running") : t("stopped"));
        const runningTone = !browserKnown ? "" : (browser.running ? "ok" : "bad");
        return `
          <div class="page-stack">
            ${renderUpdateNotice()}
			${recovery.state && !["healthy", "idle"].includes(recovery.state) ? stateBanner(recovery.state === "failed" || recovery.state === "auth_blocked" ? "bad" : "warn", t("recoveryNeedsAttention"), recovery.last_result || recovery.reason || recovery.state, button("recoverNow", "browser-auto-recover", "primary")) : ""}
            ${stateBanner(browserKnown ? (browser.running ? "ok" : "bad") : "warn", browserKnown ? (browser.running ? t("displayReady") : t("displayNeedsAttention")) : t("loading"), browserMessage, browser.running
              ? `<button data-view="kiosk-pages">${esc(t("managePages"))}</button>`
              : (browserKnown ? button("startBrowser", "browser-start", "primary") : ""))}
            <section class="status-strip" aria-label="${esc(t("status"))}">
              ${statusTile(t("displayStatus"), runningLabel, runningTone, browser.pid ? `PID ${browser.pid}` : (browserKnown ? t("noProcess") : t("loading")))}
              ${statusTile(t("currentPage"), browser.page_name || activePage.name || "-", "", `${activeIndex + 1} / ${Math.max(1, pages.length)}`)}
			  ${statusTile(t("processorLoad"), formatValue(stats.cpu_percent, "%"), Number(stats.cpu_percent || 0) > 250 ? "warn" : "", `${formatValue(stats.rss_mb, " MB RAM")} · ${(stats.pids || []).length} ${t("processes")}`)}
              ${statusTile("MQTT", formatMQTTState(mqtt.state), mqtt.connected ? "ok" : mqtt.state === "auth_error" || mqtt.state === "error" ? "bad" : "", mqtt.last_error || (cfg.mqtt?.version ? `MQTT ${cfg.mqtt.version}` : "-"))}
            </section>
            <div class="dashboard-layout">
              <div class="control-stack">
                <div class="card">
                  <div class="head">
                    <div><h3>${esc(t("quickControl"))}</h3><span class="section-kicker">${esc(browser.page_name || activePage.name || t("noData"))}</span></div>
                    <span class="chip ${runningTone}">${esc(runningLabel)}</span>
                  </div>
                  <div class="body command-panel">
                    <div class="command-primary">
                      ${browser.running ? button("stopBrowser", "browser-stop") : button("startBrowser", "browser-start", "primary")}
                      ${browser.running ? button("reloadBrowser", "browser-reload", "primary") : ""}
                      <button data-view="kiosk-pages">${esc(t("manageFlow"))}</button>
                    </div>
                    <div class="active-page-summary">
                      <div class="active-page-copy"><strong>${esc(activePage.name || browser.page_name || t("noData"))}</strong><span>${esc(activePage.url || browser.url || "-")}</span><small>${esc(browser.scheduler?.next_switch ? `${t("nextSwitch")}: ${formatDate(browser.scheduler.next_switch)}` : t("manualPageControl"))}</small></div>
                      ${enabledPages.length > 1 ? `<div class="actions">${button("previousPage", "browser-previous")}${button("nextPage", "browser-next")}</div>` : ""}
                    </div>
                    <div id="page-check-output" class="action-feedback" aria-live="polite"></div>
                    <details class="disclosure">
                      <summary>${esc(t("troubleshooting"))}</summary>
                      <div class="disclosure-body action-matrix">
                        ${button("restartBrowser", "browser-restart")}
						${button("recoverNow", "browser-auto-recover")}
                        ${button("checkPage", "dashboard-page-check")}
                        ${button("renderCheck", "dashboard-render-check")}
                        ${button("repairHA", "browser-repair-ha")}
                        ${button("recoveryWorkflow", "browser-recover")}
                        ${button("browserDoctor", "browser-doctor")}
                        ${button("resetSession", "browser-reset-session", "danger-ghost")}
                        ${button("openPreview", "dashboard-preview-open")}
                        ${button("diagnostics", "dashboard-diagnostics")}
                      </div>
                    </details>
                  </div>
                </div>
                <div class="card">
                  <div class="head"><h3>${esc(t("healthAndProtection"))}</h3><button data-view="system-logs">${esc(t("openLogs"))}</button></div>
                  <div class="body health-list">
                    ${watchdogReason ? `<div class="notice warn">${esc(watchdogReason)}</div>` : ""}
                    ${healthRow(t("browserControl"), browser.devtools ? t("connected") : t("notConnected"), browser.devtools ? "ok" : "warn")}
                    ${healthRow(t("haThemeSync"), formatThemeStatus(browser.theme_status), browser.theme_status?.state === "applied" ? "ok" : browser.theme_status?.state === "failed" ? "bad" : "")}
                    ${healthRow(t("authGuard"), browser.auth_guard?.tripped ? `${t("blocked")}: ${browser.auth_guard.reason || "-"}` : t("ready"), browser.auth_guard?.tripped ? "bad" : "ok")}
					${browser.auth_guard?.tripped && browser.auth_guard?.kiosk_ip ? healthRow(t("kioskIPAddress"), browser.auth_guard.kiosk_ip, "warn") : ""}
					${healthRow(t("recoveryState"), recovery.state || t("idle"), recovery.state === "failed" || recovery.state === "auth_blocked" ? "bad" : recovery.state === "backoff" ? "warn" : "ok")}
					${recovery.backoff_until ? healthRow(t("backoffUntil"), formatDate(recovery.backoff_until), "warn") : ""}
                    ${healthRow(t("watchdog"), watchdog.pressure || t("normal"), watchdog.pressure && watchdog.pressure !== "normal" ? "warn" : "ok")}
					${healthRow(t("runtimeAverage"), `${formatValue(telemetry.cpu_average, "% CPU")} · ${formatValue(telemetry.rss_average_mb, " MB RAM")}`, "")}
					${override.active ? healthRow(t("temporaryOverride"), `${pages[override.page]?.name || override.page + 1} · ${formatDate(override.until)}`, "warn") : ""}
                    ${browser.last_error ? healthRow(t("lastError"), browser.last_error, "bad") : ""}
                    ${renderActionLog()}
                  </div>
                </div>
              </div>
              <div class="card live-preview">
                <div class="head">
                  <h3>${esc(t("liveView"))}</h3>
                  <div class="actions">
                    <span class="chip">${esc(browser.page_name || activePage.name || "-")}</span>
                    ${button("refreshSnapshot", "dashboard-snapshot-refresh")}
                    ${button("openPreview", "dashboard-preview-open")}
                  </div>
                </div>
                <div class="preview-shell">
                  ${state.snapshotURL ? `<img id="snapshot-image" class="snapshot-image" src="${esc(state.snapshotURL)}" alt="${esc(t("liveView"))}" />` : `<div id="snapshot-empty" class="empty">${esc(browser.running ? t("snapshotOnDemand") : t("liveViewStopped"))}</div>`}
                </div>
                <div class="body">
                  <div class="preview-meta"><span>${esc(state.snapshotTime ? formatDate(state.snapshotTime) : t("snapshotOnDemand"))}</span><button data-view="kiosk-pages">${esc(t("managePages"))}</button></div>
                </div>
              </div>
            </div>
          </div>`;
      }

      function renderUpdateNotice() {
        const update = state.update || {};
        if (!update.update_available) return "";
        return `<section class="update-notice" role="status">
          <div><strong>${esc(t("updateAvailable"))}: ${esc(update.latest_version || "")}</strong><span>${esc(t("dashboardUpdateHint"))}</span></div>
          <button class="primary" data-view="settings-updates">${esc(t("openUpdate"))}</button>
        </section>`;
      }

      function renderKiosk() {
        const kiosk = state.config?.kiosk || {};
        const browser = state.status?.browser || {};
        const pages = normalizePages(kiosk.pages, kiosk.urls);
        const enabledPages = pages.filter((page) => !page.disabled && page.url).length;
        const selected = selectedKioskPageIndex(pages);
        const current = pages[selected] || {};
		const override = browser.override || {};
        return `
          <div class="page-stack kiosk-workspace">
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
            <section class="kiosk-overview" aria-label="${esc(t("workflowStatus"))}">
              <div><span>${esc(t("pages"))}</span><strong>${enabledPages} / ${pages.length}</strong></div>
              <div><span>${esc(t("scheduler"))}</span><strong class="${kiosk.scheduler?.enabled ? "ok-text" : ""}">${esc(kiosk.scheduler?.enabled ? t("active") : t("disabled"))}</strong></div>
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

      function selectedKioskPageIndex(pages) {
        const fallback = Number(state.status?.browser?.active || 0);
        const selected = state.kioskSelectedPageIndex === null ? fallback : Number(state.kioskSelectedPageIndex);
        return Math.max(0, Math.min(pages.length - 1, Number.isFinite(selected) ? selected : 0));
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

      function renderMQTT() {
        const mqtt = state.config?.mqtt || {};
        const runtime = state.status?.mqtt || {};
        const mqttTone = runtime.connected ? "ok" : runtime.state === "auth_error" || runtime.state === "error" ? "bad" : "warn";
        const mqttTitle = runtime.connected ? t("mqttReady") : mqtt.enabled ? t("mqttNeedsAttention") : t("mqttDisabledTitle");
        const mqttMessage = runtime.connected
          ? `${t("mqttConnectedHint")} ${mqtt.url || ""}`.trim()
          : (runtime.last_error || (mqtt.enabled ? t("mqttSaveToApply") : t("mqttEnablePrompt")));
        const passwordConfigured = !!mqtt.password_configured || !!state.status?.config?.mqtt_password_configured;
        return `
          <div class="page-stack">
            ${stateBanner(mqttTone, mqttTitle, mqttMessage, button("testConnection", "mqtt-test", runtime.connected ? "" : "primary"))}
            <section class="status-strip">
              ${statusTile(t("mqttService"), formatMQTTState(runtime.state), runtime.connected ? "ok" : runtime.state === "error" || runtime.state === "auth_error" ? "bad" : "", runtime.last_error || (mqtt.version ? `MQTT ${mqtt.version}` : "-"))}
              ${statusTile(t("broker"), mqtt.url || t("notConfigured"), mqtt.url ? "" : "warn", mqtt.username || t("anonymous"))}
              ${statusTile(t("homeAssistantDiscovery"), mqtt.discovery || "homeassistant", mqtt.enabled ? "ok" : "", `${mqtt.base_topic || "kioskmate"}/${mqtt.node || "kioskmate"}`)}
              ${statusTile(t("lastPublished"), formatDate(runtime.last_published), runtime.connected ? "ok" : "", `${t("failures")}: ${runtime.consecutive_failures || 0}`)}
            </section>
            <div class="settings-columns">
              <div class="card">
                <div class="head"><div><h3>${esc(t("connection"))}</h3><span class="section-kicker">${esc(t("mqttSettings"))}</span></div>${switchHtml("mqtt-enabled", t("enabled"), !!mqtt.enabled)}</div>
                <div class="body form-grid">
                  ${field("mqtt-url", t("mqttUrl"), "text", "", mqtt.url || "")}
                  ${selectHtml("mqtt-version", t("mqttVersion"), mqtt.version || "3.1.1", [["3.1.1", "MQTT 3.1.1"], ["5.0", "MQTT 5.0"]])}
                  ${field("mqtt-user", t("username"), "text", "username", mqtt.username || "")}
                  ${field("mqtt-password", passwordConfigured ? `${t("password")} (${t("configured")})` : t("password"), "password", "new-password", "", passwordConfigured ? "••••••••" : "")}
                </div>
              </div>
              <div class="card">
                <div class="head"><div><h3>${esc(t("homeAssistantDiscovery"))}</h3><span class="section-kicker">${esc(t("pageEntities"))}</span></div><div class="actions">${button("publishDiscovery", "mqtt-discovery", "primary")}${button("resetDiscovery", "mqtt-discovery-reset")}</div></div>
                <div class="body form-grid">
                  ${field("mqtt-discovery", t("discoveryPrefix"), "text", "", mqtt.discovery || "homeassistant")}
                  ${field("mqtt-base-topic", t("baseTopic"), "text", "", mqtt.base_topic || "kioskmate")}
                  ${field("mqtt-node", t("node"), "text", "", mqtt.node || "kioskmate")}
                  <div class="span-2 topic-preview"><span>${esc(t("commandTopic"))}</span><strong id="mqtt-topic">${esc(commandTopic())}</strong></div>
                </div>
              </div>
            </div>
            <section class="readiness-grid" aria-label="${esc(t("mqttReadiness"))}">
              ${readinessItem(t("brokerAddress"), !!mqtt.url, mqtt.url || t("notConfigured"))}
              ${readinessItem(t("credentials"), passwordConfigured || !mqtt.username, mqtt.username ? (passwordConfigured ? t("configured") : t("passwordMissing")) : t("anonymous"))}
              ${readinessItem(t("protocol"), !!mqtt.version, mqtt.version ? `MQTT ${mqtt.version}` : t("notConfigured"))}
              ${readinessItem(t("homeAssistantDiscovery"), runtime.connected && !!mqtt.discovery, runtime.connected ? `${mqtt.discovery || "homeassistant"}/…` : t("requiresConnection"))}
            </section>
            <details class="card disclosure advanced-settings">
              <summary>${esc(t("advancedSettings"))}</summary>
              <div class="disclosure-body form-grid">
                ${field("mqtt-client-id", t("clientId"), "text", "", mqtt.client_id || "")}
                ${field("mqtt-keepalive", t("keepalive"), "number", "", secondsToDuration(mqtt.keepalive, 60))}
                ${field("mqtt-interval", t("interval"), "number", "", secondsToDuration(mqtt.interval, 30))}
                <div>${switchHtml("mqtt-disable-retain", t("forceDisableRetain"), !!mqtt.force_disable_retain)}</div>
              </div>
            </details>
            <div class="card result-panel">
              <div class="head"><h3>${esc(t("connectionProtocol"))}</h3><span class="section-kicker">${esc(t("lastTestResult"))}</span></div>
              <pre id="mqtt-result" class="logbox compact-log empty-log">${esc(t("mqttNoTestResult"))}</pre>
            </div>
            <div class="save-bar"><span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span><div class="actions">${button("testConnection", "mqtt-test")}${button("save", "mqtt-save", "primary")}</div></div>
          </div>`;
      }

      function renderHardware() {
        const hw = state.hardware || {};
        const timeCfg = state.config?.time || {};
        const support = hw.support || {};
        const timeInfo = state.time || {};
        const zone = timeInfo.timezone || timeCfg.timezone || "Europe/Berlin";
        const zoneOptions = (state.timezones.length ? state.timezones : ["UTC", "Europe/Berlin"]).map((item) => [item, item]);
        return `
          <div class="page-stack">
            <section class="status-strip">
              ${statusTile("CPU", formatValue(hw.system?.processor_usage_percent, "%"), Number(hw.system?.processor_usage_percent || 0) > 250 ? "warn" : "", `${t("temperature")} ${formatValue(hw.system?.processor_temperature_c, " C")}`)}
              ${statusTile("RAM", formatValue(hw.system?.memory_usage_percent, "%"), Number(hw.system?.memory_usage_percent || 0) > 85 ? "warn" : "", formatValue(hw.system?.memory_size_gib, " GiB"))}
              ${statusTile(t("storage"), formatValue(hw.system?.disk_usage?.percent, "%"), "", formatValue(hw.system?.disk_usage?.available_gb, " GB " + t("available")))}
              ${statusTile(t("display"), formatValue(hw.display?.power), String(hw.display?.power || "").toUpperCase() === "ON" ? "ok" : "", `${t("brightness")} ${formatValue(hw.display?.brightness, "%")}`)}
              ${statusTile(t("time"), formatClock(timeInfo.current_time), timeInfo.synchronized ? "ok" : "warn", timeInfo.synchronized ? t("timeSynchronized") : t("timeNotSynchronized"))}
            </section>
            <div class="settings-columns">
              <div class="card">
                <div class="head"><div><h3>${esc(t("displayAndInput"))}</h3><span class="section-kicker">${esc(hw.display?.command || t("unsupported"))}</span></div></div>
                <div class="body form-grid">${support.display_status ? selectHtml("display-power", t("display"), String(hw.display?.power || "ON").toUpperCase(), [["ON", t("on")], ["OFF", t("off")]]) : unsupportedControl(t("display"))}${support.display_brightness ? field("display-brightness", t("brightness"), "number", "", hw.display?.brightness ?? 80) : unsupportedControl(t("brightness"))}${support.display_status || support.display_brightness ? `<button data-action="display-apply" class="primary span-2">${esc(t("applyDisplay"))}</button>` : ""}</div>
              </div>
              <div class="card">
                <div class="head"><div><h3>${esc(t("audio"))}</h3><span class="section-kicker">${esc(t("volumeAndMicrophone"))}</span></div></div>
                <div class="body form-grid">${support.audio_volume ? field("audio-volume", t("volume"), "number", "", hw.audio?.volume ?? 50) : unsupportedControl(t("volume"))}${support.microphone_volume ? field("audio-mic", t("microphone"), "number", "", hw.audio?.microphone ?? 50) : unsupportedControl(t("microphone"))}${support.keyboard_visibility ? selectHtml("keyboard-power", t("keyboard"), "ON", [["ON", t("on")], ["OFF", t("off")]]) : unsupportedControl(t("keyboard"))}${support.audio_volume || support.microphone_volume || support.keyboard_visibility ? `<button data-action="audio-apply" class="primary">${esc(t("applyAudio"))}</button>` : ""}</div>
              </div>
            </div>
            <div class="settings-columns">
              <div class="card">
                <div class="head"><div><h3>${esc(t("timeSettings"))}</h3><span class="section-kicker">${esc(timeInfo.service || t("timeAndTimezone"))}</span></div><span class="chip ${timeInfo.synchronized ? "ok" : "warn"}">${esc(timeInfo.synchronized ? t("timeSynchronized") : t("timeNotSynchronized"))}</span></div>
                <div class="body form-grid">
                  ${field("time-ntp", t("ntpServer"), "text", "", timeCfg.ntp_server || "pool.ntp.org")}
                  ${selectHtml("time-zone", t("timezone"), zone, zoneOptions)}
                  <button data-action="time-save" class="primary span-2">${esc(t("applyTimeSettings"))}</button>
                </div>
              </div>
              <div class="card"><div class="head"><h3>${esc(t("device"))}</h3></div><div class="body">${kvTable(objectEntries(hw.device))}</div></div>
            </div>
            <details class="card disclosure"><summary>${esc(t("technicalDetails"))}</summary><div class="disclosure-body settings-columns"><div>${kvTable(objectEntries(hw.system))}</div><div>${kvTable(objectEntries(hw.support))}</div></div></details>
          </div>`;
      }

      function renderSystemActions() {
        const privilege = state.privilege || {};
        const repairIssues = (state.repair?.issues || []).filter((issue) => issue.id !== "ok");
        const activeJobs = state.jobs.filter((job) => !job.finished).length;
        return `
          <div class="page-stack">
            ${stateBanner(privilege.configured ? "ok" : "warn", privilege.configured ? t("maintenanceReady") : t("privilegeRequired"), privilege.configured ? t("maintenanceReadyHint") : t("privilegeRequiredHint"))}
            <section class="status-strip">
              ${statusTile(t("privilege"), privilege.configured ? t("configured") : t("notConfigured"), privilege.configured ? "ok" : "warn", privilege.mode || "sudo")}
              ${statusTile(t("jobs"), String(activeJobs), activeJobs ? "warn" : "ok", activeJobs ? t("jobRunning") : t("noActiveJobs"))}
              ${statusTile(t("repairCenter"), repairIssues.length ? `${repairIssues.length} ${t("issues")}` : t("ready"), repairIssues.length ? "warn" : "ok", state.repair?.changed ? t("repairChanged") : t("noRepairIssues"))}
            </section>
            <div class="settings-columns">
              <div class="card">
                <div class="head"><div><h3>${esc(t("privilege"))}</h3><span class="section-kicker">${esc(t("privilegeSession"))}</span></div><span class="chip ${privilege.configured ? "ok" : ""}">${esc(privilege.configured ? t("configured") : t("notConfigured"))}</span></div>
                <div class="body form-grid">
                  ${selectHtml("priv-mode", t("privilegeMode"), privilege.mode || "sudo", [["sudo", "sudo"], ["su", "su / root"]])}
                  ${field("priv-password", t("password"), "password", "current-password", "")}
                  <div class="span-2">${switchHtml("priv-remember", t("rememberPassword"), false)}</div>
                  ${privilege.configured ? `<button data-action="priv-clear" class="span-2">${esc(t("clearPrivilege"))}</button>` : `<p class="hint span-2">${esc(t("privilegeStorageHint"))}</p>`}
                </div>
              </div>
              <div class="card">
                <div class="head"><div><h3>${esc(t("systemActions"))}</h3><span class="section-kicker">${esc(t("privilegedActions"))}</span></div></div>
                <div class="body action-list">
                  <div><span>${esc(t("packageMaintenance"))}</span><div class="actions">${button("aptUpdate", "sys-apt-update")}${button("aptUpgrade", "sys-apt-upgrade")}</div></div>
                  <div><span>${esc(t("service"))}</span><div class="actions">${button("restartService", "sys-restart-service", "primary")}</div></div>
                </div>
                <details class="danger-disclosure">
                  <summary>${esc(t("powerActions"))}</summary>
                  <div><p>${esc(t("powerActionsHint"))}</p><div class="actions">${button("reboot", "sys-reboot", "danger-ghost")}${button("shutdown", "sys-shutdown", "danger")}</div></div>
                </details>
              </div>
            </div>
            <div class="card">
              <div class="head"><div><h3>${esc(t("repairCenter"))}</h3><span class="section-kicker">${esc(t("configurationCheck"))}</span></div><div class="actions">${button("refresh", "repair-check")}${button("runRepair", "repair-run", "primary")}</div></div>
              <div class="body">${renderRepair()}</div>
            </div>
            <div class="card">
              <div class="head"><div><h3>${esc(t("jobs"))}</h3><span class="section-kicker">${esc(t("jobOutput"))}</span></div></div>
              <div class="body job-list" id="job-output">${renderJobsHTML()}</div>
            </div>
          </div>`;
      }

      function renderTerminal() {
        return `
          <div class="page-stack">
            <div class="card">
              <div class="head"><div><h3>${esc(t("terminal"))}</h3><span class="section-kicker">${esc(t("terminalHint"))}</span></div></div>
              <div class="body grid">
                <div class="terminal-command">
                  <input id="terminal-command" aria-label="${esc(t("command"))}" value="systemctl --user status kioskmate.service --no-pager" />
                  <button class="primary" data-busy="terminal-run" data-action="terminal-run">${esc(t("runCommand"))}</button>
                </div>
                <pre class="terminal">${esc(state.terminal || "")}</pre>
              </div>
            </div>
          </div>`;
      }

      function renderLogs() {
        const sources = [
          ["combined", t("logCombined")],
          ["core", t("logCore")],
          ["browser", t("logBrowser")],
          ["journal", t("logJournal")],
          ["status", t("logStatus")],
          ["paths", t("logPaths")],
        ];
        const visibleLogs = filteredLogs();
        return `
          <div class="page-stack">
            <section class="section-toolbar log-toolbar">
              <div class="log-filters">
                ${selectHtml("log-source", t("logSource"), state.logSource, sources)}
                ${field("log-lines", t("logLines"), "number", "", 300)}
                ${field("log-filter", t("filterLogs"), "search", "", state.logFilter)}
              </div>
              <div class="actions">
                <button class="primary" data-busy="logs-refresh" data-action="logs-refresh">${esc(t("refreshLogs"))}</button>
                <button data-action="logs-download">${esc(t("downloadLogs"))}</button>
                <button data-action="diagnostics-download">${esc(t("diagnosticBundle"))}</button>
              </div>
            </section>
            ${state.logWarning ? `<div class="notice warn">${esc(state.logWarning)}</div>` : ""}
            <div class="card">
              <div class="head"><div><h3>${esc(t("logs"))}</h3><span id="log-result-count" class="section-kicker">${esc(t("logEntries"))}: ${visibleLogs.length}</span></div></div>
              <pre id="log-output" class="logbox log-output ${visibleLogs.length ? "" : "empty-log"}">${esc(visibleLogs.length ? visibleLogs.join("\n") : t("noLogsAvailable"))}</pre>
            </div>
          </div>`;
      }

      function renderSettingsAdmin() {
        const cfg = state.config || {};
        const endpoint = `${cfg.admin?.bind || "0.0.0.0"}:${cfg.admin?.port || 33333}`;
        return `
          <div class="page-stack">
            <section class="status-strip">
              ${statusTile(t("adminEndpoint"), endpoint, "ok", t("adminEndpointHint"))}
              ${statusTile(t("sessions"), String(state.sessions.length), "", t("activeAdminSessions"))}
              ${statusTile(t("sshKey"), state.ssh?.exists ? t("configured") : t("notConfigured"), state.ssh?.exists ? "ok" : "", state.ssh?.path || "-")}
            </section>
            <div class="grid two">
              <div class="card">
                <div class="head"><div><h3>${esc(t("adminSettings"))}</h3><span class="section-kicker">${esc(t("networkAccess"))}</span></div><button class="primary" data-action="admin-save">${esc(t("save"))}</button></div>
                <div class="body form-grid">
                  ${field("admin-bind", t("bindAddress"), "text", "", cfg.admin?.bind || "0.0.0.0")}
                  ${field("admin-port", t("port"), "number", "", cfg.admin?.port || 33333)}
                </div>
              </div>
              <div class="card">
                <div class="head"><div><h3>${esc(t("changePassword"))}</h3><span class="section-kicker">${esc(t("accountSecurity"))}</span></div><button data-action="password-save">${esc(t("save"))}</button></div>
                <div class="body form-grid">
                  ${field("password-current", t("currentPassword"), "password", "current-password", "")}
                  ${field("password-next", t("newPassword"), "password", "new-password", "")}
                </div>
              </div>
            </div>
            <div class="grid two">
              <div class="card">
                <div class="head"><h3>${esc(t("sessions"))}</h3><button data-action="sessions-logout-all">${esc(t("logoutAll"))}</button></div>
                <div class="body">${renderSessions()}</div>
              </div>
              <div class="card">
                <div class="head"><h3>${esc(t("sshKey"))}</h3><button data-action="ssh-generate">${esc(t("generateKey"))}</button></div>
                <div class="body grid">
                  ${kvTable([[t("configFile"), state.ssh?.path || "-"], [t("configured"), state.ssh?.exists ? t("yes") : t("no")]])}
                  <pre class="terminal">${esc(state.ssh?.public_key || "")}</pre>
                </div>
              </div>
            </div>
          </div>`;
      }

      function renderSettingsBrowser() {
        const cfg = state.config || {};
        const kiosk = cfg.kiosk || {};
        const perf = cfg.performance || {};
        const watchdog = cfg.watchdog || {};
		const recommendation = state.status?.profile_recommendation || {};
		const telemetry = state.telemetry?.summary || state.status?.browser?.telemetry || {};
        return `
          <div class="page-stack">
            <div class="settings-columns">
              <div class="card">
                <div class="head"><div><h3>${esc(t("appearance"))}</h3><span class="section-kicker">${esc(t("displayRendering"))}</span></div></div>
                <div class="body form-grid">
                  ${selectHtml("kiosk-theme", t("themeField"), kiosk.theme || "dark", [["dark", t("dark")], ["light", t("light")], ["force-dark", t("forceDark")]])}
                  ${field("kiosk-zoom", t("zoomPercent"), "number", "", kiosk.zoom_percent || 125)}
                  <div class="span-2">${switchHtml("kiosk-widget", t("widgetFlag"), kiosk.widget !== false)}</div>
                  <p class="hint span-2">${esc(t("themeSyncHint"))}</p>
                </div>
              </div>
              <div class="card">
                <div class="head"><div><h3>${esc(t("performance"))}</h3><span class="section-kicker">${esc(t("resourceProfile"))}</span></div>${button("safeMode", "safe-mode")}</div>
                <div class="body form-grid">
                  ${selectHtml("perf-profile", t("performanceProfile"), perf.profile || "low-power", [["low-power", t("lowPower")], ["raspberry", t("raspberry")], ["minimal", t("minimal")], ["balanced", t("balanced")], ["quality", t("quality")], ["conservative", t("conservative")]])}
                  ${selectHtml("perf-gpu", t("gpuMode"), perf.gpu_mode || "auto", [["auto", t("auto")], ["software", t("software")], ["hardware", t("hardwareMode")]])}
                  <div class="span-2">${switchHtml("perf-reduce", t("reduceMotion"), perf.reduce_motion !== false)}</div>
				  ${recommendation.profile ? `<div class="notice span-2 recommendation"><div><strong>${esc(t("recommendedProfile"))}: ${esc(recommendation.profile)} / ${esc(recommendation.gpu_mode)}</strong><span>${esc(recommendation.reason_key ? t(recommendation.reason_key) : recommendation.reason || "")}</span></div><button data-action="profile-recommendation-apply">${esc(t("applyRecommendation"))}</button></div>` : ""}
                </div>
              </div>
            </div>
			<div class="card">
			  <div class="head"><div><h3>${esc(t("runtimeTelemetry"))}</h3><span class="section-kicker">${esc(t("last24Hours"))}</span></div><button data-action="telemetry-reset">${esc(t("resetTelemetry"))}</button></div>
			  <div class="body telemetry-panel">
				${statusTile(t("averageCpu"), formatValue(telemetry.cpu_average, "%"), "", `${t("maximum")}: ${formatValue(telemetry.cpu_maximum, "%")}`)}
				${statusTile(t("averageMemory"), formatValue(telemetry.rss_average_mb, " MB"), "", `${t("maximum")}: ${formatValue(telemetry.rss_maximum_mb, " MB")}`)}
				${statusTile(t("samples"), telemetry.samples || 0, "", `${t("processes")}: ${telemetry.process_maximum || 0}`)}
				${renderTelemetryChart(state.telemetry?.samples || [])}
			  </div>
			</div>
            <div class="card">
              <div class="head"><div><h3>${esc(t("watchdog"))}</h3><span class="section-kicker">${esc(t("browserProtection"))}</span></div>${switchHtml("watchdog-enabled", t("enabled"), watchdog.enabled !== false)}</div>
              <div class="body form-grid four-fields">
                ${field("watchdog-rss", t("maxMemory"), "number", "", watchdog.max_rss_mb || 900)}
                ${field("watchdog-cpu", t("maxCpu"), "number", "", watchdog.max_cpu_percent || 300)}
                ${field("watchdog-grace", t("cpuGrace"), "number", "", secondsToDuration(watchdog.cpu_grace, 600))}
                ${field("watchdog-interval", t("checkInterval"), "number", "", secondsToDuration(watchdog.check_interval, 10))}
              </div>
            </div>
            <details class="card disclosure advanced-settings">
              <summary>${esc(t("advancedBrowserSettings"))}</summary>
              <div class="disclosure-body form-grid">
                ${selectHtml("kiosk-browser-preset", t("browserPreset"), kiosk.browser_preset || "chromium", browserPresetOptions())}
                ${field("kiosk-browser", t("browserCommand"), "text", "", kiosk.browser_command || "")}
                ${field("kiosk-user-data", t("browserProfile"), "text", "", kiosk.user_data_dir || "")}
                <div>${switchHtml("kiosk-isolate-sessions", t("isolatePageSessions"), !!kiosk.isolate_page_sessions)}</div>
                <div class="span-2">${textarea("kiosk-extra-args", t("extraArgs"), (kiosk.extra_args || []).join("\n"))}</div>
                <div class="span-2">${button("testBrowser", "browser-diagnostics")}</div>
                ${state.diagnostics ? `<div class="span-2">${kvTable(objectEntries(state.diagnostics))}</div>` : ""}
              </div>
            </details>
            <div class="save-bar"><span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span><div class="actions">${button("save", "browser-settings-save")}${button("saveRestart", "browser-settings-save-restart", "primary")}</div></div>
          </div>`;
      }

      function renderSettingsConfig() {
        const cfg = state.config || {};
        return `
          <div class="page-stack">
            <div class="card">
              <div class="head"><div><h3>${esc(t("configAndBackups"))}</h3><span class="section-kicker">${esc(cfg.path || "-")}</span></div><div class="actions">${button("exportConfig", "config-export")}${button("importConfig", "config-import")}</div></div>
              <div class="body">
                <input id="config-import-file" type="file" accept="application/json,.json" class="hidden" />
                ${renderBackups()}
              </div>
            </div>
            <details class="card disclosure advanced-settings">
              <summary>${esc(t("rawConfig"))}</summary>
              <div class="disclosure-body grid">
                ${textarea("config-raw", t("rawConfig"), JSON.stringify(cfg, null, 2))}
                <div class="actions">${button("saveRaw", "config-raw-save", "primary")}</div>
              </div>
            </details>
          </div>`;
      }

      function renderSettingsMaintenance() {
        const update = state.update || {};
        const privilege = state.privilege || {};
        const passwordless = !!state.hardware?.support?.sudo_rights;
        const hasPrivilege = privilege.configured || passwordless;
        const checked = update.checked_at ? formatDate(update.checked_at) : t("notChecked");
        return `
          <div class="page-stack">
            <section class="status-strip">
              ${statusTile(t("installed"), update.current_version || update.current || state.auth?.version || "-", "ok", t("currentVersion"))}
              ${statusTile(t("latest"), update.latest_version || update.latest || t("notChecked"), update.update_available ? "warn" : "", update.update_available ? t("updateAvailable") : t("upToDate"))}
              ${statusTile(t("lastUpdateCheck"), checked, update.error ? "bad" : "", update.checking ? t("checkingUpdate") : update.error || t("automaticUpdateCheck"))}
              ${statusTile(t("administratorRights"), hasPrivilege ? t("ready") : t("required"), hasPrivilege ? "ok" : "warn", passwordless ? t("passwordlessSudo") : privilege.configured ? t("temporaryPrivilegeActive") : t("enterPasswordBelow"))}
            </section>
            <div class="card">
              <div class="head"><div><h3>${esc(t("update"))}</h3><span class="section-kicker">${esc(t("releaseChannel"))}: ${esc(update.channel || "stable")}</span></div><div class="actions">${button("checkUpdate", "update-check")}</div></div>
              <div class="body update-layout">
                <div>
                  <label>${esc(t("changelog"))}</label>
                  <pre class="jsonbox">${esc(update.changelog || t("noChangelog"))}</pre>
                </div>
                <div class="update-control">
                  <div class="notice ${hasPrivilege ? "" : "warn"}"><strong>${esc(hasPrivilege ? t("updateReadyToInstall") : t("updateNeedsPrivilege"))}</strong><span>${esc(hasPrivilege ? t("updateReadyHint") : t("updatePrivilegeHint"))}</span></div>
                  <div class="form-grid one">
                    ${selectHtml("update-priv-mode", t("privilegeMode"), privilege.mode || "sudo", [["sudo", "sudo"], ["su", "su / root"]])}
                    ${field("update-priv-password", t("administratorPassword"), "password", "current-password", "")}
                    ${switchHtml("update-priv-remember", t("rememberFor15Minutes"), false)}
                  </div>
                  <p class="hint">${esc(t("passwordMemoryOnlyHint"))}</p>
                  <div id="update-preflight-result">${renderUpdatePreflight()}</div>
                  <div class="update-commands">
                    <button data-busy="update-preflight" data-action="update-preflight" ${update.installing ? "disabled" : ""}>${esc(t("runUpdatePreflight"))}</button>
                    <button class="primary" data-busy="update-install" data-action="update-install" ${update.installing || !update.update_available ? "disabled" : ""}>${esc(update.installing ? t("updateInstalling") : t("installUpdate"))}</button>
                  </div>
                </div>
              </div>
            </div>
            <div class="card">
              <div class="head"><div><h3>${esc(t("updateProgress"))}</h3><span class="section-kicker">${esc(t("updateProgressHint"))}</span></div></div>
              <div class="body job-list" id="update-job-output">${renderUpdateJobsHTML()}</div>
            </div>
            <div class="card">
              <div class="head"><div><h3>${esc(t("updateHistory"))}</h3><span class="section-kicker">${esc(t("updateHistoryHint"))}</span></div><div class="actions">${state.updateHistory?.rollback_available ? `<button class="danger-ghost" data-busy="update-rollback" data-action="update-rollback">${esc(t("rollbackTo"))} ${esc(state.updateHistory.rollback_target)}</button>` : ""}</div></div>
              <div class="body">${renderUpdateHistory()}</div>
            </div>
          </div>`;
      }

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
            if (source) cfg.kiosk.pages.splice(index + 1, 0, { ...source, name: `${source.name || t("pages")} ${t("copy")}` });
            syncKioskURLs(cfg.kiosk);
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
              ${switchHtml("wizard-screensaver", t("activateScreensaver"), !!page.display_options?.screensaver)}
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
            screensaver: checked("wizard-screensaver"),
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
          cfg.kiosk.scheduler.enabled = checked("scheduler-enabled");
          cfg.kiosk.scheduler.tick_interval = durationToNs(val("scheduler-tick") || secondsToDuration(cfg.kiosk.scheduler.tick_interval, 15));
          synchronizeKioskWorkflow(cfg.kiosk);
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
		const active = selectedKioskPageIndex(collectPages());
        await runAction("page-activate", async () => {
          await postJSON("/api/browser/page", { index: active });
          await refreshCore();
          renderApp();
        });
      }

	  async function applyPageOverride() {
		const page = selectedKioskPageIndex(collectPages());
		const durationMinutes = Math.max(1, Math.min(1440, Number(val("override-duration") || 60)));
		await runAction("override-apply", async () => {
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
          if ((page.display_mode || "duration") === "duration") rotation.push({ page: enabledIndex, duration_seconds: Math.max(5, Number(page.duration_seconds || 60)) });
          if (page.display_mode === "schedule") rules.push({ name: page.name || `Page ${enabledIndex + 1}`, page: enabledIndex, start: page.schedule?.start || "08:00", end: page.schedule?.end || "18:00", days: [...(page.schedule?.days || [])], disabled: false });
          enabledIndex++;
        });
        kiosk.rotation = rotation;
        kiosk.time_rules = rules;
        kiosk.scheduler = kiosk.scheduler || {};
        kiosk.scheduler.mode = rules.length && rotation.length ? "hybrid" : rules.length ? "time" : "rotation";
      }

      function movePage(kiosk, index, direction) {
        const pages = kiosk.pages || [];
        const next = index + direction;
        if (index < 0 || next < 0 || index >= pages.length || next >= pages.length) return;
        const order = pages.map((_, i) => i);
        [order[index], order[next]] = [order[next], order[index]];
        kiosk.pages = order.map((oldIndex) => pages[oldIndex]);
        remapPageIndexes(kiosk, order);
        syncKioskURLs(kiosk);
      }

      function removePage(kiosk, index) {
        const pages = kiosk.pages || [];
        const order = pages.map((_, i) => i).filter((oldIndex) => oldIndex !== index);
        kiosk.pages = order.map((oldIndex) => pages[oldIndex]);
        remapPageIndexes(kiosk, order);
        syncKioskURLs(kiosk);
      }

      function remapPageIndexes(kiosk, order) {
        const map = new Map(order.map((oldIndex, newIndex) => [oldIndex, newIndex]));
        const remap = (page) => map.has(Number(page)) ? map.get(Number(page)) : 0;
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
          validateScheduler(cfg.kiosk);
          await postJSON("/api/config", cfg);
          await refreshCore();
          clearDirty("kiosk-schedule");
          renderApp();
        }, t("saved"));
      }

      function bindMQTT() {
        ["mqtt-node", "mqtt-base-topic", "mqtt-discovery"].forEach((id) => document.getElementById(id)?.addEventListener("input", () => {
          const topic = document.getElementById("mqtt-topic");
          if (topic) topic.textContent = commandTopic();
        }));
        document.querySelectorAll('[data-action="mqtt-save"]').forEach((button) => button.addEventListener("click", saveMQTT));
        document.querySelectorAll('[data-action="mqtt-test"]').forEach((button) => button.addEventListener("click", testMQTT));
        document.querySelectorAll('[data-action="mqtt-discovery"]').forEach((button) => button.addEventListener("click", publishMQTTDiscovery));
        document.querySelectorAll('[data-action="mqtt-discovery-reset"]').forEach((button) => button.addEventListener("click", resetMQTTDiscovery));
      }

      async function saveMQTT() {
        await runAction("mqtt-save", async () => {
          const cfg = cloneConfig();
          cfg.mqtt = cfg.mqtt || {};
          const mqttUpdate = {
            enabled: checked("mqtt-enabled"),
            url: val("mqtt-url"),
            version: val("mqtt-version"),
            username: val("mqtt-user"),
            discovery: val("mqtt-discovery") || "homeassistant",
            base_topic: val("mqtt-base-topic") || "kioskmate",
            node: val("mqtt-node") || "kioskmate",
            client_id: val("mqtt-client-id"),
            keepalive: durationToNs(val("mqtt-keepalive") || 60),
            force_disable_retain: checked("mqtt-disable-retain"),
            interval: durationToNs(val("mqtt-interval")),
          };
          const password = val("mqtt-password");
          if (password) mqttUpdate.password = password;
          Object.assign(cfg.mqtt, mqttUpdate);
          delete cfg.mqtt.password_configured;
          validateMQTT(cfg.mqtt);
          await postJSON("/api/config", cfg);
          await refreshCore();
          clearDirty("mqtt");
          renderApp();
          if (cfg.mqtt.enabled) await pollMQTTRuntime();
        }, t("saved"));
      }

      async function pollMQTTRuntime() {
        for (let i = 0; i < 8; i++) {
          await sleep(1000);
          try {
            const status = await getJSON("/api/status?fast=1", { timeout: 8000 });
            if (status) {
              applyCoreState({ status });
              if (state.view === "mqtt") renderApp();
              const stateName = status.mqtt?.state || "";
              if (status.mqtt?.connected || stateName === "auth_error" || stateName === "error" || stateName === "disabled") {
                return;
              }
            }
          } catch (_) {}
        }
      }

      async function testMQTT() {
        const output = document.getElementById("mqtt-result");
        await runAction("mqtt-test", async () => {
          openMQTTTestDialog();
          const payload = {
            url: val("mqtt-url"),
            version: val("mqtt-version"),
            username: val("mqtt-user"),
            password: val("mqtt-password"),
            discovery: val("mqtt-discovery") || "homeassistant",
            base_topic: val("mqtt-base-topic") || "kioskmate",
            node: val("mqtt-node"),
            client_id: val("mqtt-client-id"),
            keepalive_seconds: Number(val("mqtt-keepalive") || 60),
            force_disable_retain: checked("mqtt-disable-retain"),
          };
          if (output) output.textContent = `${t("loading")}...`;
          const controller = new AbortController();
          const timer = setTimeout(() => controller.abort(), 35_000);
          let result = null;
          try {
            await streamJSONLines("/api/mqtt/test/live", payload, (event) => {
              appendMQTTEvent(event);
              if (event.result) result = event.result;
              if (event.step === "result" && event.result) result = event.result;
            }, controller.signal);
            if (!result) throw new Error("MQTT test did not return a result");
          } catch (err) {
            const message = err.name === "AbortError" ? "Timeout after 35s" : err.message;
            appendMQTTEvent({
              step: "client",
              status: "error",
              message,
              elapsed_ms: 0,
            });
            if (output) output.textContent = `${t("disconnected")}: ${message}`;
            throw err;
          } finally {
            clearTimeout(timer);
          }
          const topics = (result.published_topics || []).join("\n");
          if (output) output.textContent = result.ok
            ? `${t("connected")} (${result.latency_ms} ms)\n${t("publishedTopics")}:\n${topics}`
            : `${t("disconnected")}: ${result.error || t("unknown")}`;
        }, t("testConnection"));
      }

      async function publishMQTTDiscovery() {
        const output = document.getElementById("mqtt-result");
        await runAction("mqtt-discovery", async () => {
          const result = await postJSON("/api/mqtt/discovery", {});
          if (!result.ok) throw new Error(result.error || t("failed"));
          if (output) output.textContent = [
            `${t("success")}: ${t("publishDiscovery")}`,
            `${t("discoveryPrefix")}: ${result.discovery_prefix || val("mqtt-discovery")}`,
            `${t("rootTopic")}: ${result.root_topic || commandTopic().replace("/command", "")}`,
            `${t("commandTopic")}: ${commandTopic()}`,
            `${t("pages")}: ${result.page_count ?? "-"}`,
            `${t("pageEntities")}: ${result.page_entities ?? "-"}`,
          ].join("\n");
        }, t("publishDiscovery"));
      }

      async function resetMQTTDiscovery() {
        const output = document.getElementById("mqtt-result");
        await runAction("mqtt-discovery-reset", async () => {
          const result = await postJSON("/api/mqtt/discovery-reset", {});
          if (!result.ok) throw new Error(result.error || t("failed"));
          if (output) output.textContent = `${t("success")}: ${t("resetDiscovery")}\n${t("publishedTopics")}: ${result.cleared || 0}`;
        }, t("resetDiscovery"));
      }

      function bindHardware() {
        document.querySelector('[data-action="display-apply"]')?.addEventListener("click", async () => {
          await runAction("display-apply", async () => {
            if (state.hardware?.support?.display_status) await postJSON("/api/hardware/display", { value: val("display-power") });
            if (state.hardware?.support?.display_brightness) await postJSON("/api/hardware/brightness", { value: Number(val("display-brightness") || 80) });
            state.hardware = await getJSON("/api/hardware");
            renderApp();
          });
        });
        document.querySelector('[data-action="audio-apply"]')?.addEventListener("click", async () => {
          await runAction("audio-apply", async () => {
            if (state.hardware?.support?.audio_volume) await postJSON("/api/hardware/volume", { value: Number(val("audio-volume") || 50) });
            if (state.hardware?.support?.microphone_volume) await postJSON("/api/hardware/microphone", { value: Number(val("audio-mic") || 50) });
            if (state.hardware?.support?.keyboard_visibility) await postJSON("/api/hardware/keyboard", { value: val("keyboard-power") });
            state.hardware = await getJSON("/api/hardware");
            renderApp();
          });
        });
      }

      function bindSystem() {
        const map = {
          "sys-apt-update": "apt-update",
          "sys-apt-upgrade": "apt-upgrade",
          "sys-restart-service": "restart-service",
          "sys-reboot": "reboot",
          "sys-shutdown": "shutdown",
        };
        for (const [action, name] of Object.entries(map)) {
          document.querySelector(`[data-action="${action}"]`)?.addEventListener("click", () => {
            if (action === "sys-reboot" && !window.confirm(t("confirmReboot"))) return;
            if (action === "sys-shutdown" && !window.confirm(t("confirmShutdown"))) return;
            startSystemJob(action, name);
          });
        }
        document.querySelector('[data-action="priv-clear"]')?.addEventListener("click", async () => {
          await deleteJSON("/api/privilege");
          state.privilege = await getJSON("/api/privilege");
          renderApp();
        });
        document.querySelector('[data-action="time-save"]')?.addEventListener("click", saveTimeSettings);
      }

      function bindSystemMaintenance() {
        bindSystem();
        document.querySelector('[data-action="repair-check"]')?.addEventListener("click", checkRepair);
        document.querySelector('[data-action="repair-run"]')?.addEventListener("click", runRepair);
        if (!state.loaded.repair) {
          state.loaded.repair = true;
          getJSON("/api/repair").then((data) => {
            state.repair = data;
            if (state.view === "system-maintenance") renderApp();
          }).catch(() => {});
        }
      }

      async function startSystemJob(action, name) {
        await runAction(action, async () => {
          const job = await postJSON("/api/system/" + name, { mode: val("priv-mode"), password: val("priv-password"), remember: checked("priv-remember") });
          state.jobs.unshift(job);
          renderApp();
          pollJob(job.id);
        }, t("actionStarted"));
      }

      async function saveTimeSettings() {
        await runAction("time-save", async () => {
          const job = await postJSON("/api/time", {
            ntp_server: val("time-ntp") || "pool.ntp.org",
            timezone: val("time-zone") || "Europe/Berlin",
            mode: val("priv-mode") || "sudo",
            password: val("priv-password"),
            remember: checked("priv-remember"),
          });
          state.jobs.unshift(job);
          pollJob(job.id);
          await refreshCore();
          clearDirty("system-device");
          renderApp();
        }, t("saved"));
      }

      async function pollJob(id) {
        for (let i = 0; i < 80; i++) {
          await new Promise((resolve) => setTimeout(resolve, 1000));
          try {
            const job = await getJSON("/api/jobs/" + encodeURIComponent(id));
            const index = state.jobs.findIndex((item) => item.id === id);
            if (index >= 0) state.jobs[index] = job;
            if (state.view === "system" || state.view === "system-maintenance") {
              const output = document.getElementById("job-output");
              if (output) output.innerHTML = renderJobsHTML();
            }
            if (job.finished) {
              if (Number(job.exit_code) !== 0) {
                const detail = (job.output || []).slice(-3).join("\n") || t("unknownError");
                toast(t("actionFailed"), detail, "bad");
              } else {
                toast(t("actionDone"), job.name || "", "ok");
              }
              return;
            }
          } catch {
            return;
          }
        }
      }

      function bindTerminal() {
        document.querySelector('[data-action="terminal-run"]')?.addEventListener("click", async () => {
          await runAction("terminal-run", async () => {
            const result = await postJSON("/api/terminal/run", { command: val("terminal-command") });
            state.terminal = `$ ${val("terminal-command")}\n${result.output || ""}${result.error ? "\n" + result.error : ""}`;
            renderApp();
          });
        });
      }

      function bindLogs() {
        document.querySelector('[data-action="logs-refresh"]')?.addEventListener("click", refreshLogs);
        document.querySelector('[data-action="logs-download"]')?.addEventListener("click", () => {
          const source = encodeURIComponent(state.logSource || "combined");
          const lines = encodeURIComponent(val("log-lines") || "300");
          window.location.href = `/api/logs/download?source=${source}&lines=${lines}`;
        });
        document.querySelector('[data-action="diagnostics-download"]')?.addEventListener("click", () => {
          window.location.href = "/api/diagnostics/export";
        });
        document.getElementById("log-source")?.addEventListener("change", (event) => {
          state.logSource = event.target.value;
          localStorage.setItem("kioskmate.logSource", state.logSource);
          refreshLogs().catch(() => {});
        });
        document.getElementById("log-filter")?.addEventListener("input", (event) => {
          state.logFilter = event.target.value;
          localStorage.setItem("kioskmate.logFilter", state.logFilter);
          updateFilteredLogs();
        });
        if (!state.loaded.logs) {
          state.loaded.logs = true;
          refreshLogs().catch(() => {});
        }
      }

      async function refreshLogs() {
        await runAction("logs-refresh", async () => {
          const source = state.logSource || "combined";
          const result = await getJSON("/api/logs?source=" + encodeURIComponent(source) + "&lines=" + encodeURIComponent(val("log-lines") || "300"));
          state.logs = result.lines || [];
          state.logSource = result.source || source;
          state.logWarning = result.warning || "";
          renderApp();
        }, t("refreshLogs"));
      }

      function filteredLogs() {
        const query = state.logFilter.trim().toLowerCase();
        if (!query) return state.logs || [];
        return (state.logs || []).filter((line) => String(line).toLowerCase().includes(query));
      }

      function updateFilteredLogs() {
        const lines = filteredLogs();
        const output = document.getElementById("log-output");
        const count = document.getElementById("log-result-count");
        if (output) {
          output.textContent = lines.length ? lines.join("\n") : t("noLogsAvailable");
          output.classList.toggle("empty-log", !lines.length);
        }
        if (count) count.textContent = `${t("logEntries")}: ${lines.length}`;
      }

      function bindSettings() {
        document.querySelector('[data-action="admin-save"]')?.addEventListener("click", saveAdminSettings);
        document.querySelector('[data-action="password-save"]')?.addEventListener("click", savePassword);
        document.querySelector('[data-action="browser-settings-save"]')?.addEventListener("click", () => saveBrowserSettings(false));
        document.querySelector('[data-action="browser-settings-save-restart"]')?.addEventListener("click", () => saveBrowserSettings(true));
        document.querySelector('[data-action="safe-mode"]')?.addEventListener("click", applySafeMode);
		document.querySelector('[data-action="profile-recommendation-apply"]')?.addEventListener("click", applyProfileRecommendation);
		document.querySelector('[data-action="telemetry-reset"]')?.addEventListener("click", resetTelemetry);
        document.querySelector('[data-action="browser-diagnostics"]')?.addEventListener("click", loadBrowserDiagnostics);
        document.querySelector('[data-action="config-export"]')?.addEventListener("click", () => { window.location.href = "/api/config/export"; });
        document.querySelector('[data-action="config-import"]')?.addEventListener("click", () => document.getElementById("config-import-file").click());
        document.getElementById("config-import-file")?.addEventListener("change", importConfig);
        document.querySelector('[data-action="config-raw-save"]')?.addEventListener("click", saveRawConfig);
        document.querySelector('[data-action="sessions-logout-all"]')?.addEventListener("click", logoutAllSessions);
        document.querySelector('[data-action="backups-refresh"]')?.addEventListener("click", loadBackups);
        document.querySelector('[data-action="update-check"]')?.addEventListener("click", checkUpdate);
        document.querySelector('[data-action="update-preflight"]')?.addEventListener("click", runUpdatePreflight);
        document.querySelector('[data-action="update-install"]')?.addEventListener("click", installUpdate);
        document.querySelector('[data-action="update-rollback"]')?.addEventListener("click", rollbackUpdate);
        document.querySelector('[data-action="repair-check"]')?.addEventListener("click", checkRepair);
        document.querySelector('[data-action="repair-run"]')?.addEventListener("click", runRepair);
        document.querySelector('[data-action="ssh-generate"]')?.addEventListener("click", async () => {
          await runAction("ssh-generate", async () => {
            state.ssh = await postJSON("/api/ssh-key");
            renderApp();
          });
        });
        document.querySelectorAll("[data-restore]").forEach((button) => button.addEventListener("click", () => restoreBackup(button.dataset.restore)));
        if (state.view === "settings-admin" && !state.loaded.sessions) {
          state.loaded.sessions = true;
          getJSON("/api/auth/sessions").then((data) => { state.sessions = data; if (state.view.startsWith("settings")) renderApp(); }).catch(() => {});
        }
        if (state.view === "settings-admin" && !state.loaded.ssh) {
          state.loaded.ssh = true;
          getJSON("/api/ssh-key").then((data) => { state.ssh = data; if (state.view.startsWith("settings")) renderApp(); }).catch(() => {});
        }
        if (state.view === "settings-config" && !state.loaded.backups) {
          state.loaded.backups = true;
          loadBackups().catch(() => {});
        }
		if (state.view === "kiosk-display" && !state.loaded.telemetry) {
		  state.loaded.telemetry = true;
		  getJSON("/api/browser/telemetry").then((data) => { state.telemetry = data; if (state.view === "kiosk-display") renderApp(); }).catch(() => { state.loaded.telemetry = false; });
		}
      }

	  async function applyProfileRecommendation() {
		if (!confirm(t("confirmProfileRecommendation"))) return;
		await runAction("profile-recommendation-apply", async () => {
		  await postJSON("/api/browser/profile-recommendation", {});
		  await refreshCore();
		  renderApp();
		}, t("recommendationApplied"));
	  }

	  async function resetTelemetry() {
		if (!confirm(t("confirmTelemetryReset"))) return;
		await runAction("telemetry-reset", async () => {
		  await request("/api/browser/telemetry", { method: "DELETE" });
		  state.telemetry = await getJSON("/api/browser/telemetry");
		  await refreshCore();
		  renderApp();
		}, t("telemetryReset"));
	  }

      async function saveAdminSettings() {
        await runAction("admin-save", async () => {
          const cfg = cloneConfig();
          cfg.admin = cfg.admin || {};
          cfg.admin.bind = val("admin-bind");
          cfg.admin.port = Number(val("admin-port") || 33333);
          await postJSON("/api/config", cfg);
          await refreshCore();
          clearDirty("settings-admin");
          renderApp();
        }, t("saved"));
      }

      async function saveBrowserSettings(restart) {
        if (restart && !confirm(t("confirmRestart"))) return;
        await runAction(restart ? "browser-settings-save-restart" : "browser-settings-save", async () => {
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.performance = cfg.performance || {};
          cfg.watchdog = cfg.watchdog || {};
          cfg.kiosk.browser_preset = val("kiosk-browser-preset") || "chromium";
          cfg.kiosk.browser_command = val("kiosk-browser");
          cfg.kiosk.user_data_dir = val("kiosk-user-data");
          cfg.kiosk.isolate_page_sessions = checked("kiosk-isolate-sessions");
          cfg.kiosk.theme = val("kiosk-theme");
          cfg.kiosk.zoom_percent = Number(val("kiosk-zoom") || 125);
          cfg.kiosk.extra_args = lines("kiosk-extra-args");
          cfg.kiosk.widget = checked("kiosk-widget");
          cfg.performance.profile = val("perf-profile");
          cfg.performance.gpu_mode = val("perf-gpu");
          cfg.performance.reduce_motion = checked("perf-reduce");
          cfg.watchdog.enabled = checked("watchdog-enabled");
          cfg.watchdog.max_rss_mb = Number(val("watchdog-rss") || 900);
          cfg.watchdog.max_cpu_percent = Number(val("watchdog-cpu") || 300);
          cfg.watchdog.cpu_grace = durationToNs(val("watchdog-grace"));
          cfg.watchdog.check_interval = durationToNs(val("watchdog-interval"));
          await postJSON("/api/config", cfg);
          if (restart) await postJSON("/api/browser/restart");
          await refreshCore();
          state.config = cfg;
          clearDirty("kiosk-display");
          renderApp();
        }, t("saved"));
      }

      async function savePassword() {
        await runAction("password-save", async () => {
          await postJSON("/api/auth/password", { current: val("password-current"), next: val("password-next") });
        }, t("saved"));
      }

      async function importConfig(event) {
        const file = event.target.files?.[0];
        if (!file) return;
        await runAction("config-import", async () => {
          const text = await file.text();
          await request("/api/config/import", { method: "POST", body: text, headers: { "Content-Type": "application/json" } });
          await refreshCore();
          renderApp();
        }, t("saved"));
      }

      async function saveRawConfig() {
        await runAction("config-raw-save", async () => {
          const cfg = JSON.parse(val("config-raw"));
          await postJSON("/api/config", cfg);
          await refreshCore();
          clearDirty("settings-config");
          renderApp();
        }, t("saved"));
      }

      async function logoutAllSessions() {
        if (!confirm(t("confirmLogoutAll"))) return;
        await postJSON("/api/auth/logout-all");
        await boot();
      }

      async function loadBackups() {
        const result = await getJSON("/api/config/backups");
        state.backups = result.backups || [];
        if (state.view.startsWith("settings")) renderApp();
      }

      async function restoreBackup(path) {
        if (!confirm(t("confirmRestore"))) return;
        await runAction("restore", async () => {
          await postJSON("/api/config/restore", { path });
          await refreshCore();
          renderApp();
        }, t("saved"));
      }

      async function checkRepair() {
        await runAction("repair-check", async () => {
          state.repair = await getJSON("/api/repair");
          renderApp();
        }, t("diagnostics"));
      }

      async function runRepair() {
        await runAction("repair-run", async () => {
          state.repair = await postJSON("/api/repair", {});
          await refreshCore();
          renderApp();
        }, t("saved"));
      }

      async function checkUpdate() {
        await runAction("update-check", async () => {
          state.update = await getJSON("/api/update");
          renderApp();
        }, t("checkUpdate"));
      }

      function updateCredentials() {
        return {
          mode: val("update-priv-mode") || "sudo",
          password: val("update-priv-password"),
          remember: checked("update-priv-remember"),
        };
      }

      async function runUpdatePreflight() {
        await runAction("update-preflight", async () => {
          state.updatePreflight = await postJSON("/api/update/preflight", updateCredentials());
          const output = document.getElementById("update-preflight-result");
          if (output) output.innerHTML = renderUpdatePreflight();
          if (!state.updatePreflight.ok) throw new Error(t("updatePreflightFailed"));
        }, t("updatePreflightPassed"));
      }

      async function loadUpdateHistory() {
        state.updateHistory = await getJSON("/api/update/history");
        if (state.view === "settings-updates") renderApp();
      }

      async function installUpdate() {
        if (!confirm(t("confirmUpdate"))) return;
        const job = await runAction("update-install", async () => {
          const result = await postJSON("/api/update/install", {
            ...updateCredentials(),
          });
          state.updateJobs.unshift(result);
          state.update = { ...(state.update || {}), installing: true };
          return result;
        }, t("actionStarted"));
        if (job) {
          toast(t("actionStarted"), job.id || "", "ok");
          pollUpdateJob(job.id);
          renderApp();
        }
      }

      async function rollbackUpdate() {
        const target = state.updateHistory?.rollback_target || "";
        if (!target || !confirm(t("confirmRollback").replace("{version}", target))) return;
        const job = await runAction("update-rollback", async () => {
          const result = await postJSON("/api/update/rollback", updateCredentials());
          state.updateJobs.unshift(result);
          state.update = { ...(state.update || {}), installing: true };
          return result;
        }, t("actionStarted"));
        if (job) {
          pollUpdateJob(job.id);
          renderApp();
        }
      }

      async function pollUpdateJob(id) {
        if (!id) return;
        let lastJob = state.updateJobs.find((item) => item.id === id);
        for (let i = 0; i < 180; i++) {
          try {
            const job = await getJSON("/api/update/jobs/" + encodeURIComponent(id));
            lastJob = job;
            const index = state.updateJobs.findIndex((item) => item.id === id);
            if (index >= 0) state.updateJobs[index] = job;
            else state.updateJobs.unshift(job);
            if (state.view === "settings-updates") renderApp();
            if (job.finished || job.exit_code >= 0) {
              state.update = { ...(state.update || {}), installing: false };
              if (job.exit_code === 0) {
                toast(t("updateInstalled"), t("serviceRestarting"), "ok");
                setTimeout(() => window.location.reload(), 3500);
              } else {
                toast(t("updateFailed"), job.error || t("failed"), "error");
                if (state.view === "settings-updates") renderApp();
              }
              loadUpdateHistory().catch(() => {});
              return;
            }
          } catch (err) {
            if (lastJob?.stage === "restarting" || lastJob?.exit_code === 0) {
              toast(t("serviceRestarting"), t("reconnecting"), "warn");
              setTimeout(() => window.location.reload(), 4000);
              return;
            }
            toast(t("updateFailed"), err.message, "error");
            return;
          }
          await sleep(1000);
        }
      }

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
        if (!state.actionLog.length) return "";
        return `
          <div class="grid">
            <strong>${esc(t("recentActions"))}</strong>
            <div class="workflow-line">
              ${state.actionLog.slice(0, 4).map((item) => `<span class="chip ${item.type === "error" ? "bad" : item.type === "warn" ? "warn" : "ok"}">${esc(item.title)} - ${esc(new Date(item.at).toLocaleTimeString())}</span>`).join("")}
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
        return state.jobs.map((job) => `$ ${job.name} (${job.exit_code})\n${(job.output || []).join("\n")}`).join("\n\n");
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
        </div>`;
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
          if (!["mqtt:", "mqtts:", "ws:", "wss:"].includes(parsed.protocol)) throw new Error("protocol");
        } catch {
          validationError("mqtt-url", t("validationBrokerUrl"));
        }
        if (!String(mqtt.node || "").trim()) validationError("mqtt-node", t("validationMQTTNode"));
        if (!String(mqtt.base_topic || "").trim()) validationError("mqtt-base-topic", t("validationMQTTTopic"));
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
          display_options: { power_off_after: !!p.display_options?.power_off_after, screensaver: !!p.display_options?.screensaver, brightness: Number(p.display_options?.brightness ?? 100) },
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

      function field(id, label, type = "text", autocomplete = "", value = "", placeholder = "") {
        return `<div><label for="${esc(id)}">${esc(label)}</label><input id="${esc(id)}" type="${esc(type)}" ${autocomplete ? `autocomplete="${esc(autocomplete)}"` : ""} ${placeholder ? `placeholder="${esc(placeholder)}"` : ""} value="${esc(value)}" /></div>`;
      }

      function unsupportedControl(label) {
        return `<div class="unsupported-control"><label>${esc(label)}</label><span>${esc(t("notSupportedOnDevice"))}</span></div>`;
      }

      function textarea(id, label, value = "") {
        return `<div><label for="${esc(id)}">${esc(label)}</label><textarea id="${esc(id)}">${esc(value)}</textarea></div>`;
      }

      function selectHtml(id, label, value, options) {
        return `<div><label for="${esc(id)}">${esc(label)}</label><select id="${esc(id)}">${(options || [])
          .map(([v, text]) => `<option value="${esc(v)}" ${String(v) === String(value) ? "selected" : ""}>${esc(text)}</option>`)
          .join("")}</select></div>`;
      }

      function switchHtml(id, label, on) {
        return `<label class="switch" for="${esc(id)}"><span>${esc(label)}</span><input id="${esc(id)}" type="checkbox" ${on ? "checked" : ""} /></label>`;
      }

      function button(labelKey, action, cls = "") {
        const hint = t(action + "Hint");
        const title = hint === action + "Hint" ? t(labelKey) : hint;
        return `<button class="${esc(cls)}" title="${esc(title)}" data-busy="${esc(action)}" data-action="${esc(action)}">${esc(t(labelKey))}</button>`;
      }

      boot();
