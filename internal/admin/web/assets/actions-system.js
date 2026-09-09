"use strict";

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
        document.querySelector('[data-action="priv-activate"]')?.addEventListener("click", async () => {
          await runAction("priv-activate", async () => {
            const password = val("priv-password");
            if (!password) throw new Error(t("passwordRequired"));
            state.privilege = await postJSON("/api/privilege", { mode: val("priv-mode") || "sudo", password });
            const passwordField = document.getElementById("priv-password");
            if (passwordField) passwordField.value = "";
            renderApp();
          }, t("privilegeActivated"));
        });
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
          const job = await postJSON("/api/system/" + name, { mode: val("priv-mode"), password: val("priv-password"), remember: true });
          state.jobs.unshift(job);
          try { state.privilege = await getJSON("/api/privilege"); } catch {}
          const passwordField = document.getElementById("priv-password");
          if (passwordField && val("priv-password")) passwordField.value = "";
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
            remember: true,
          });
          state.jobs.unshift(job);
          pollJob(job.id);
          await refreshCore();
          clearDirty("system-device");
          renderApp();
        }, t("saved"));
      }

      async function pollJob(id) {
        // apt-update/upgrade can take several minutes on Pi hardware; keep
        // polling until the job finishes instead of stopping after ~80s.
        for (let i = 0; i < 7200; i++) {
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
          const result = source === "events"
            ? await getJSON("/api/events?limit=" + encodeURIComponent(val("log-lines") || "300"))
            : await getJSON("/api/logs?source=" + encodeURIComponent(source) + "&lines=" + encodeURIComponent(val("log-lines") || "300"));
          state.logs = source === "events" ? formatEvents(result.events || []) : (result.lines || []);
          state.logSource = result.source || source;
          state.logWarning = result.warning || "";
          renderApp();
        }, t("refreshLogs"));
      }

      function formatEvents(events) {
        return events.slice().reverse().map((event) => {
          const at = event.at ? new Date(event.at).toLocaleString() : "-";
          const action = [event.component, event.action].filter(Boolean).join("/");
          const duration = event.duration_ms ? ` (${event.duration_ms} ms)` : "";
          const details = Object.entries(event.details || {}).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => `${key}=${value}`).join(", ");
          return `[${at}] ${action || "runtime"} ${event.status || "info"}${duration}: ${event.message || ""}${details ? ` [${details}]` : ""}`;
        });
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
