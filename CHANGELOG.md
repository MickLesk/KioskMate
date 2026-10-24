# Changelog

## v1.0.0-pre.2

Second pre-release: the redesigned Admin UI gets the depth it was missing. Configs from 1.0.0-pre and 0.9.x keep working unchanged.

### Conditions, presence and sun times
- Inserts can depend on an MQTT value ("only when `home/motion` = ON", or negated). The timeline marks conditional inserts and shows whether the condition holds right now.
- Presence: an MQTT topic, `POST /api/presence` or a touch counts as someone being there. Mode "wake" keeps the display on outside the schedule for a set time; mode "require" turns it off inside the schedule when nobody is there.
- Time windows of pages, time rules and inserts accept sunrise and sunset with an offset (for example "sunset −30 min"). With the kiosk's location (Programme → Advanced, or "Use this device's location") they follow the real sun times; without it sunrise counts as 06:00 and sunset as 18:00.

### Local page templates
- Five new pages that KioskMate renders itself, chosen like the clock under Pages → Add page:
  - **Weather** from Open-Meteo without an API key: now, the next eight hours and six days, for the page's own coordinates or the kiosk location.
  - **Countdown** to a date, with a text or counting up once it has passed.
  - **News**: headlines from up to five RSS or Atom feeds, rotating, with suggestions (tagesschau, heise, SPIEGEL, BBC).
  - **Camera**: up to four go2rtc, MJPEG or snapshot streams in a grid; frozen streams reconnect.
  - **Immich photos**: an album or random photos as a photo frame. The API key stays on the device and is never sent to the Admin UI.
- Weather, news and Immich data is fetched and cached by KioskMate (15 minutes, the last answer is kept while a service is offline), so pages stay sandboxed and work behind CORS.

### Home Assistant
- MQTT discovery adds a sensor for the active insert, a switch per insert, a presence button, an occupancy sensor (when presence is on) and an undervoltage problem sensor.

### Setup and editing
- First-run assistant on a fresh install: page, when the screen is on, optional MQTT. It can be reopened under Settings.
- Removing a page, insert or workflow no longer asks first; a toast offers "Undo" instead.
- Duration and time window of base programme pages can be edited directly in the list.
- Saving checks whether the config was changed elsewhere in the meantime (another browser, MQTT, the API) and asks before overwriting. `GET /api/config` returns a `revision`; a `POST` with an old revision answers 409 unless `?force=1`.
- Page previews: KioskMate keeps a small screenshot of each page after it has been on screen for 20 seconds (at most every 30 minutes per page) and shows it in Pages and the programme.

### Diagnosis
- Doctor shows a sorted, translated list of checks with technical details folded away, and on a Raspberry Pi checks hardware video decoding: CMA size and allocation failures, `gpu_mem`, the decoder Chromium reports. It recommends `dtoverlay=vc4-kms-v3d,cma-512` when the decoder is starved.

### Fixes
- The page wizard and the setup assistant scrolled their content underneath the step bar.
- Buttons whose handler was a named function could stop working after a re-render (the Doctor button among them).
- Doctor no longer gives up after 12 seconds on slow devices.

### Development
- Browser tests of the Admin UI (`tests/e2e`, Playwright) run in CI against a real server.

### Programme
- The base programme is a clear list of only the pages that run in it, each with its time window as a 24-hour strip or its rotation duration, "Show now", edit, move up/down (or drag) and remove. Insert-only and trigger pages are listed separately with "Add to base programme".
- "Show now" takes a duration (15 minutes to 8 hours); an active override shows its end time and a "Back to programme" button.
- The flow timeline shows 1, 6 or 24 hours and draws the base programme (time windows, rotation, display off) underneath the inserts. Inserts that fall into a display-off period are left out, as the scheduler never wakes the display for them.
- Every insert shows its next run (also "tomorrow 08:00"), can be duplicated, and an empty insert list offers templates (every 5 minutes, on the hour, at fixed times).
- `/api/program/timeline` accepts `day=1` (today from midnight) and returns each insert's next run within two days.

### Overview
- The live view can refresh itself every 30 seconds while the overview is open (on by default, switchable; no captures in the background).
- "Up next" lists each insert once with its rhythm plus the next change of the base programme, such as "23:00 display off".
- "Today" shows the whole day from midnight with the base programme and all inserts.
- A stopped display is shown as stopped, with the page that would be planned now.

### Pages
- Filter chips (base programme, inserts, triggers, unused, disabled) next to the search, a switch per page to enable or disable it, and per page check, duplicate and open in a new tab. "Check all pages" puts the result on each card.
- "Show now" uses the saved page, so unsaved reordering never shows the wrong one.

### Automations
- Six templates (doorbell shows camera, display off at night, display on in the morning, back to the programme after use, touch wakes the display, webhook shows a page) open the editor prefilled.
- Workflows show their trigger as an icon, an on/off switch and their last run; the MQTT connection state sits on top.

### Device and messages
- Browser and recovery messages are shown in plain language ("The browser quit by itself after 3 ms", "Next start in 10 min") instead of raw supervisor strings.
- The display tile says "not controllable" instead of "-", and the status tiles wrap evenly.
- The content area uses the width of large screens and stays centred.

### Updates
- Pre-releases are ordered among themselves (1.0.0-pre < 1.0.0-pre.2 < 1.0.0-rc.1 < 1.0.0), so a pre-release is offered the next one when pre-releases are enabled.

## v1.0.0-pre

Pre-release of the redesigned Admin UI. Configs from 0.9.x keep working unchanged.

