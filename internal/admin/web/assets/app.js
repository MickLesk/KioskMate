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
        diagnostics: null,
        repair: null,
        pageFilter: "",
        actionLog: [],
        logs: [],
        logSource: localStorage.getItem("kioskmate.logSource") || "combined",
        logWarning: "",
        ssh: null,
        terminal: "",
        jobs: [],
        loaded: {},
        busy: new Set(),
        dirtyViews: new Set(),
        snapshotURL: "",
        snapshotTime: "",
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
        const response = await fetch(path, {
          credentials: "same-origin",
          headers: { "Content-Type": "application/json", ...(options.headers || {}) },
          ...options,
        });
        const text = await response.text();
        let data = {};
        if (text) {
          try {
            data = JSON.parse(text);
          } catch {
            data = { error: text };
          }
        }
        if (!response.ok) {
          const error = new Error(data.error || response.statusText || "HTTP " + response.status);
          error.data = data;
          throw error;
        }
        return data;
      }

      const getJSON = (path) => request(path);
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
          await refreshCore();
          renderApp();
        } catch (err) {
          root.innerHTML = `<div class="login"><div class="login-card"><div class="body"><h2>${esc(t("failed"))}</h2><p class="muted">${esc(err.message)}</p></div></div></div>`;
        }
      }

      async function refreshCore() {
        const [cfg, status, privilege, timeInfo, zones, jobs] = await Promise.all([
          getJSON("/api/config"),
          getJSON("/api/status"),
          getJSON("/api/privilege"),
          getJSON("/api/time"),
          getJSON("/api/time/zones"),
          getJSON("/api/jobs?limit=25"),
        ]);
        state.config = cfg;
        state.persistedConfig = JSON.parse(JSON.stringify(cfg));
        state.status = status;
        state.hardware = status.hardware || {};
        state.privilege = privilege;
        state.time = timeInfo;
        state.timezones = zones.zones || [];
        state.jobs = jobs.jobs || [];
        syncThemeFromConfig();
      }

      async function refreshAndRender() {
        await runAction("refresh", async () => {
          await refreshCore();
          renderApp();
        }, t("refresh"));
      }

      function renderLogin() {
        const setup = !!state.auth?.setupRequired;
        root.innerHTML = `
          <main class="login">
            <form class="login-card" id="auth-form">
              <div class="head">
                <div class="login-brand">
                  <div class="mark">K</div>
                  <div>
                    <h1>${esc(setup ? t("setupTitle") : t("signInTitle"))}</h1>
                    <p class="muted">${esc(setup ? t("setupHint") : t("signInHint"))}</p>
                  </div>
                </div>
              </div>
              <div class="body grid">
                <input class="hidden" autocomplete="username" value="admin" />
                ${setup ? field("setup-token", t("setupToken"), "text", "one-time-code") : ""}
                ${field("auth-password", t("password"), "password", "current-password")}
                <button class="primary" data-busy="auth">${esc(setup ? t("createPassword") : t("signIn"))}</button>
                <p class="muted">${esc(t("setupInfo"))}</p>
                <div class="row">
                  ${selectHtml("login-lang", t("language"), state.lang, [["en", "English"], ["de", "Deutsch"]])}
                  ${selectHtml("login-theme", t("theme"), state.theme, [["dark", t("dark")], ["light", t("light")]])}
                </div>
              </div>
            </form>
          </main>`;
        document.getElementById("auth-form").addEventListener("submit", async (event) => {
          event.preventDefault();
          await runAction("auth", async () => {
            if (setup) await postJSON("/api/auth/setup", { token: val("setup-token"), password: val("auth-password") });
            else await postJSON("/api/auth/login", { password: val("auth-password") });
            await boot();
          }, t("success"));
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
            <aside class="sidebar">
              <div class="brand">
                <div class="mark">K</div>
                <div>
                  <h1>KioskMate</h1>
                  <p>${esc(t("appSubtitle"))}</p>
                </div>
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
                  <span class="chip ${browser.running ? "ok" : "bad"}" title="${esc(t("displayStatus"))}">${esc(browser.running ? t("running") : t("stopped"))}</span>
                  <span class="chip ${mqtt.connected ? "ok" : mqtt.state === "auth_error" || mqtt.state === "error" ? "bad" : ""}" title="${esc(mqtt.last_error || t("mqttService"))}">MQTT: ${esc(formatMQTTState(mqtt.state))}</span>
                  <span class="chip ${timeInfo.synchronized ? "ok" : "warn"}" title="${esc(timeInfo.synchronized ? t("timeSynchronized") : t("timeNotSynchronized"))}"><span id="kiosk-clock">${esc(formatClock(timeInfo.current_time))}</span></span>
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
          const target = children[0]?.id || item.id;
          const childHtml = children.length
            ? `<div class="nav-children">${children.map((child) => `<button class="${state.view === child.id ? "active" : ""}" data-view="${child.id}"><span>${esc(t(child.label))}</span><small>${esc(t(child.hint))}</small></button>`).join("")}</div>`
            : "";
          return `
            <div class="nav-group">
              <button class="nav-parent ${active ? "active" : ""}" data-view="${target}">
                <span>${esc(t(item.label))}</span>
                ${children.length ? `<small>${esc(t(item.hint))}</small>` : `<small>${esc(t(item.hint))}</small>`}
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
        document.querySelectorAll("[data-view]").forEach((button) => {
          button.addEventListener("click", () => {
            const nextView = button.dataset.view;
            if (nextView === state.view || !confirmDiscard()) return;
            state.view = nextView;
            localStorage.setItem("kioskmate.view", state.view);
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
        const cfg = state.config || {};
        const stats = browser.stats || {};
        const watchdog = browser.watchdog || {};
        const pages = normalizePages(cfg.kiosk?.pages, cfg.kiosk?.urls);
        const activeIndex = Number(browser.active || 0);
        const activePage = pages[activeIndex] || {};
        const watchdogReason = watchdog.last_reason || (browser.last_error === "signal: killed" ? t("watchdogKilledHint") : "");
        return `
          <div class="page-stack">
            <section class="status-strip" aria-label="${esc(t("status"))}">
              ${statusTile(t("displayStatus"), browser.running ? t("running") : t("stopped"), browser.running ? "ok" : "bad", browser.pid ? `PID ${browser.pid}` : t("noProcess"))}
              ${statusTile(t("currentPage"), browser.page_name || activePage.name || "-", "", `${activeIndex + 1} / ${Math.max(1, pages.length)}`)}
              ${statusTile(t("processorLoad"), formatValue(stats.cpu_percent, "%"), Number(stats.cpu_percent || 0) > 250 ? "warn" : "", formatValue(stats.rss_mb, " MB RAM"))}
              ${statusTile("MQTT", cfg.mqtt?.enabled ? t("enabled") : t("disabled"), cfg.mqtt?.enabled ? "ok" : "", cfg.mqtt?.version ? `MQTT ${cfg.mqtt.version}` : "-")}
            </section>
            <div class="dashboard-layout">
              <div class="control-stack">
                <div class="card">
                  <div class="head">
                    <div><h3>${esc(t("quickControl"))}</h3><span class="section-kicker">${esc(browser.page_name || activePage.name || t("noData"))}</span></div>
                    <span class="chip ${browser.running ? "ok" : "bad"}">${esc(browser.running ? t("running") : t("stopped"))}</span>
                  </div>
                  <div class="body command-panel">
                    <div class="command-primary">
                      ${browser.running ? button("stopBrowser", "browser-stop") : button("startBrowser", "browser-start", "primary")}
                      ${button("reloadBrowser", "browser-reload", browser.running ? "primary" : "")}
                      ${button("restartBrowser", "browser-restart")}
                    </div>
                    <div class="active-page-summary">
                      <div class="active-page-copy"><strong>${esc(activePage.name || browser.page_name || t("noData"))}</strong><span>${esc(activePage.url || browser.url || "-")}</span></div>
                      ${pages.filter((page) => !page.disabled && page.url).length > 1 ? `<div class="actions">${button("previousPage", "browser-previous")}${button("nextPage", "browser-next")}</div>` : ""}
                    </div>
                    <div id="page-check-output" class="action-feedback" aria-live="polite"></div>
                    <details class="disclosure">
                      <summary>${esc(t("troubleshooting"))}</summary>
                      <div class="disclosure-body action-matrix">
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
                    ${healthRow(t("watchdog"), watchdog.pressure || t("normal"), watchdog.pressure && watchdog.pressure !== "normal" ? "warn" : "ok")}
                    ${healthRow(t("lastError"), browser.last_error || t("none"), browser.last_error ? "bad" : "")}
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

      function renderKiosk() {
        const cfg = state.config || {};
        const kiosk = cfg.kiosk || {};
        const browser = state.status?.browser || {};
        const pages = normalizePages(kiosk.pages, kiosk.urls);
        const enabledPages = pages.filter((page) => !page.disabled && page.url).length;
        return `
          <div class="page-stack">
            <section class="section-toolbar">
              <div class="section-summary">
                <strong>${esc(t("kioskPages"))}</strong>
                <span>${esc(t("enabledPages"))}: ${enabledPages} / ${pages.length}</span>
              </div>
              <div class="actions">
                ${button("addPage", "page-add", "primary")}
                ${button("addHomeAssistant", "page-add-ha")}
              </div>
            </section>
            <div class="card">
              <div class="head">
                <div><h3>${esc(t("currentSelection"))}</h3><span class="section-kicker">${esc(browser.page_name || t("noData"))}</span></div>
                <div class="actions">
                  ${button("activatePage", "page-activate", "primary")}
                  ${button("checkPage", "page-check")}
                </div>
              </div>
              <div class="body">
                <div id="page-check-output" class="action-feedback">${esc(browser.url || "-")}</div>
              </div>
            </div>
            <section class="data-section">
              <div class="data-section-head">
                <div><h3>${esc(t("pages"))}</h3><span class="section-kicker">${esc(t("pageOrderHint"))}</span></div>
              </div>
              <div class="data-section-body">
                <input id="pages-import-file" type="file" accept="application/json,.json" class="hidden" />
                <div class="toolbar">
                  <input id="page-filter" data-no-dirty type="search" placeholder="${esc(t("filterPages"))}" value="${esc(state.pageFilter || "")}" />
                  <span id="visible-pages-count" class="chip">${esc(t("visiblePages"))}: ${pages.length}</span>
                </div>
                <div id="pages-list">${renderPages(pages)}</div>
                <div id="page-check-all-output" class="check-panel"></div>
                <details class="disclosure compact-disclosure">
                  <summary>${esc(t("advancedActions"))}</summary>
                  <div class="disclosure-body actions">
                    ${button("renderCheck", "render-check")}
                    ${button("openPreview", "preview-open")}
                    ${button("checkAllPages", "page-check-all")}
                    ${button("importPages", "pages-import")}
                    ${button("exportPages", "pages-export")}
                    ${button("enableAll", "page-enable-all")}
                    ${button("disableAll", "page-disable-all")}
                  </div>
                </details>
              </div>
            </section>
            <section class="data-section workflow-editor">
              <div class="data-section-head">
                <div><h3>${esc(t("workflow"))}</h3><span class="section-kicker">${esc(t("schedulerSaveHint"))}</span></div>
                ${switchHtml("scheduler-enabled", t("enabled"), !!kiosk.scheduler?.enabled)}
              </div>
              <div class="data-section-body grid">
                <div class="form-grid workflow-settings">
                  ${selectHtml("scheduler-mode", t("mode"), kiosk.scheduler?.mode || "rotation", [["rotation", t("rotationMode")], ["time", t("timeMode")], ["hybrid", t("mixedMode")]])}
                  ${field("scheduler-tick", t("tickInterval"), "number", "", secondsToDuration(kiosk.scheduler?.tick_interval, 15))}
                </div>
                <div class="schedule-columns">
                  <div>
                    <div class="subsection-head"><strong>${esc(t("rotation"))}</strong><div class="actions">${button("buildRotation", "rotation-build")}${button("addRotation", "rotation-add", "primary")}</div></div>
                    <div id="rotation-list">${renderRotation(kiosk.rotation || [])}</div>
                  </div>
                  <div>
                    <div class="subsection-head"><strong>${esc(t("timeRules"))}</strong><div class="actions">${button("addRule", "rule-add", "primary")}</div></div>
                    <div id="rules-list">${renderRules(kiosk.time_rules || [])}</div>
                  </div>
                </div>
              </div>
            </section>
            <div class="save-bar">
              <span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span>
              <div class="actions">
                ${button("save", "kiosk-save")}
                ${button("saveRestart", "kiosk-save-restart", "primary")}
              </div>
            </div>
          </div>`;
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
        return `
          <div class="page-stack">
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
                  ${field("mqtt-password", state.status?.config?.mqtt_password_configured ? `${t("password")} (${t("configured")})` : t("password"), "password", "current-password", mqtt.password || "")}
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
              <pre id="mqtt-result" class="logbox compact-log"></pre>
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
        return `
          <div class="page-stack">
            <section class="status-strip">
              ${statusTile(t("privilege"), privilege.configured ? t("configured") : t("notConfigured"), privilege.configured ? "ok" : "warn", privilege.mode || "sudo")}
              ${statusTile(t("jobs"), String(state.jobs.length), "", state.jobs.some((job) => !job.finished) ? t("jobRunning") : t("noActiveJobs"))}
              ${statusTile(t("repairCenter"), repairIssues.length ? `${repairIssues.length} ${t("issues")}` : t("ready"), repairIssues.length ? "warn" : "ok", state.repair?.changed ? t("repairChanged") : t("noRepairIssues"))}
            </section>
            <div class="settings-columns">
              <div class="card">
                <div class="head"><div><h3>${esc(t("privilege"))}</h3><span class="section-kicker">${esc(t("privilegeSession"))}</span></div><span class="chip ${privilege.configured ? "ok" : ""}">${esc(privilege.configured ? t("configured") : t("notConfigured"))}</span></div>
                <div class="body form-grid">
                  ${selectHtml("priv-mode", t("privilegeMode"), privilege.mode || "sudo", [["sudo", "sudo"], ["su", "su / root"]])}
                  ${field("priv-password", t("password"), "password", "current-password", "")}
                  <div class="span-2">${switchHtml("priv-remember", t("rememberPassword"), false)}</div>
                  <button data-action="priv-clear" class="span-2">${esc(t("clearPrivilege"))}</button>
                </div>
              </div>
              <div class="card">
                <div class="head"><div><h3>${esc(t("systemActions"))}</h3><span class="section-kicker">${esc(t("privilegedActions"))}</span></div></div>
                <div class="body action-list">
                  <div><span>${esc(t("packageMaintenance"))}</span><div class="actions">${button("aptUpdate", "sys-apt-update")}${button("aptUpgrade", "sys-apt-upgrade")}</div></div>
                  <div><span>${esc(t("service"))}</span><div class="actions">${button("restartService", "sys-restart-service", "primary")}</div></div>
                  <div class="danger-zone"><span>${esc(t("power"))}</span><div class="actions">${button("reboot", "sys-reboot", "danger-ghost")}${button("shutdown", "sys-shutdown", "danger")}</div></div>
                </div>
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
        return `
          <div class="page-stack">
            <section class="section-toolbar log-toolbar">
              <div class="log-filters">
                ${selectHtml("log-source", t("logSource"), state.logSource, sources)}
                ${field("log-lines", t("logLines"), "number", "", 300)}
              </div>
              <div class="actions">
                <button class="primary" data-busy="logs-refresh" data-action="logs-refresh">${esc(t("refreshLogs"))}</button>
                <button data-action="logs-download">${esc(t("downloadLogs"))}</button>
                <button data-action="diagnostics-download">${esc(t("diagnosticBundle"))}</button>
              </div>
            </section>
            ${state.logWarning ? `<div class="notice warn">${esc(state.logWarning)}</div>` : ""}
            <div class="card">
              <div class="head"><div><h3>${esc(t("logs"))}</h3><span class="section-kicker">${esc(t("logSource"))}: ${esc(t(`log${state.logSource.charAt(0).toUpperCase()}${state.logSource.slice(1)}`))}</span></div></div>
              <pre class="logbox log-output">${esc((state.logs || []).join("\n"))}</pre>
            </div>
          </div>`;
      }

      function renderSettingsAdmin() {
        const cfg = state.config || {};
        return `
          <div class="grid">
            <div class="grid two">
              <div class="card">
                <div class="head"><h3>${esc(t("adminSettings"))}</h3><button class="primary" data-action="admin-save">${esc(t("save"))}</button></div>
                <div class="body form-grid">
                  ${field("admin-bind", t("bindAddress"), "text", "", cfg.admin?.bind || "0.0.0.0")}
                  ${field("admin-port", t("port"), "number", "", cfg.admin?.port || 33333)}
                </div>
              </div>
              <div class="card">
                <div class="head"><h3>${esc(t("changePassword"))}</h3><button data-action="password-save">${esc(t("save"))}</button></div>
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
                </div>
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
        return `
          <div class="page-stack">
            <section class="status-strip">
              ${statusTile(t("installed"), state.update?.current_version || state.update?.current || state.auth?.version || "-", "ok", t("currentVersion"))}
              ${statusTile(t("latest"), state.update?.latest_version || state.update?.latest || t("notChecked"), state.update?.update_available ? "warn" : "", state.update?.update_available ? t("updateAvailable") : t("upToDate"))}
            </section>
            <div class="card">
              <div class="head"><div><h3>${esc(t("update"))}</h3><span class="section-kicker">${esc(t("releaseChannel"))}: ${esc(state.update?.channel || "stable")}</span></div><div class="actions">${button("checkUpdate", "update-check")}${button("installUpdate", "update-install", "primary")}</div></div>
              <div class="body update-layout">
                <div>
                  <label>${esc(t("changelog"))}</label>
                  <pre class="jsonbox">${esc(state.update?.changelog || "")}</pre>
                </div>
                <div>
                  <label>${esc(t("jobOutput"))}</label>
                  <pre class="terminal">${esc(renderJobs())}</pre>
                </div>
              </div>
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
          document.querySelector(`[data-action="${key}"]`)?.addEventListener("click", () => browserAction(action, key));
        }
        document.querySelector('[data-action="browser-repair-ha"]')?.addEventListener("click", repairHASession);
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

      function bindKiosk() {
        bindBrowserButtons();
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
        document.querySelector('[data-action="preview-open"]')?.addEventListener("click", openPreview);
        document.getElementById("page-filter")?.addEventListener("input", (event) => {
          state.pageFilter = event.target.value || "";
          applyPageFilter(state.pageFilter);
        });
        document.querySelectorAll("[data-page-remove]").forEach((button) => {
          button.addEventListener("click", () => {
            const cfg = cloneConfig();
            cfg.kiosk.pages = collectPages();
            removePage(cfg.kiosk, Number(button.dataset.pageRemove));
            state.config = cfg;
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
            state.config = cfg;
            markDirty();
            renderApp();
          });
        });
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

      async function saveKiosk(restart) {
        if (restart && !confirm(t("confirmRestart"))) return;
        await runAction(restart ? "kiosk-save-restart" : "kiosk-save", async () => {
          const cfg = cloneConfig();
          cfg.kiosk = cfg.kiosk || {};
          cfg.kiosk.pages = collectPages();
          validatePages(cfg.kiosk.pages);
          cfg.kiosk.urls = cfg.kiosk.pages.filter((p) => !p.disabled).map((p) => p.url);
          cfg.kiosk.scheduler = { enabled: checked("scheduler-enabled"), mode: val("scheduler-mode") || "rotation", tick_interval: durationToNs(val("scheduler-tick") || 15) };
          cfg.kiosk.rotation = collectRotation();
          cfg.kiosk.time_rules = collectRules();
          validateScheduler(cfg.kiosk);
          await postJSON("/api/config", cfg);
          if (restart) await postJSON("/api/browser/restart");
          await refreshCore();
          state.config = cfg;
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
          cfg.kiosk.pages = pages.map((page, index) => ({
            name: String(page.name || `Kiosk ${index + 1}`),
            url: String(page.url || ""),
            disabled: !!page.disabled,
          })).filter((page) => page.name || page.url);
          syncKioskURLs(cfg.kiosk);
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
        const active = Number(document.querySelector("[data-page-active]:checked")?.value || 0);
        await runAction("page-activate", async () => {
          await postJSON("/api/browser/page", { index: active });
          await refreshCore();
          renderApp();
        });
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
        document.querySelector('[data-action="mqtt-save"]')?.addEventListener("click", saveMQTT);
        document.querySelector('[data-action="mqtt-test"]')?.addEventListener("click", testMQTT);
        document.querySelector('[data-action="mqtt-discovery"]')?.addEventListener("click", publishMQTTDiscovery);
        document.querySelector('[data-action="mqtt-discovery-reset"]')?.addEventListener("click", resetMQTTDiscovery);
      }

      async function saveMQTT() {
        await runAction("mqtt-save", async () => {
          const cfg = cloneConfig();
          cfg.mqtt = cfg.mqtt || {};
          Object.assign(cfg.mqtt, {
            enabled: checked("mqtt-enabled"),
            url: val("mqtt-url"),
            version: val("mqtt-version"),
            username: val("mqtt-user"),
            password: val("mqtt-password"),
            discovery: val("mqtt-discovery") || "homeassistant",
            base_topic: val("mqtt-base-topic") || "kioskmate",
            node: val("mqtt-node") || "kioskmate",
            client_id: val("mqtt-client-id"),
            keepalive: durationToNs(val("mqtt-keepalive") || 60),
            force_disable_retain: checked("mqtt-disable-retain"),
            interval: durationToNs(val("mqtt-interval")),
          });
          validateMQTT(cfg.mqtt);
          await postJSON("/api/config", cfg);
          await refreshCore();
          clearDirty("mqtt");
          renderApp();
        }, t("saved"));
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
          document.querySelector(`[data-action="${action}"]`)?.addEventListener("click", () => startSystemJob(action, name));
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
            if (job.finished) return;
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

      function bindSettings() {
        document.querySelector('[data-action="admin-save"]')?.addEventListener("click", saveAdminSettings);
        document.querySelector('[data-action="password-save"]')?.addEventListener("click", savePassword);
        document.querySelector('[data-action="browser-settings-save"]')?.addEventListener("click", () => saveBrowserSettings(false));
        document.querySelector('[data-action="browser-settings-save-restart"]')?.addEventListener("click", () => saveBrowserSettings(true));
        document.querySelector('[data-action="safe-mode"]')?.addEventListener("click", applySafeMode);
        document.querySelector('[data-action="browser-diagnostics"]')?.addEventListener("click", loadBrowserDiagnostics);
        document.querySelector('[data-action="config-export"]')?.addEventListener("click", () => { window.location.href = "/api/config/export"; });
        document.querySelector('[data-action="config-import"]')?.addEventListener("click", () => document.getElementById("config-import-file").click());
        document.getElementById("config-import-file")?.addEventListener("change", importConfig);
        document.querySelector('[data-action="config-raw-save"]')?.addEventListener("click", saveRawConfig);
        document.querySelector('[data-action="sessions-logout-all"]')?.addEventListener("click", logoutAllSessions);
        document.querySelector('[data-action="backups-refresh"]')?.addEventListener("click", loadBackups);
        document.querySelector('[data-action="update-check"]')?.addEventListener("click", checkUpdate);
        document.querySelector('[data-action="update-install"]')?.addEventListener("click", installUpdate);
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

      async function installUpdate() {
        if (!confirm(t("confirmUpdate"))) return;
        await runAction("update-install", async () => {
          const job = await postJSON("/api/update/install");
          state.jobs.unshift(job);
          toast(t("actionStarted"), job.id || "", "ok");
          pollUpdateJob(job.id);
          renderApp();
        }, t("actionStarted"));
      }

      async function pollUpdateJob(id) {
        if (!id) return;
        for (let i = 0; i < 180; i++) {
          const job = await getJSON("/api/update/jobs/" + encodeURIComponent(id));
          const index = state.jobs.findIndex((item) => item.id === id);
          if (index >= 0) state.jobs[index] = job;
          else state.jobs.unshift(job);
          if (state.view === "settings-updates") renderApp();
          if (job.finished || job.exit_code >= 0) return;
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
        if (!state.jobs.length) return `<div class="empty">${esc(t("noData"))}</div>`;
        return state.jobs.map((job) => {
          const running = !job.finished;
          const duration = Math.max(0, Math.round(((job.finished ? new Date(job.finished) : new Date()) - new Date(job.started)) / 1000));
          return `<article class="job-item">
            <div class="job-head"><div><strong>${esc(job.name || "-")}</strong><span>${esc(formatDate(job.started))} · ${esc(formatDuration(duration))}</span></div><span class="chip ${running ? "warn" : job.exit_code === 0 ? "ok" : "bad"}">${esc(running ? t("jobRunning") : job.exit_code === 0 ? t("success") : t("failed"))}</span></div>
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
        if ((mode === "rotation" && !rotation.length) || (mode === "time" && !activeRules.length) || (mode === "hybrid" && !rotation.length && !activeRules.length)) {
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
        if (Array.isArray(pages) && pages.length) return pages.map((p, i) => ({ name: p.name || `Page ${i + 1}`, url: p.url || "", disabled: !!p.disabled }));
        return (urls || []).map((url, i) => ({ name: `Page ${i + 1}`, url, disabled: false }));
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

      function field(id, label, type = "text", autocomplete = "", value = "") {
        return `<div><label for="${esc(id)}">${esc(label)}</label><input id="${esc(id)}" type="${esc(type)}" ${autocomplete ? `autocomplete="${esc(autocomplete)}"` : ""} value="${esc(value)}" /></div>`;
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
