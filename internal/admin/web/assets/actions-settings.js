"use strict";

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
		  getJSON("/api/auth/sessions").then((data) => { state.sessions = data; if (state.view.startsWith("settings")) renderAppIfIdle(); }).catch(() => {});
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
		  getJSON("/api/browser/telemetry").then((data) => { state.telemetry = data; if (state.view === "kiosk-display") renderAppIfIdle(); }).catch(() => { state.loaded.telemetry = false; });
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
