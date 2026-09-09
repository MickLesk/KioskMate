# KioskMate next major version plan

Status: proposal based on the repository at `main`, current head `89ba3c1`, version `0.7.7` in the changelog.

This plan is intentionally implementation-oriented. It is sized for approximately 12-24 hours of focused agent work for the first production-quality milestone. A real Raspberry Pi soak test is an external dependency: it needs to run on the target hardware and cannot be replaced by a local build.

## Release strategy

The next release should be a stability-focused `0.8.0`, followed by a short release candidate soak. It should not be called `1.0.0` until the browser lifecycle and Home Assistant authorization paths have survived a target-device soak without recurring restart loops or new bans.

The major version theme is **Reliable kiosk operations**:

- The browser is a supervised state machine, not a collection of independent buttons.
- Home Assistant authentication is treated as a separate failure domain from page loading, images, MQTT and the Admin UI.
- The Admin UI exposes one clear task per screen and always shows an actionable result.
- Configuration is migrated without losing pages, credentials, sessions or user choices.
- Performance is measured as a browser/content problem and a KioskMate process problem separately.

## What the current code tells us

These are code-backed risks, not claims about the exact cause of every historical IP ban.

| Area | Current finding | Consequence |
| --- | --- | --- |
| Startup | `cmd/kioskmate/main.go` starts Chromium synchronously before the Admin listener is ready. | A slow browser start can make login, diagnosis and recovery appear unavailable. |
| DevTools | `internal/supervisor/cdp.go` uses synchronous reads per command and separate monitor connections. Unrelated events can be discarded, and the monitor exits on a connection error. | Initial auth events can be missed; theme and network monitoring are not reliably long-lived. |
| HA preflight | `internal/supervisor/ha_auth.go` follows same-host redirects and checks only a generic `403` result. | A 403 is not sufficient proof of an IP ban; protected resources and redirects need classification. |
| HA guard | `internal/supervisor/browser.go` stops the browser after a broad guard classification. Automatic recovery timers are not tied to a lifecycle generation. | A manual stop or a new session can race with an old recovery timer. |
| Browser profiles | Isolated paths are derived from page index/name rather than a stable page ID. | Renaming or reordering a page can silently switch the authenticated profile. |
| Runtime metrics | The project correctly moved to Linux PSS, but browser CPU remains content-dependent and is collected from a process tree. | UI must label CPU as process-tree/core percentage and avoid treating a busy camera dashboard as a KioskMate fault. |
| Config concurrency | `Config.Snapshot` is a shallow copy. Slices and maps remain shared while the configuration is edited by workers. | Scheduler, MQTT and Admin writes can race or observe partially changed collections. |
| Admin frontend | `app.js` is still a large global script and calls `root.innerHTML` for broad rerenders. Background refresh can replace active form state. | Focus, unsaved input and local feedback can disappear during polling. |
| Auth persistence | Sessions are persisted in `sessions.json` as bearer session IDs. | A readable config directory can become an Admin account takeover. |
| Login throttling | Failed attempts are keyed by the reported remote address and limited in memory. | Proxies and repeated local retries can create confusing lockouts; the response needs `Retry-After` and visible state. |
| Packaging | Both package builders contain a `postinst` migration that rewrites Admin bind addresses with `sed`. | Installing an update can change a security-sensitive setting without an explicit user action. |
| Packaging QA | The previous Debian failure was a malformed control description without a final newline. | Packages need metadata linting and installation tests before a GitHub release is published. |
| Logs | Own file logging exists, while `journalctl --user` can legitimately be empty on systems without persistent user journaling. | The UI needs source status and must not display an empty journal as if the application had no logs. |

The repeated IP-ban issue is therefore handled as a controlled investigation: capture HA logs and KioskMate event history, stop avoidable request storms, classify responses correctly, and only then decide whether an HA-side configuration or account change is needed. KioskMate must not silently delete `ip_bans.yaml` or disable HA protection.

## P0: reliability and safety

### 1. Replace browser control with one lifecycle state machine

Introduce explicit states:

`disabled -> starting -> running -> reloading -> stopping -> stopped -> failed -> recovering -> auth_blocked`

Requirements:

- Serialize start, stop, reload, restart, reset-session and page switch operations.
- Give every operation an ID, start time, stage, result and correlation ID.
- Return the operation ID immediately from Admin and MQTT commands; stream progress separately.
- Make manual stop authoritative. A stale watchdog/recovery callback must not start a later browser.
- Use a lifecycle generation/context that invalidates all old timers and monitor goroutines.
- Keep `starting`, `stopping`, `unknown` and `auth_blocked` distinct from `stopped`.
- Start the Admin server and its health endpoint before attempting Chromium. Browser startup failure must not remove the recovery UI.
- Always stop with a finite deadline and wait for the process group. Do not use `context.Background()` for shutdown.
- Ensure only one KioskMate instance and one configured browser profile can be active.