### Programme and inserts
- Added inserts (`kiosk.inserts`): pages that come in between the base programme on a fixed rhythm, every N minutes, hourly at a given minute or at fixed times, for a set duration, optionally only within a daily window and on chosen weekdays. Example: Home Assistant 07:00-23:00, the weather and cameras every 5 minutes for 30 seconds and today's calendar at every full hour for one minute.
- Overlapping inserts are decided by priority (then list order). Manual overrides and workflows still win over inserts, an insert never wakes a display the time windows turned off, and without a base programme the page shown before the insert returns afterwards.
- The scheduler wakes exactly for each insert start and end instead of waiting for the next tick.
- Added the page display mode "insert" (only shown by inserts or on demand, not in the rotation).
- Added `/api/program/timeline`, which previews the next hour and the upcoming insert runs, also for unsaved changes.

### Admin UI
- New structure with five areas: Overview, Programme, Pages, Automations and Device. Device groups health, display, maintenance, logs and settings as tabs; Automations holds workflows and the MQTT connection. Bookmarked views of 0.9 open their new place.
- Overview shows the live view, what is on screen and why, up next, page selection, reload and display power, and today's programme as a bar. Health problems appear as one banner with a link to Device.
- Programme shows the base programme, the inserts with an editor (rhythm, duration, window, days, priority, "insert only" and preloading, with a plain-language summary), the next hour as a timeline and the order that wins on overlaps.
- Pages is a library of cards with type, address and where each page is used (base programme, time window, insert, workflow), with "Show now".
- Device opens with a health checklist (power, touch, browser, Home Assistant login, MQTT, memory and temperature, storage, time) that names a fix for every warning.
- Phone layout with a bottom bar and a drawer for the remaining areas.
- Timeline geometry is applied through the CSSOM, so it works with the strict Content-Security-Policy (inline styles stay blocked).

### Updates and releases
- A pre-release such as 1.0.0-pre now counts as older than the final 1.0.0, so the update check offers the final release.
- The release workflow builds tags of any major version and marks `-pre` tags as pre-releases.

## v0.9.1

### Hardware
- Reported undervoltage on Raspberry Pi boards (live from the firmware voltage sensor, since boot from `vcgencmd get_throttled`) as a banner on the dashboard and on Device and Time, and in the Doctor. A weak supply throttles the CPU and can drop USB touch controllers.
- Reported registered touch input devices under Display and Input and in the Doctor, so a touchscreen the kernel never saw is visible without `libinput debug-events`.

### Admin UI
- Showed browser CPU as a share of the whole device plus the cores in use (for example "43 %, 1.7 / 4 cores" instead of "171 %"), and the system tile as load average next to the core count.
- Translated the hardware tables and replaced raw values such as the disk usage JSON with readable text.

### Kiosk
- Added `kiosk.display_backend` (`auto`, `wayland`, `x11`; Settings -> Performance) to choose Chromium's display backend. `auto` keeps native Wayland in a Wayland session; `x11` forces XWayland where a setup needs it. An `--ozone-platform` switch in the extra browser arguments always wins.

## v0.9.0

### Lower resource use on small hardware
- Cached slow hardware probes (sudo rights, display command, session type, DDC/CI) instead of re-running them on every status request; **System -> Rediscover hardware** forces a fresh scan.
- Limited Chromium network monitoring for the Home Assistant auth guard to five minutes after each navigation, with capped DevTools buffers and large WebSocket frames dropped before decoding (`perf.auth_monitor: "always"` keeps the old behaviour).
- Reused process memory (PSS) samples for a minute and built the process tree in one pass.
- Merged Chromium `--enable-features` flags, enabled GPU rasterization in hardware GPU mode, stopped adding `--disable-dev-shm-usage` and reported the GPU feature status in the Doctor.

### Kiosk
- Added a renderer heartbeat: a page that stops responding is reloaded, then the browser restarts, and optionally the system reboots when restarts stop helping.
- Added workflows: rules triggered by MQTT, a time of day, a webhook, touch input or inactivity that show a page, switch the display, set brightness, reload, show a banner, click, scroll, play a sound or run a script, with conditions, cooldown, fixed priority below manual overrides, conflict warnings, a test button and run history.
- Added local pages that KioskMate renders itself: a clock, an image slideshow from a folder and custom HTML, sandboxed and only served to the device.
- Added an optional tab mode that keeps recent pages open and switches without reloading, with per-page preloading.
- Added scheduled maintenance reloads every N minutes, at a fixed time or after N rotation rounds.
- Added an optional desktop-free mode with the Cage compositor.
- Added a ready-made Raspberry Pi OS Lite image (pi-gen, `image/build.sh`, manual workflow) with Cage, Chromium and first-boot setup from `kioskmate.txt`, plus `--set-admin-password-stdin` and `--set-start-url` command-line flags.

### Windows
- Added experimental Windows 10/11 support: Edge or Chrome discovery, job-object process handling, configuration in `%APPDATA%\KioskMate`, browser profiles in `%LOCALAPPDATA%\KioskMate`, WMI monitor brightness, a Task Scheduler installer (`install.ps1`) and `windows_amd64`/`windows_arm64` zip packages; Linux-only hardware controls and system actions report "unsupported on Windows" ([docs/WINDOWS.md](docs/WINDOWS.md)).

### Admin UI
- Converted the Admin UI to ES modules with per-language translation files loaded on demand.
- Added live updates over Server-Sent Events and DOM morphing so open forms keep focus and input.
- Startup loads only what every view needs; journal, jobs, time zones and update history load with their view.
- Served assets with ETag revalidation and gzip.
- Split the stylesheet into tokens, layout, components and views and removed unused rules.
- Release packages embed a minified Admin UI (about 25% smaller, 12% after gzip).
- Live updates keep running while a form has unsaved changes; edited fields and action results stay as they are.

