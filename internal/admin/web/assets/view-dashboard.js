"use strict";

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
		const lastExit = browser.last_exit_details || {};
        const pages = normalizePages(cfg.kiosk?.pages, cfg.kiosk?.urls);
        const activeIndex = Number(browser.active || 0);
        const activePage = pages[activeIndex] || {};
        const watchdogReason = watchdog.last_reason || (browser.last_error === "signal: killed" ? t("watchdogKilledHint") : "");
        const enabledPages = pages.filter((page) => !page.disabled && page.url);
        const browserMessage = !browserKnown
          ? t("loading")
          : (browser.ready
          ? `${t("displayRunningHint")} ${browser.page_name || activePage.name || ""}`.trim()
          : (browser.running
            ? t("displayConnectingHint")
            : (browser.last_error ? `${t("displayStoppedErrorHint")}: ${browser.last_error}` : t("displayStoppedHint"))));
        const runningLabel = !browserKnown ? t("loading") : (browser.ready ? t("running") : browser.running ? t("connecting") : t("stopped"));
        const runningTone = !browserKnown ? "" : (browser.ready ? "ok" : browser.running ? "warn" : "bad");
        const schedulerReasonKey = String(browser.scheduler?.reason || "").toLowerCase();
        const hasTimeRules = (cfg.kiosk?.time_rules || []).length > 0;
        const schedulerNeedsAttention = schedulerReasonKey === "no active time rule" || (schedulerReasonKey === "disabled" && hasTimeRules);
        return `
          <div class="page-stack">
            ${renderUpdateNotice()}
			${recovery.state && !["healthy", "idle"].includes(recovery.state) ? stateBanner(recovery.state === "failed" || recovery.state === "auth_blocked" ? "bad" : "warn", t("recoveryNeedsAttention"), recovery.last_result || recovery.reason || recovery.state, button("recoverNow", "browser-auto-recover", "primary")) : ""}
            ${schedulerNeedsAttention ? stateBanner("warn", t("scheduler"), schedulerReasonKey === "disabled" ? t("schedulerDisabledWithRulesHint") : t("schedulerNoActiveRuleHint"), `<button data-view="kiosk-pages">${esc(t("manageFlow"))}</button>`) : ""}
            ${stateBanner(browserKnown ? (browser.ready ? "ok" : browser.running ? "warn" : "bad") : "warn", browserKnown ? (browser.ready ? t("displayReady") : browser.running ? t("displayConnecting") : t("displayNeedsAttention")) : t("loading"), browserMessage, browser.running
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
                      <div class="active-page-copy"><strong>${esc(activePage.name || browser.page_name || t("noData"))}</strong><span>${esc(activePage.url || browser.url || "-")}</span><small>${esc(browser.scheduler?.next_switch ? `${t("nextSwitch")}: ${formatDate(browser.scheduler.next_switch)}` : (browser.scheduler?.reason ? formatSchedulerReason(browser.scheduler.reason) : t("manualPageControl")))}</small></div>
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
                    ${browser.control?.failures ? healthRow(t("browserControlFailures"), `${browser.control.failures}: ${browser.control.last_error || "-"}`, "warn") : ""}
					${healthRow(t("browserGeneration"), String(browser.generation || 0), "")}
					${lastExit.reason ? healthRow(t("lastExitReason"), String(lastExit.reason).replaceAll("_", " "), lastExit.expected ? "" : "bad") : ""}
					${lastExit.at ? healthRow(t("lastBrowserRuntime"), `${formatDuration(Math.round(Number(lastExit.runtime_ms || 0) / 1000))} · ${t("exitCode")} ${lastExit.exit_code ?? "-"}`, lastExit.expected ? "" : "warn") : ""}
                    ${healthRow(t("haThemeSync"), formatThemeStatus(browser.theme_status), browser.theme_status?.state === "applied" ? "ok" : browser.theme_status?.state === "failed" ? "bad" : "")}
                    ${healthRow(t("authGuard"), browser.auth_guard?.tripped ? `${t("blocked")}: ${browser.auth_guard.reason || "-"}` : t("ready"), browser.auth_guard?.tripped ? "bad" : "ok")}
					${browser.auth_guard?.tripped ? healthRow(t("authEvidence"), `${browser.auth_guard.confidence || "-"} · ${browser.auth_guard.occurrences || 1} ${t("signals")}`, browser.auth_guard.confidence === "confirmed" ? "bad" : "warn") : ""}
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