Acceptance criteria:

- Double-clicking any browser action creates one operation, not parallel Chromium processes.
- A stop followed immediately by an automatic-recovery deadline leaves the browser stopped.
- A failed start shows the exact stage and exit information in Dashboard, Logs and the API.
- Admin remains reachable while Chromium is unavailable.
- Repeated start/stop/restart actions do not increase the browser restart counter unless a real process was restarted.

### 2. Make Home Assistant authorization monitoring deterministic

Implement a single CDP connection/session manager with:

- One reader goroutine and a response dispatcher keyed by command ID.
- A bounded event channel for network, WebSocket and runtime events.
- Explicit attach/reconnect state and a monitor heartbeat.
- Target selection by page target ID and origin, not only the first available target.
- Navigation sequence: launch Chromium on a blank local document, attach DevTools and enable Network/Runtime, then navigate to the configured page.
- Safe teardown when the browser exits or the target changes.

Parse network data as structured JSON. Classify independently:

- `auth_invalid` WebSocket frame: authenticated HA session rejected.
- `/auth/token` `400 invalid_grant`: stale or revoked OAuth grant, not automatically an IP ban.
- API/WebSocket `401`: authentication required or expired.
- Relevant HA API/auth/WebSocket `403`: possible ban or account policy rejection; require corroborating evidence before calling it an IP ban.
- Image, camera, font, addon and arbitrary page-resource failures: page/resource error, never an authentication ban by themselves.
- Connection refusal, DNS and timeout: transport failure, not an auth failure.

The guard should expose `kind`, `confidence`, `origin`, `resource`, `status`, `first_seen`, `last_seen` and the next safe action. It should block retries only for confirmed auth failures and high-confidence repeated failures. It should never submit a password or retry a failed login in a loop.

Use Home Assistant's documented ban behavior as an external diagnostic boundary: an entry in `ip_bans.yaml` is cleared on the HA host and HA must be restarted afterwards. KioskMate can guide the user, show the detected kiosk IP and pause requests; it must not edit that file without an explicit, separately designed HA integration.

Acceptance criteria:

- A page containing a broken camera token does not stop the kiosk or trip the HA guard.
- A real `auth_invalid` event is captured even if it happens during initial navigation.
- A transient network outage enters backoff without creating a login storm.
- A confirmed guard state produces one clear recovery action: preserve session, reset session, or inspect HA ban.
- Resetting a session creates a backup, invalidates old monitors and starts one clean generation.

### 3. Remove restart-loop causes

- Add a restart budget per rolling hour and a persisted reason histogram.
- Require a stable-runtime window before clearing the failure counter.
- Distinguish watchdog kill, browser self-exit, manual restart, page switch and service restart.
- Never restart on CPU alone by default. Use sustained memory pressure, process death or explicit policy.
- Make browser health checks opt-in or low-frequency, use a safe unauthenticated endpoint, cap redirects, strip credentials/query fragments and apply exponential backoff.
- Do not run page health checks while the auth guard is active.
- Add a circuit breaker after repeated failures with a visible countdown and a manual resume action.

## P1: clean architecture and Admin UX

### 4. Split the Admin frontend into real modules

Keep the embedded, local-first delivery model, but remove the single global script. Use a small development build with no runtime framework dependency:

```text
web/
  index.html
  styles/
    tokens.css
    layout.css
    components.css
    states.css
  app/
    bootstrap.js
    router.js
    store.js
    api.js
    events.js
    i18n.js
    components/
      button.js
      toast.js
      modal.js
      status.js
      job.js
      form.js
    views/
      dashboard.js
      kiosk.js
      mqtt.js
      system.js
      settings.js
      login.js
```

The build should produce versioned, minified assets that Go embeds. No CDN and no network dependency at runtime. Use DOM-owned view roots and targeted updates; a background refresh must not replace a focused input or a dirty form.

Add a typed state model for:

- authentication/bootstrap
- browser lifecycle
- workflow draft versus persisted workflow
- MQTT connection/test
- maintenance jobs
- update state
- logs and diagnostics

Every request has a timeout, cancellation and an error code. Every mutating action returns a structured result with `operation_id`, `status`, `message`, `next_action` and optional job URL.

### 5. Redesign around five user tasks

The navigation should be:

