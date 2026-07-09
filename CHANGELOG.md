# Changelog

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