## v0.8.0

### Runtime reliability and diagnostics
- Added structured browser exit details and persisted reason counters so controlled stops, restarts, recovery actions, startup failures and crashes are distinguishable in Admin and Home Assistant.
- Added explicit service-shutdown handling, browser runtime duration and exit-code diagnostics, and privacy-safe URL reporting.
- Paused passive Home Assistant page health checks while the authentication guard is active to avoid repeated requests during an IP ban or rejected session.
- Hardened Home Assistant authentication evidence with confidence levels, corroboration for generic page `403` responses, bounded retention and first/last-seen metadata.

### Configuration and Admin security
- Added dry-run previews for configuration imports and restores, including changed areas, page counts and required browser or service restarts.
- Added rolling atomic configuration backups with bounded history while preserving the existing last-known-good backup.
- Added an explicit schema-migration boundary, centralized save-time validation and visible Admin warnings for recovered or migrated configurations.
- Improved login throttling with bounded client state, stale-entry pruning, `Retry-After` status and a persistent visible countdown after reload.
- Expanded sensitive-value redaction for URLs in Admin responses and the structured event journal.

### MQTT and updates
- Added a persistent, validated Home Assistant discovery-topic registry so stale entities from deleted pages, renamed nodes or changed discovery prefixes are removed after restarts.
- Separated MQTT status locking from broker network I/O so a slow or unreachable broker no longer stalls Admin status requests.
- Strengthened built-in update validation with exact release-version matching plus executable and systemd-unit content checks before installation.

### Build quality
- Canonicalized the English and German translation catalog and made canonical form, parity and referenced-key validation a build gate.
- Enabled CI for development branches, added race testing to release builds and consolidated Debian packaging behind one deterministic generator with stronger artifact checks.

## v0.8.0-alpha2

### Diagnostics and Home Assistant
- Added a bounded, privacy-safe event journal for browser operations, HA authentication guards, MQTT connection/command state, admin security actions and maintenance/update jobs.
- Added an Events log source, combined-log output and diagnostic-bundle export for the structured journal; sensitive fields and MQTT payloads are excluded.
- Made Home Assistant discovery capability-aware so unsupported display, audio, input and host sensors are cleared instead of being published with unusable `null` states.

## v0.8.0-alpha1

### Browser reliability
- Serialized browser lifecycle operations and persisted recent operation history so start, stop, reload and recovery actions have one authoritative result.
- Added stable per-page Chromium profile identities, including migration support when pages are reordered or renamed.
- Reworked the DevTools transport around one bounded event reader, reducing command races, unbounded event growth and false recovery triggers.
- Added recovery backoff and hourly restart limits; CPU-only watchdog restarts are opt-in instead of being triggered by normal multi-core Chromium load.
- Starts the Admin control path before browser navigation and preserves actionable diagnostics when Chromium or the display session is unavailable.

### Home Assistant session safety
- Detects Home Assistant `auth_invalid`, invalid token responses and relevant authentication `401`/`403` responses without treating camera or media resource failures as a login ban.
- Stores authentication-guard evidence, the affected origin and the local kiosk address so an IP-ban incident can be diagnosed from Admin.
- Runs a safe Home Assistant preflight before browser startup and blocks automatic navigation while the persistent authentication guard is active.
- Applies kiosk theme preferences through the controlled DevTools session after attach, including dashboards that provide their own custom UI theme.

### MQTT and Home Assistant
- Added validated MQTT transport configuration for `mqtt://` and `mqtts://`, custom CA files, mutual TLS, SNI override, certificate verification and maximum packet size.
- Kept MQTT 3.1.1 and 5.0 support explicit in the Admin UI and expanded connection-test diagnostics for validation, CONNACK and publish failures.
- Preserved the existing Home Assistant discovery model, including per-page controls and display/audio capability entities.

### Admin and packaging
- Added hashed persistent Admin sessions, CSRF protection for mutations and `Retry-After` details for rate-limited sign-ins.
- Added operation IDs, action history and safer background refreshes that do not overwrite focused or unsaved forms.
- Added release-time syntax and translation checks for the embedded Admin UI.
- Hardened Debian packaging and added verification for package metadata, maintainer scripts, service files and architecture-specific artifacts.

## v0.7.7

### Privileges / APT jobs
- Privilege status no longer stays “Not configured” after typing a password: Activate verifies sudo/su and keeps a 15‑minute in-memory session; starting a job with a password does the same.
- Explicit **Activate privileges** button; clearer session remaining time in the status tile.
- APT job output flushes carriage-return progress (`Reading package lists...`); noninteractive apt env; UI polls until the job finishes (not only ~80s).

### Telemetry
- Memory uses Linux PSS (`smaps_rollup`) instead of summing per-process RSS, which heavily over-counted Chromium shared pages (~1.5 GB looked worse than reality).
- CPU labels clarify that 100% = one core across the Chromium process tree (values above 100% are normal on multi-core).

## v0.7.6

Large hardening wave (~40 concrete fixes) for MQTT/HA, scheduler restore, Admin indexing, sessions and packaging.

### Scheduler / workflow
- Restore the real page after schedule idle-blank (`about:blank`) when the window becomes active again.
- Only mark idle-blanked after a successful blank; ClearOverride immediately re-applies the scheduler target.
- Reset rotation state on config changes; hybrid mode powers the display off outside schedule windows when any schedule page exists.
- Admin activate/override now map absolute UI page indexes to enabled-page indexes; remaps after reorder/delete stay consistent.
- `saveScheduler` mirrors edited time rules back onto page schedules (pages remain source of truth).
- Dashboard surfaces critical scheduler reasons; wizard save reminds to persist; non-functional screensaver toggle removed from the wizard.

