"use strict";

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
				${statusTile(t("averageCpu"), formatValue(telemetry.cpu_average, "%"), telemetry.cpu_average >= 200 ? "warn" : "", `${t("maximum")}: ${formatValue(telemetry.cpu_maximum, "%")} · ${t("cpuCoresHint")}`)}
				${statusTile(t("averageMemory"), formatValue(telemetry.rss_average_mb, " MB"), telemetry.rss_average_mb >= 1200 ? "warn" : "", `${t("maximum")}: ${formatValue(telemetry.rss_maximum_mb, " MB")} · ${t("memoryPssHint")}`)}
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