- **Dashboard**: current display, live preview/snapshot, one primary browser action, current page, next transition, MQTT status and update notice.
- **Kiosk**: one all-in-one Page & Workflow workspace. Storybook is the default; Flow is an advanced view. Page creation/editing uses the wizard. Display appearance and performance live in a collapsible “Display behavior” section on the same page.
- **MQTT**: broker, protocol version, TLS, credentials, discovery, test dialog and entity preview. Keep it as its own menu item.
- **System**: Device & time, Maintenance, Logs and optional Terminal as subsections. The Terminal is hidden unless explicitly enabled and is a real streaming session, not a one-shot command box.
- **Settings**: Admin access, config/backups and Updates. Security-sensitive settings show scope and restart requirements beside the control.

Remove or rename confusing commands:

- “Previous/Next Kiosk Page” becomes “Switch page” and is hidden when fewer than two enabled pages exist.
- “Current page reload” becomes “Reload kiosk page”.
- “Open selected page” becomes “Open preview in new tab” and must report popup-blocked or navigation errors.
- “Restart browser” and “Restart service” are separate actions with separate confirmation text.

Dashboard feedback requirements:

- Toast for completion/failure, inline operation state for long actions, and persistent banner for blocked conditions.
- Disable only the action currently in progress.
- Show “why unavailable” in the control itself.
- Hover/focus help uses a tooltip or adjacent help text, never a hidden-only explanation.
- Display last action, timestamp, duration, exit code and next action.
- Keep a compact event timeline across page refreshes.

### 6. Make the Kiosk page manager the primary workflow

Use one canonical sequence model:

```json
{
  "pages": [
    {
      "page_id": "stable-id",
      "name": "Weather",
      "url": "http://homeassistant.local:8123/dashboard/weather",
      "duration_seconds": 60,
      "schedule": {"days": ["mon", "tue"], "start": "08:00", "end": "18:00"},
      "trigger": {"topic": "", "payload": ""},
      "display": {"power_off_after": false, "brightness": 80}
    }
  ],
  "workflow": {"mode": "rotation", "enabled": true, "tick_seconds": 5}
}
```

Implement:

- Drag-and-drop order with keyboard reorder fallback.
- Stable IDs used for profile directories and MQTT entity IDs.
- Page card status: configured, disabled, unreachable, auth required, active and next.
- Wizard validation before advancing and a review screen before saving.
- Fixed schedules, duration rotation, MQTT triggers and manual override in one model.
- Conflict detection for overlapping schedules and duplicate triggers.
- Preview without starting a second Chromium instance: use the existing signed-in target snapshot when possible, otherwise make a clearly labeled unauthenticated render check.
- “Save and apply” that reports whether a reload, page switch or full restart was necessary.

### 7. i18n and accessibility as build gates

- Use a typed translation key registry and fail the build on missing, unused or duplicate keys.
- Keep German and English complete for every visible state, not only labels.
- Interpolate values safely; never construct visible error text in English inside Go or JavaScript without a translation key.
- Add keyboard focus management for modals, escape handling, focus restoration and visible focus rings.
- Use semantic headings, labels, live regions and status roles.
- Test narrow Raspberry display dimensions and large desktop dimensions.

## P1: MQTT and Home Assistant integration

### 8. Make MQTT diagnostics and discovery first-class

The current client supports MQTT 3.1.1 and 5.0. “MQTT 7” is not a protocol version; the UI should explain the supported protocol choices as MQTT 3.1.1 or MQTT 5.0, with broker error details shown directly.

Add:

- TLS CA, client certificate/key, SNI/server name, reject-unauthorized, client ID, keepalive, maximum packet size and retain policy.
- MQTT 5 reason code and property decoding, including authorization, bad username/password, server unavailable and protocol errors.
- A live test dialog with ordered events: validate, resolve, connect, CONNACK, publish, subscribe, receive, disconnect, result.
- A test correlation ID and a timeout reason; never leave a modal in `running` after the request has ended.
- Last connection, last discovery publish, last error and current backoff in the header and Dashboard.
- A discovery preview showing exactly which topics will be retained.

Keep one Home Assistant device with stable identifiers and publish only capabilities that exist. Do not publish `null` as a usable sensor state. Per-page entities should include activation, active state, order, health, last error and URL/name diagnostics, but page control commands must be validated and serialized.

For hardware, publish a display `light` only when the platform reports power/brightness support. Publish volume as a separate supported entity. Unsupported controls should be absent or explicitly marked unavailable, not shown as fake working switches.

Add retained-topic cleanup with a versioned registry and a dry-run list before deletion. State and command topics need availability, origin metadata and a result topic for every command.

## P1: security and privilege handling

### 9. Protect the Admin boundary