### MQTT / Home Assistant
- MQTT 5 Last Will properties byte; command keepalive waits for PINGRESP.
- Page activate / page_number use temporary overrides; trigger topic mismatches no longer fall through into command handlers.
- `page_url/set` captures the active index before mutate; discovery is capability-gated and clears unsupported retained entities.
- Stale deleted page discovery/state topics are cleared; HA health 403 is classified as auth-required (not “down”).
- `needs_restart` binary sensor after theme/GPU/profile MQTT changes; override select state tracks the override page name.
- Availability publish errors propagate; discovery reset covers override/recovery/telemetry entities.

### Hardware / packaging / Admin
- `wlr-randr` Wayland fallback; clearer ddcutil sudo errors; brightness/volume invalidate status cache; shorter hardware cache TTL; apt upgrade count cached.
- Persist Admin sessions across restarts (throttled LastSeen); password change invalidates all sessions; atomic setup PasswordHash race closed.
- Remove hardcoded `WAYLAND_DISPLAY`; strengthen `wlopm` packaging recommends; prerm/postrm stop the user service; postinst restarts enabled user units after upgrade; Doctor warns when Wayland display power tools are missing.
- MQTT `page_url/set` maps enabled→absolute page indexes; `page_name/set` uses scheduler override; per-page `auth_required` discovery; publish loop serialized against concurrent map races.

## v0.7.5

- Fixed Zeitplan no-ops: saving a schedule workflow now enables the scheduler; existing configs with `time_rules` are migrated (config v3) so display power follows the window without hunting for “Run automatically”.
- Schedule pages default to `power_off_after`; pure `time` mode no longer gets a synthetic rotation injected; clocks accept `HH:MM:SS`; scheduler evaluates rules in the configured timezone.
- Outside the schedule window the panel is powered off and the browser is blanked once; page brightness from display options is applied on switch.
- MQTT: Last Will `availability=offline`, command-connection keepalive pings, serialized commands, page triggers use temporary overrides, `page_url/set` patches the active page only, and `page_number` is range-checked.
- Publish loop no longer holds the MQTT mutex across hardware/health probes; Admin activate uses a temporary override so the scheduler cannot steal the page back immediately.

## v0.7.4

- Fixed a MQTT publish deadlock that blocked Home Assistant state updates, connection status and follow-up commands after the first successful publish.
- Subscribed to `kiosk_theme/set`, refreshed MQTT command subscriptions when page/trigger configuration changes, and mapped generic `reload` to a page refresh instead of a full browser restart.
- Stopped passive Home Assistant health checks from tripping the auth guard on transient `/manifest.json` 403 responses.
- Wired schedule windows to display power: outside a pure time workflow the panel turns off, inside it turns back on, with an immediate first scheduler tick.
- Kept manual page overrides from forcing the display off, synchronized Admin/MQTT display-power changes with the scheduler cache, and guarded browser restart while the auth guard is active.
- Improved MQTT Admin UX (password retention signal, post-save runtime polling), Wayland display-env detection, sudo privilege probing for reboot/shutdown, and accurate browser status via the fast status endpoint.

## v0.7.3

- Moved Home Assistant network authentication monitoring ahead of theme synchronization and onto a dedicated CDP connection so `auth_invalid` events cannot be consumed by unrelated DevTools commands.
- Disabled Chromium password saving, credential autofill, password leak services and crashed-session restore in the dedicated KioskMate browser profile.
- Added a one-time non-destructive migration that quarantines existing Chromium password databases under `SessionBackups` without removing the active Home Assistant token, cookies or dashboard storage.
- Expanded explicit HA session resets to back up and remove saved Chromium login databases as well as cookies, local storage, IndexedDB and service worker state.
- Blocked reload, page navigation, scheduler switches and MQTT-driven page changes while the persistent Home Assistant authentication guard is active.
- Added a safe `/manifest.json` preflight that refuses to start Chromium when Home Assistant already returns an IP-ban `403` response.
- Added a Linux process lock before browser startup so overlapping service or manual KioskMate instances cannot launch multiple Chromium sessions.
- Added regression coverage for credential quarantine, profile preference preservation, auth-failure detection, guarded actions and Home Assistant ban preflight behavior.

## v0.7.2

- Removed all browser, MQTT, hardware and updater runtime locks from the authenticated Admin bootstrap path.
- Added the sanitized public configuration directly to login and authenticated-session responses so the Admin UI can render without a follow-up status request.
- Changed the login flow to use its successful authentication response immediately instead of waiting for another session and runtime round trip.
- Made the fast status endpoint strictly configuration-only so a stalled Chromium supervisor cannot block Admin access.
- Added regression tests proving login and session reload work without calling browser runtime status and never expose stored credentials.

## v0.7.1

- Fixed Admin sign-in stalls by loading only the required configuration and fast runtime status before rendering the authenticated UI.
- Moved slower hardware, privilege, time, maintenance and update-history requests into a resilient background refresh so one unavailable subsystem can no longer block login.
- Added bounded request timeouts, visible fatal-load recovery and explicit inline authentication errors instead of leaving the interface in a permanent busy state.
- Added versioned, non-cacheable embedded Admin assets to prevent mixed frontend versions after package updates and browser reloads.
- Added a server-rendered loading fallback so script or asset failures remain visible and recoverable instead of producing a black page.
- Redesigned the sign-in screen with clearer hierarchy, password visibility control, responsive mobile layout and complete English/German feedback.
- Added regression coverage for the fast status path, versioned assets, cache headers and the embedded authentication bootstrap contract.

