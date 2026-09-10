"use strict";

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
				${field("mqtt-ca-file", t("mqttCAFile"), "text", "", mqtt.ca_file || "")}
				${field("mqtt-cert-file", t("mqttCertFile"), "text", "", mqtt.cert_file || "")}
				${field("mqtt-key-file", t("mqttKeyFile"), "text", "", mqtt.key_file || "")}
				${field("mqtt-server-name", t("mqttServerName"), "text", "", mqtt.server_name || "")}
				${field("mqtt-max-packet", t("mqttMaximumPacketSize"), "number", "", mqtt.maximum_packet_size || 1048576)}
				<div>${switchHtml("mqtt-reject-unauthorized", t("mqttRejectUnauthorized"), mqtt.reject_unauthorized !== false)}</div>
              </div>
            </details>
            <div class="card result-panel">
              <div class="head"><h3>${esc(t("connectionProtocol"))}</h3><span class="section-kicker">${esc(t("lastTestResult"))}</span></div>
              <pre id="mqtt-result" class="logbox compact-log empty-log">${esc(t("mqttNoTestResult"))}</pre>
            </div>
            <div class="save-bar"><span data-dirty-indicator>${esc(t(isDirty() ? "unsavedChanges" : "allChangesSaved"))}</span><div class="actions">${button("testConnection", "mqtt-test")}${button("save", "mqtt-save", "primary")}</div></div>
          </div>`;
      }