- Default Admin binding should be explicit at setup. Do not rewrite it during package installation.
- Prefer loopback by default, with a guided LAN bind option and a clear warning. If LAN access is required, support TLS with certificate validation and display the actual URL.
- Hash persisted session identifiers at rest, rotate session IDs after login, expire them by age and inactivity, and invalidate them on password change.
- Add a CSRF token for cookie-authenticated state-changing requests in addition to same-origin checks.
- Do not trust `X-Forwarded-For` unless a configured trusted proxy is present.
- Add `Retry-After` and a visible countdown for Admin login throttling. Ensure failed attempts never result in a Home Assistant request.
- Add audit events for login, password changes, config import/restore, privilege activation, update install, rollback, session reset and HA guard actions.
- Keep secrets out of logs, job output, diagnostics and release artifacts.

### 10. Replace password reuse with scoped privilege sessions

The current privilege password is held in process memory for a short TTL, which is safer than writing it to config but still broad. Make the scope visible and narrow it to approved operations:

- Prefer passwordless, narrowly scoped `sudoers` rules for service restart, display tools, APT and power operations.
- Otherwise request the password per maintenance operation or activate a short-lived, explicit privilege session.
- Never pass user-controlled shell fragments to `su -c`; use fixed argument arrays and allowlisted operations.
- Show preflight results before starting APT/update jobs, including whether the configured mode is `sudo`, `su` or passwordless.
- Keep update installation, service restart and rollback as a single job with stages and a post-restart verification.

## P1: persistence, migration and packaging

### 11. Introduce a tested config migration boundary

- Deep-copy snapshots or move to immutable snapshot values so workers cannot share mutable slices/maps.
- Separate persisted configuration, redacted API configuration and runtime status types.
- Add schema version migrations with a backup before each migration.
- Migrate duplicated `urls`, `pages`, `rotation`, `time_rules` and scheduler fields into the canonical workflow while preserving old fields for one compatibility release.
- Preserve stable page IDs, profiles, MQTT node identity and Admin sessions across updates.
- Add import preview, validation errors and rollback to the previous config before applying.
- Test upgrades from representative 0.5, 0.6 and 0.7 configurations, including empty, malformed and legacy values.

### 12. Make Debian releases boring

- Generate control metadata through one implementation or validate both builders against the same fixtures.
- Always end control fields and descriptions with a newline.
- Run `dpkg-deb --info`, `dpkg-deb --contents` and a disposable install/upgrade test in CI.
- Verify package architecture, version ordering, executable permissions, service path and dependency alternatives.
- Remove the bind-rewrite `sed` migration. If a compatibility migration is needed, run it in KioskMate with a backup, an audit event and an explicit UI result.
- Use user-service-aware install scripts that reload units without globally enabling an unexpected service for every user.
- During update: download, verify SHA-256, verify package metadata, back up config, install, daemon-reload, restart, verify version and preserve the old package as a rollback target.
- Publish arm64 and amd64 packages plus checksums and English release notes from the tag.

## P2: observability and performance

### 13. Build an evidence trail for the next ban or hang

Add a bounded, privacy-safe event journal separate from verbose logs. Each event has:

- timestamp and monotonic duration
- component and operation ID
- browser generation and page ID
- URL origin/path category, never query credentials
- HTTP status/resource type or WebSocket event type
- action, result and retry/backoff decision

Expose it in Logs with filters for browser lifecycle, HA auth, page health, MQTT, update and privilege. Show when a journal source is unavailable instead of “no entries”. Include a redacted export that can be attached to an issue.

### 14. Measure performance correctly

Separate budgets:

- **KioskMate core**: no busy polling, bounded goroutine count, no unbounded event queue, Admin status fast path under 100 ms when cached.
- **Chromium**: one expected process tree per kiosk window; no duplicate browser tree after 10 minutes; memory shown as PSS; CPU shown as process-tree percentage with core count.
- **Network**: no repeated HA login/token requests; health checks obey the configured interval and backoff.
- **UI**: no full-page rerender during typing; no request fan-out on every control change; snapshots are on demand and cached.

Add a benchmark harness that records startup time, first usable frame, RSS/PSS, CPU, process count, page-switch latency, reload latency, MQTT publish latency and recovery latency. Test at least:

- Raspberry Pi 4, 4 GB, Wayland, Chromium, one HA dashboard.
- Raspberry Pi 4 with camera cards and multiple media resources.
- A desktop amd64 control run.

Do not hard-code a universal CPU promise for arbitrary HA dashboards. The release gate is “no KioskMate-induced restart/request storm” plus a measured comparison against the previous build on the same dashboard.