## v0.7.0

- Added a persistent browser recovery state machine with reload-first recovery, controlled restart fallback, exponential crash-loop backoff and automatic recovery after unexpected browser exits.
- Persisted browser start/restart counters, recovery state, temporary page overrides and rolling 24-hour runtime telemetry in the private KioskMate configuration directory.
- Changed process monitoring to collect browser CPU, memory and process-count telemetry even when automatic watchdog restarts are disabled.
- Added 24-hour CPU/RSS averages and maxima, process-count diagnostics, telemetry reset and a compact runtime history chart to the Admin UI.
- Added device-aware browser profile recommendations for low-memory devices, Raspberry Pi 3/4/5 systems and larger installations, including one-click apply and restart.
- Added temporary page overrides with a configurable duration that supersede the scheduler and automatically return to the configured workflow when they expire.
- Hardened Home Assistant authentication handling with persistent failure classification, the detected kiosk IP address, explicit IP-ban recovery guidance and restart suppression while authentication is blocked.
- Expanded Home Assistant MQTT discovery with browser recovery, page override and telemetry controls plus recovery, backoff, authentication and 24-hour performance entities.
- Added Admin API endpoints for non-destructive browser recovery, runtime telemetry, temporary page overrides and device profile recommendations.
- Fixed selected-page activation in the Storybook/Flow editor so the visible selection, temporary override and direct activation always target the same page.
- Added English/German UI coverage for recovery, telemetry, recommendations and page overrides plus regression tests for persistence, expiry, backoff and hardware recommendations.

## v0.6.0

- Reorganized the remaining Admin UI around consistent runtime banners, status summaries, focused primary actions and secondary diagnostic disclosures.
- Added collapsible desktop navigation groups and a dedicated compact mobile menu so the control panel no longer pushes content below a large horizontal navigation strip.
- Simplified Dashboard operation to display start/stop, page reload and workflow management while moving restart, session reset and diagnostics into troubleshooting.
- Added state-driven Dashboard guidance for stopped displays, browser errors, the current page, the next scheduler switch and the real MQTT runtime state.
- Added MQTT connection readiness checks for broker address, credentials, protocol version and Home Assistant discovery, plus actionable connected, disabled and error states.
- Added a useful MQTT protocol empty state and preserved the detailed live connection test as the primary diagnostic path.
- Separated package and service maintenance from reboot and shutdown actions, added privilege guidance and replaced empty job output with a clear maintenance-job timeline state.
- Added live client-side log filtering, visible result counts and useful no-result feedback without additional server requests.
- Added Admin access summaries for the listening endpoint, active sessions and SSH key state, with clearer network-access and account-security grouping.
- Expanded and verified complete English/German translations, responsive layouts, keyboard-accessible navigation and embedded Admin UI regression contracts.

## v0.5.0

- Rebuilt Kiosk -> Pages and workflow as a single all-in-one sequence workspace instead of separate page, rotation and time-rule forms.
- Added a non-technical Storybook view with numbered page cards, source previews, duration badges, inline insertion, keyboard-friendly move actions and native drag-and-drop ordering.
- Added an optional visual flow view that renders START, page nodes, transitions and END/LOOP state for power users.
- Added a guided three-step page wizard for source details, URL validation, timing/trigger behavior, display actions and a plain-language review before activation.
- Added per-page display modes for custom duration, fixed schedules and MQTT triggers with conditional fields that keep advanced settings out of the normal workflow.
- Added stable `page_id` values, source metadata, page-level timing, schedule, trigger and display-option fields while preserving existing URLs, sessions, rotations and time rules during migration.
- Kept Home Assistant MQTT page entity IDs compatible for existing installations and made newly renamed pages retain their entity identity.
- Added custom per-page MQTT trigger topics and payload matching that activate the configured page without routing through the generic command topic.
- Unified page order, scheduler mode, rotation and fixed time rules behind one save path so the same configuration is no longer maintained in multiple places.
- Added responsive English and German UI text, clear selected-page actions, advanced diagnostics disclosure and zero-overflow mobile layouts.
- Added migration, MQTT trigger and embedded Admin UI contract tests plus browser-based desktop, mobile and wizard verification.

## v0.4.0

- Added persistent update history with install/rollback versions, lifecycle stages, timestamps, results and recovery metadata across service restarts.
- Added a complete update preflight for release availability, Linux/architecture compatibility, Debian tooling, temporary disk space and administrator authentication.
- Added strict downloaded-package validation for the `kioskmate` package name and the running device architecture before APT is invoked.
- Added private configuration backups under `~/.config/kioskmate/update-backups` before every package installation or rollback.
- Added post-restart version verification so an update is only recorded as installed when the expected binary version actually starts.
- Added a controlled rollback action that downloads, verifies and installs the previously working release with Debian downgrade protection explicitly enabled.
- Added first-upgrade migration from Debian's package log so an upgrade from v0.3.1 can still discover its rollback target.
- Added Admin UI preflight results, persistent update history, recovery status and rollback controls with responsive English/German guidance.
- Fixed the Home Assistant update entity to publish the real latest release instead of mirroring the installed version.
- Added Home Assistant update availability/installing diagnostics, last check, last error, rollback target and update check/rollback actions.
- Kept the updater locked through the restart window and reports a failed systemd restart instead of silently accepting it.

## v0.3.1

