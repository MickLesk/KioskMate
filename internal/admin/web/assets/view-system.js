"use strict";

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

      function privilegeRemainingLabel(privilege) {
        const seconds = Number(privilege?.remaining_seconds || 0);
        if (!privilege?.configured || seconds <= 0) return privilege?.mode || "sudo";
        const minutes = Math.max(1, Math.ceil(seconds / 60));
        return `${privilege.mode || "sudo"} · ${minutes} ${t("minutesLeft")}`;
      }

      function renderSystemActions() {
        const privilege = state.privilege || {};
        const repairIssues = (state.repair?.issues || []).filter((issue) => issue.id !== "ok");
        const activeJobs = state.jobs.filter((job) => !job.finished).length;
        return `
          <div class="page-stack">
            ${stateBanner(privilege.configured ? "ok" : "warn", privilege.configured ? t("maintenanceReady") : t("privilegeRequired"), privilege.configured ? t("maintenanceReadyHint") : t("privilegeRequiredHint"))}
            <section class="status-strip">
              ${statusTile(t("privilege"), privilege.configured ? t("sessionActive") : t("notConfigured"), privilege.configured ? "ok" : "warn", privilegeRemainingLabel(privilege))}
              ${statusTile(t("jobs"), String(activeJobs), activeJobs ? "warn" : "ok", activeJobs ? t("jobRunning") : t("noActiveJobs"))}
              ${statusTile(t("repairCenter"), repairIssues.length ? `${repairIssues.length} ${t("issues")}` : t("ready"), repairIssues.length ? "warn" : "ok", state.repair?.changed ? t("repairChanged") : t("noRepairIssues"))}
            </section>
            <div class="settings-columns">
              <div class="card">
                <div class="head"><div><h3>${esc(t("privilege"))}</h3><span class="section-kicker">${esc(t("privilegeSession"))}</span></div><span class="chip ${privilege.configured ? "ok" : ""}">${esc(privilege.configured ? t("sessionActive") : t("notConfigured"))}</span></div>
                <div class="body form-grid">
                  ${selectHtml("priv-mode", t("privilegeMode"), privilege.mode || "sudo", [["sudo", "sudo"], ["su", "su / root"]])}
                  ${field("priv-password", t("password"), "password", "current-password", "")}
                  <p class="hint span-2">${esc(t("privilegeStorageHint"))}</p>
                  <div class="actions span-2">
                    <button data-action="priv-activate" class="primary">${esc(t("activatePrivilege"))}</button>
                    ${privilege.configured ? `<button data-action="priv-clear">${esc(t("clearPrivilege"))}</button>` : ""}
                  </div>
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
          ["events", t("logEvents")],
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