### 15. Add the missing test layers

P0/P1 tests:

- Go race tests for config, browser lifecycle, scheduler and MQTT publish/command paths.
- Fake CDP server with interleaved responses/events, reconnects, target changes and initial auth failures.
- HA HTTP/WebSocket fixture for 200, timeout, 401, 400 `invalid_grant`, 403 resource failure and confirmed auth rejection.
- MQTT broker fixture or container test for 3.1.1 and 5.0 CONNACK reason codes, TLS and retained discovery cleanup.
- Admin API tests for bootstrap, timeout, CSRF, rate limit, session rotation and redaction.
- Browser E2E tests for login, Dashboard action, dirty form refresh, page wizard, live MQTT test and update preflight.
- Package install/upgrade tests on Debian arm64/amd64 runners or disposable containers.
- Fuzz tests for MQTT remaining length, properties and malformed packets.

Release gates:

```text
gofmt + go vet + go test -race
Node syntax + i18n parity + frontend build
CDP/HA/MQTT integration fixtures
Debian metadata/install/upgrade checks
Desktop smoke test
Raspberry Pi 4 soak: 24h, no duplicate browser tree, no unexplained restart loop
```

## Timeboxed implementation order

This is a practical 18-hour agent work allocation, not a promise about wall-clock completion or hardware soak time.

| Time | Work package | Exit result |
| ---: | --- | --- |
| 1.0 h | Baseline, fixtures and failure reproduction | Current metrics, test matrix and a reproducible fake HA/CDP/MQTT harness. |
| 4.0 h | Browser lifecycle serialization and startup ordering | Admin stays available; operations are serialized and observable. |
| 3.5 h | CDP multiplexer, navigation attach and HA classification | Initial auth events are captured; 403/resource/transport states are separated. |
| 2.0 h | Recovery circuit breaker, stable profiles and config deep-copy | No stale timer restarts; page IDs preserve sessions; races are covered. |
| 3.0 h | Frontend module split, state store and operation/toast model | UI refresh no longer destroys active input; actions have consistent feedback. |
| 1.5 h | Kiosk AIO workflow cleanup and navigation grouping | One understandable page/workflow task with fewer duplicate controls. |
| 1.5 h | MQTT diagnostics/discovery and HA capability truth | Test dialog and entity state match actual broker/platform behavior. |
| 1.0 h | Security/privilege/session hardening | No package bind rewrite; session and privilege boundaries are explicit. |
| 0.5 h | Package CI checks, changelog and release checklist | Bad `.deb` metadata cannot reach a release. |
| 0.5 h | Local verification and handoff | Tests, known limits and Pi soak instructions documented. |

The 24-hour upper bound is reserved for deeper E2E coverage and migration fixtures. The 24-hour Raspberry soak itself runs after the implementation and is a release gate, not work that can be compressed into coding time.

## Explicitly out of scope for this milestone

- Replacing Chromium with a new browser engine. The HA frontend and camera cards still require a real browser; first make its lifecycle measurable and stable.
- Silently removing HA bans, disabling HA IP protection or storing an HA administrator password.
- A cloud backend, remote fleet management or public internet exposure of the Admin API.
- A full node-editor dependency before the linear workflow is reliable and accessible.
- Aggressive Chromium flags chosen only because they reduce one synthetic CPU number. Every flag must be benchmarked against rendering correctness and media behavior.

## Definition of done

The milestone is complete when:

1. A Raspberry Pi can boot KioskMate with the Admin UI reachable even if Chromium or Home Assistant is down.
2. A real Home Assistant login/session rejection is detected once, explained, and does not cause a retry storm or automatic password attempt.
3. Browser start/stop/reload/restart/reset are serialized, visible and recoverable.
4. The Kiosk screen lets a non-technical user add, order, preview, schedule and activate multiple pages without hunting through duplicate settings.
5. MQTT 3.1.1 and 5.0 tests show broker reason codes and publish the correct capability-based HA entities.
6. Updates preserve config and sessions, validate the Debian package, reload the user service automatically and verify the new version.
7. German and English cover every visible state, and the Admin UI survives refreshes while the user is typing.
8. The release has automated tests plus a documented 24-hour target-device soak result.

## References

- [Home Assistant HTTP integration: IP filtering and banning](https://www.home-assistant.io/integrations/http/)
- [Home Assistant Authentication API](https://developers.home-assistant.io/docs/auth_api/)
- [Chrome DevTools Protocol Target domain](https://chromedevtools.github.io/devtools-protocol/tot/Target/)
- [TouchKio, the original inspiration](https://github.com/leukipp/touchkio)