- Fixed built-in Debian updates when passwordless sudo is unavailable by supporting validated sudo and root credentials.
- Added a clear privilege preflight before package download, with optional credentials retained in process memory for 15 minutes and never written to disk.
- Added automatic update checks at startup and every six hours, cached release state and Dashboard/header notifications for available releases.
- Added named update jobs with live stages, streamed package-manager output, download progress, target version and actionable error details.
- Prevented concurrent update installations and preserved the last successful release result during temporary GitHub or network failures.
- Delayed the service restart until the successful job result reaches the Admin UI, then reconnects the page automatically after installation.
- Removed downloaded Debian packages after every completed or failed update attempt.
- Added responsive English and German update guidance plus regression coverage for privilege handling and updater lifecycle state.

## v0.3.0

- Consolidated kiosk pages, rotation and time rules into one Pages and workflow workspace with a single save path.
- Moved display appearance, Home Assistant theme behavior and browser performance controls directly under Kiosk.
- Removed the limited command runner from the normal navigation; remote administration remains available through SSH and authenticated maintenance actions.
- Added real MQTT runtime states for connecting, connected, authentication failure and transport errors in the Admin header and MQTT view.
- Fixed MQTT connection tests and config saves so an existing stored password remains usable when the redacted password field is left blank.
- Added live MQTT failure details, last connection and last publish timestamps, plus clearer Home Assistant discovery status.
- Added real Linux time synchronization diagnostics, a timezone selector and privileged systemd-timesyncd configuration jobs.
- Added the kiosk device clock and NTP synchronization state to the global header.
- Made display, brightness, audio, microphone and on-screen keyboard controls capability-driven so unsupported controls are no longer presented as functional.
- Added persistent maintenance job listing across Admin page reloads with streamed command output, state, duration and exit result.
- Simplified single-page controls by hiding previous and next navigation when only one kiosk page is enabled.
- Moved bulk page checks, import/export and render diagnostics into an advanced actions disclosure.
- Added Home Assistant diagnostic entities for MQTT connection state, last publish time, timezone, NTP server and time synchronization.
- Expanded English and German translations for the reorganized workflow and runtime states.

## v0.2.1

- Added reliable unsaved-change tracking across kiosk pages, schedules, MQTT, device time, browser, access and raw configuration forms.
- Added navigation and browser-close protection so edits cannot be discarded silently.
- Disabled save actions when a form is unchanged and added a clear saved/unsaved state to persistent action bars.
- Added translated validation for kiosk page URLs, scheduler timing and references, rotation durations and MQTT broker settings.
- Replaced free-form scheduler weekday input with an accessible seven-day selector while preserving the existing configuration format.
- Fixed the kiosk page filter so typing no longer rerenders the application, drops focus or loses pending field values.
- Removed obsolete scheduler handlers from the page-management view and consolidated page creation into one action path.
- Added direct progress labels to running buttons, dismissible live-region toasts and Escape/focus handling for dialogs.
- Added an actionable empty state for first-time kiosk page setup and retained zero-overflow mobile behavior.

## v0.2.0

- Rebuilt the embedded Admin UI around five predictable operating areas: Dashboard, Kiosk, MQTT, System and Settings.
- Split Kiosk management into independent Pages and Schedule views so saving page changes can no longer overwrite rotations or time rules, and vice versa.
- Reworked the Dashboard into a focused quick-control surface with live snapshot, active-page controls, health indicators and a collapsed recovery section.
- Consolidated device controls, hardware status, NTP and timezone settings under System -> Device and time.
- Consolidated privileged package, service and power actions, configuration repair and job output under System -> Maintenance.
- Moved browser appearance, performance, watchdog and advanced Chromium settings into clearly separated Settings sections.
- Moved updater state, changelog and update jobs into Settings -> Updates while keeping repair operations with system maintenance.
- Reorganized MQTT into connection, Home Assistant discovery, advanced options, live test output and a single persistent save action.
- Added responsive layouts with contained horizontal navigation and verified zero document overflow on mobile viewports.
- Added complete English/German labels for the new information architecture and automated translation parity checks in CI and release builds.
- Reduced duplicated controls, nested panels, visual noise and unsolicited background diagnostics while preserving all existing APIs and configuration data.

## v0.1.11

- Matched TouchKio's Electron `nativeTheme` behavior by emulating `prefers-color-scheme` directly on Chromium's active rendering target through DevTools.
- Preserved the Home Assistant user's selected custom theme instead of replacing it with the built-in default theme.
- Updated theme diagnostics to verify both the browser media preference and Home Assistant's actually applied dark mode.

## v0.1.10

- Fixed native dark mode when the Home Assistant kiosk user had selected a custom theme without dark-mode support.
- KioskMate now explicitly selects Home Assistant's built-in `default` theme together with the requested light or dark mode.
- Added live Home Assistant theme synchronization status to the Dashboard, including the selected theme, applied color mode and timeout errors.

## v0.1.9

- Fixed Home Assistant dashboards remaining light while the KioskMate kiosk theme was set to dark.
- Added native Home Assistant theme synchronization through the existing local Chromium DevTools connection, avoiding the CPU/GPU cost of forced page color transformation.
- Clarified that Home Assistant theme changes apply to the signed-in kiosk user after saving and restarting the display browser.

## v0.1.8

