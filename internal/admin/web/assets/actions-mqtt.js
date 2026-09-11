"use strict";

function bindMQTT() {
        ["mqtt-node", "mqtt-base-topic", "mqtt-discovery"].forEach((id) => document.getElementById(id)?.addEventListener("input", () => {
          const topic = document.getElementById("mqtt-topic");
          if (topic) topic.textContent = commandTopic();
        }));
        document.querySelectorAll('[data-action="mqtt-save"]').forEach((button) => button.addEventListener("click", saveMQTT));
        document.querySelectorAll('[data-action="mqtt-test"]').forEach((button) => button.addEventListener("click", testMQTT));
        document.querySelectorAll('[data-action="mqtt-discovery"]').forEach((button) => button.addEventListener("click", publishMQTTDiscovery));
        document.querySelectorAll('[data-action="mqtt-discovery-preview"]').forEach((button) => button.addEventListener("click", previewMQTTDiscovery));
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
			ca_file: val("mqtt-ca-file"),
			cert_file: val("mqtt-cert-file"),
			key_file: val("mqtt-key-file"),
			server_name: val("mqtt-server-name"),
			reject_unauthorized: checked("mqtt-reject-unauthorized"),
			maximum_packet_size: Number(val("mqtt-max-packet") || 1048576),
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
			  if (state.view === "mqtt") renderAppIfIdle();
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
			ca_file: val("mqtt-ca-file"),
			cert_file: val("mqtt-cert-file"),
			key_file: val("mqtt-key-file"),
			server_name: val("mqtt-server-name"),
			reject_unauthorized: checked("mqtt-reject-unauthorized"),
			maximum_packet_size: Number(val("mqtt-max-packet") || 1048576),
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
            }, controller.signal, { timeout: 35_000 });
            if (!result) throw new Error(t("mqttTestMissingResult"));
          } catch (err) {
            const message = err.code === "request_timeout" || err.name === "AbortError" ? t("mqttTestTimeout") : err.message;
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

      async function previewMQTTDiscovery() {
        const output = document.getElementById("mqtt-result");
        await runAction("mqtt-discovery-preview", async () => {
          const plan = await getJSON("/api/mqtt/discovery");
          if (output) output.textContent = [
            `${t("discoveryTotal")}: ${plan.total || 0}`,
            `${t("discoveryAdd")}: ${(plan.add || []).length}`,
            ...(plan.add || []).map((topic) => `  + ${topic}`),
            `${t("discoveryKeep")}: ${(plan.keep || []).length}`,
            `${t("discoveryRemove")}: ${(plan.remove || []).length}`,
            ...(plan.remove || []).map((topic) => `  - ${topic}`),
            `${t("discoveryUnsupported")}: ${(plan.unsupported || []).length}`,
          ].join("\n");
        }, t("previewDiscovery"));
      }

      async function resetMQTTDiscovery() {
        const output = document.getElementById("mqtt-result");
        await runAction("mqtt-discovery-reset", async () => {
          const result = await postJSON("/api/mqtt/discovery-reset", {});
          if (!result.ok) throw new Error(result.error || t("failed"));
          if (output) output.textContent = `${t("success")}: ${t("resetDiscovery")}\n${t("publishedTopics")}: ${result.cleared || 0}`;
        }, t("resetDiscovery"));
      }
