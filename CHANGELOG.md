# Changelog

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