- Detached the Chromium process lifetime from short-lived Admin, MQTT and scheduler request contexts.
- Reworked browser stop, restart and session reset to wait for the complete Chromium process group before touching profile data.
- Added automatic backups for browser session data and support for current Chromium cookie and WebStorage paths.
- Added a persistent Home Assistant authentication circuit breaker that detects WebSocket `auth_invalid` responses and relevant HTTP 403 responses before repeated reconnects can trigger more login failures.
- Added local Chromium DevTools control for page reloads, navigation and screenshots without restarting the browser.
- Replaced automatic temporary-profile screenshots with explicit, cached screenshots of the real signed-in kiosk session.
- Changed Home Assistant health probes to use the public manifest endpoint with redirect limits and exponential error backoff.
- Added Home Assistant MQTT diagnostic entities for the authentication guard and DevTools connection.
- Made config updates synchronized and config writes atomic while preserving backups across package upgrades.
- Redacted Admin tokens, password hashes and MQTT passwords from the Admin config API and exported config files.
- Added Argon2id password hashing with automatic migration from existing KioskMate password hashes.
- Added same-origin request validation, stricter browser security headers, server timeouts and optional built-in TLS certificate settings.
- Added log rotation, crash diagnostic retention, bounded Admin sessions/jobs and 15-minute in-memory privilege credential expiry.
- Fixed MQTT state caching so failed publishes are retried and serialized publisher/client state to avoid concurrent access.
- Required SHA-256 digests and size validation before built-in updater package installation.
- Expanded CI with race tests, frontend syntax checks, cross-architecture builds and Debian package builds.

## v0.1.7

- Split the embedded Admin UI into separate `index.html`, `app.css`, `theme.js`, `i18n.js` and `app.js` files.
- Added an embedded asset handler for `/assets/*` with explicit content types.
- Added test coverage for embedded Admin UI asset delivery.
- Added Browser Doctor checks for browser binary, display environment, runtime directory, profile permissions, active page reachability and log tails.
- Replaced the Dashboard iframe preview with a backend-rendered snapshot endpoint.
- Added a browser recovery workflow endpoint and Dashboard action for stuck Home Assistant sessions.
- Added automatic browser crash diagnostic text files under the KioskMate config diagnostics directory.

## v0.1.6

- Made browser start actions fail loudly when Chromium exits immediately instead of returning a misleading success.
- Added browser/core log tails to failed browser action responses and the Admin UI failure dialog.
- Wrote browser command, arguments, display environment and process exit details directly to the browser log for every launch attempt.
- Changed the Dashboard live view to show a clear stopped-browser message instead of a broken iframe when the display browser is not running.
- Added a Dashboard browser diagnostics dialog.

## v0.1.5

- Reworked the Admin Dashboard into a central control center with grouped display, page, recovery and hardware/audio actions.
- Added a right-side live dashboard view with fallback guidance for Home Assistant installations that block iframe previews.
- Added button tooltips and clearer recovery guidance for reload, Home Assistant session repair, session reset and browser restart.
- Added Home Assistant MQTT discovery for a browser switch, display power switch and browser restart button while keeping `light.kioskmate_display`.
- Improved Chromium dark mode startup by requesting a dark preferred color scheme for the normal `dark` theme without enabling heavy forced dark rendering.

## v0.1.4

- Stabilized Raspberry Pi low-power browser startup by removing risky GPU/raster feature overrides from the default `raspberry` and `low-power` profiles.
- Kept the safer CPU and process reductions for low-power mode: one renderer, one raster thread and background-service reductions.
- Added automatic repair for stale browser profile paths that still point to old TouchKio/KioskMate alpha directories.
- Improved browser diagnostics so very fast Chromium exits are shown as a visible last error even when Chromium exits without stderr output.
- Logged the full browser launch arguments in the core log for easier Raspberry Pi troubleshooting.

## v0.1.3

- Added a stronger `low-power` performance profile for Raspberry Pi dashboards.
- Tightened the existing `raspberry` profile to use one renderer, one raster thread and reduced GPU/raster features.
- Changed Raspberry Safe Mode to use `chromium-lite`, `low-power` and GPU `auto` instead of software GPU rendering.
- Added Admin UI and Home Assistant MQTT support for the `low-power` performance profile.
- Added tests for low-power Chromium flags and Safe Mode browser settings.

## v0.1.2

- Changed kiosk theme behavior so `dark` uses the website's native dark theme instead of forcing Chromium's expensive page-wide dark renderer.
- Added explicit `force-dark` theme mode for installations that still need Chromium ForceDark, with Admin UI and MQTT config support.
- Updated tests and documentation to cover native dark versus forced dark behavior.

## v0.1.1

- Fixed a browser restart loop where CPU-only watchdog pressure could restart Chromium about once per minute on Raspberry Pi dashboards.
- Increased default watchdog CPU tolerance to 10 minutes and the default CPU limit to 300%.
- Added a watchdog restart rate limit: at most three automatic watchdog restarts per 30 minutes, then restarts are suppressed for 30 minutes.
- Added Admin UI and MQTT diagnostics for watchdog action, suppressed-until time and restarts in the current watchdog window.
- Added tests for aggressive watchdog config migration, CPU-only grace handling and restart-loop suppression.

## v0.1.0

- First public KioskMate release.
- Ships the Go-based kiosk supervisor with embedded Admin UI, external Chromium control and Debian packages for `arm64` and `amd64`.
- Adds Home Assistant focused kiosk page management with manual switching, rotation, time rules, page checks, render checks and session repair tools.
- Adds MQTT discovery for Home Assistant sensors, diagnostics, page controls, page health, browser controls and system actions.
- Adds Raspberry Pi oriented performance profiles, watchdog diagnostics, browser restart protection and Chromium dark rendering support.
- Adds system tools for logs, diagnostics bundle export, terminal actions, package jobs, service control and update installation.
- Adds configuration migration from older KioskMate alpha builds and keeps runtime data in `~/.config/kioskmate`.

## v0.0.1-alpha19

