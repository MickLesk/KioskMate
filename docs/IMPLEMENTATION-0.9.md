# Implementation tracker: 0.9.0

Branch: `dev-kiosk-0.9`. Baseline: `a44b8be` (`v0.8.0`).

Theme: **Production-ready kiosk experience**. Existing 0.8 capabilities count only after their behavior is covered by an automated check or an explicit target-device verification.

## P0: runtime and performance

- [x] Add browser responsiveness, DevTools heartbeat and first-frame/navigation timing.
- [x] Expose Chromium process roles, process count, PSS and CPU separately from the Go core.
- [x] Add staged recovery (`reload -> reconnect -> restart`) with a visible circuit breaker.
- [x] Detect and clean up duplicate Chromium roots for the configured profile.
- [x] Harden display-session readiness and retain actionable startup failures.
- [x] Add adaptive Raspberry Pi profiles and media-safe performance recommendations.
- [x] Extend the benchmark harness with startup, page-switch, reload and process-tree metrics.

## P0: Home Assistant safety

- [x] Keep transport, page-resource, session-expired and probable-ban states separate end to end.
- [x] Add a guided recovery result with one safe next action and no automatic credential submission.
- [x] Verify that health checks back off and stop while the authentication guard is active.
- [x] Add deterministic HTTP/WebSocket fixtures for 200, timeout, 401, invalid grant and corroborated 403.

## P1: Admin UI and workflow

- [x] Introduce one shared request/action state model with cancellation, timeout and structured feedback.
- [x] Add a persistent global action center and compact event timeline.
- [x] Make Dashboard status, preview, recovery and next transition immediately understandable.
- [x] Consolidate Pages, timing, schedules and triggers in one canonical workflow editor.
- [x] Add schedule/trigger conflict detection and save/apply impact reporting.
- [x] Add keyboard-accessible page ordering and complete modal focus containment/restoration.
- [x] Split design tokens by responsibility and ship deterministic gzip/ETag embedded assets.
- [x] Add responsive browser E2E coverage for login, dashboard, wizard, MQTT and updates.
- [x] Keep German and English complete, canonical and free of hard-coded visible runtime errors.

## P1: MQTT and Home Assistant

- [x] Decode and expose MQTT 5 connection reason details consistently.
- [x] Add a discovery dry-run with retained-topic additions and removals.
- [x] Publish command acknowledgements with correlation IDs.
- [x] Complete stable per-page controls and workflow timing diagnostics in Home Assistant.
- [x] Verify capability-driven display, brightness and audio entities without `null` states.
- [x] Add MQTT 3.1.1/5.0 broker fixtures including TLS and retained cleanup.

## P1: system, security and updates

- [x] Add capability freshness and actionable install hints for optional hardware tools.
- [x] Improve NTP/time diagnostics and surface drift/synchronization state globally.
- [x] Keep Terminal disabled by default and gate command execution explicitly.
- [x] Add trusted-proxy-aware client addressing and clearer LAN/TLS setup boundaries.
- [x] Narrow privileged operations and expose their exact preflight scope.
- [x] Add update rollback verification and package provenance/SBOM metadata.
- [x] Verify 0.8 config/profile/session migration without data loss.

## P2: release engineering

- [x] Add fuzz coverage for MQTT packets and configuration imports.
- [x] Add Debian install/upgrade/removal integration tests.
- [x] Add a release smoke test for both package architectures.
- [x] Update README, hardware documentation and troubleshooting for 0.9.
- [x] Build and verify local `0.9.0-dev` arm64 and amd64 packages.
- [ ] Run a Raspberry Pi 4 24-hour soak with no duplicate browser tree, restart storm or HA request storm. *(Target device required.)*

## Release gates

- [ ] `gofmt`, `go vet`, `go test`, `go test -race` on Linux. *(All local gates pass; the race detector is verified by Linux CI.)*
- [x] JavaScript syntax, i18n parity and embedded-asset contract checks.
- [x] Admin E2E and HA/CDP/MQTT integration fixtures.
- [ ] Debian metadata, install, upgrade and service-file verification. *(Metadata and package contents pass locally; lifecycle verification runs in Linux CI.)*
- [ ] Measured comparison against 0.8.0 on the same Raspberry Pi dashboard.