- Fixed kiosk display dark mode by passing Chromium dark-rendering flags when the configured kiosk theme is `dark`.
- Added test coverage so the stored kiosk theme actually affects Chromium launch arguments.
- Hardened Admin UI theme initialization so old local `light` state does not override a dark kiosk config unless the Admin theme was explicitly selected.

## v0.0.1-alpha18

- Added render health checks for kiosk pages using a short-lived headless browser screenshot.
- Added blank/white page detection to distinguish HTTP reachability from visible rendering.
- Added diagnostic bundle export with redacted config, status and logs.
- Added plain-text log download from the Logs page.
- Added MQTT discovery reset to clear known KioskMate discovery topics and republish current entities.
- Improved update install feedback by polling the install job directly in the Maintenance view.
- Updated README with HA 403, render-check, diagnostics, MQTT page-health and page-session guidance.

## v0.0.1-alpha17

- Added browser start and restart counters to Admin UI and MQTT state.
- Added richer page-check diagnostics with categories and actionable Home Assistant hints for 403, auth redirects and network errors.
- Added Home Assistant MQTT page-health entities per kiosk page: reachable, status code, last error and last checked.
- Made MQTT page-health checks round-robin so large/unreachable page lists do not block every publish cycle.
- Updated MQTT discovery page entity count feedback.
- Added tests for Home Assistant page-check hints and MQTT page-health rotation.

## v0.0.1-alpha16

- Added optional separate browser profiles per kiosk page to isolate Home Assistant sessions and cookies.
- Added Admin UI and MQTT switch support for separate page sessions.
- Made performance profiles affect Chromium flags instead of only storing a config value.
- Added `minimal` and `quality` profiles to the Admin UI and aligned UI/MQTT/profile validation.
- Added improved MQTT discovery publish feedback with discovery prefix, root topic, page count and page entity count.
- Added tests for isolated page profile paths and performance profile Chromium arguments.

## v0.0.1-alpha15

- Added watchdog diagnostics to browser status, including pressure, limits, last restart and last restart reason.
- Raised Raspberry safe-mode watchdog limits from 700 MB / 160% to 1200 MB / 220% with automatic migration for the old alpha values.
- Added a HA session repair action that resets the browser session and immediately checks the active page.
- Added watchdog diagnostics to the Dashboard and Kiosk pages.
- Added Home Assistant MQTT discovery/state for browser start time, watchdog pressure, watchdog limits, watchdog last restart/reason and page indexes.
- Added watchdog details and log paths to browser diagnostics.

## v0.0.1-alpha14

- Added a dedicated browser log file at `~/.config/kioskmate/logs/browser.log`.
- Extended Admin UI logs with selectable sources: combined, core, browser, systemd journal, service status and paths.
- Improved log fallback behavior for user sessions where `journalctl --user` has no entries.
- Added service/config/log path output to the Logs page for faster Raspberry Pi troubleshooting.
- Kept browser stdout and stderr in both the service output and browser log file.

## v0.0.1-alpha13

- Added one Home Assistant MQTT button entity per enabled kiosk page so automations can switch directly to a specific page.
- Added per-page MQTT active binary sensors plus diagnostic page name and URL sensors.
- Added MQTT command topics under `pages/<page_id>/activate` for direct page switching.
- Added stable page entity IDs with duplicate-name handling.
- Added the HA session reset action directly to the Kiosk page.
- Added Chromium feature disables for local network access checks that can interfere with local Home Assistant dashboards.
- Improved Kiosk page checks with a clear HTTP 403 Home Assistant IP-ban/session hint.

## v0.0.1-alpha12

- Added Kiosk page filtering, visible/enabled counters and clearer enabled/disabled page badges.
- Added page duplicate, move up, move down and safer page removal with workflow index remapping.
- Added bulk enable/disable for all kiosk pages.
- Added check-all-pages with per-page reachability results.
- Added page import/export as JSON for faster multi-kiosk setup.
- Added browser start, stop and restart controls directly to the Kiosk page.
- Added Scheduler status cards for mode, reason, active rule and next switch.
- Added quick workflow tools to build rotation from enabled pages, clear rotation and clear time rules.
- Added a small recent-action history on the Kiosk page so button feedback remains visible after toasts disappear.

## v0.0.1-alpha11

- Redesigned the Kiosk page around the actual operator workflow: current page, primary page actions, page list and workflow board.
- Replaced the dense page table with clearer page cards and moved the selected page controls into the main status area.
- Added expandable sidebar groups for System and Settings with focused subpages.
- Split System into Actions, Hardware, Terminal and Logs.
- Split Settings into Access, Browser and Performance, Config and Maintenance.
- Moved SSH key handling into Settings Access and kept Terminal focused on command execution.

## v0.0.1-alpha10

- Refactored the Admin UI navigation around the real operating areas: Dashboard, Kiosk, MQTT, System and Settings.
- Reworked the Kiosk page into a page and workflow control center with page activation, reachability checks, rotation durations and time rules in one place.
- Moved browser engine, browser arguments, GPU, performance profile and watchdog settings into Settings.
- Merged hardware controls, privileged actions, terminal and logs into the System page.
- Added stored NTP server and timezone settings to prepare time alignment with Home Assistant installations.
- Replaced the embedded page preview with an external-open action because Home Assistant dashboards commonly block iframe previews.

## v0.0.1-alpha1

- Started KioskMate as a clean Go-based kiosk supervisor project.
- Removed the legacy Electron application from the repository.
- Added the `kioskmate` daemon, embedded Admin UI, browser supervisor, watchdog, MQTT discovery and updater.
- Added Debian packaging for `arm64` and `amd64`.
- Added root-level CI and release workflows for `v0*` tags.
