# KioskMate on Windows

KioskMate runs on Windows 10 and 11 (x64 and ARM64) as a first, experimental port. It starts Microsoft Edge, or Google Chrome when Edge is missing, full screen and controls it over the Chrome DevTools Protocol exactly like Chromium on Linux. The Admin UI, page rotation, schedules, workflows, MQTT and Home Assistant integration work the same way.

Linux remains the main platform. The Windows port has been built and unit-tested on Linux only; please report what works and what does not.

## Install

1. Download `kioskmate_<version>_windows_amd64.zip` (or `_arm64.zip`) from the [releases](https://github.com/MickLesk/KioskMate/releases) and extract it.
2. Open PowerShell in the extracted folder as the user that should show the kiosk and run:

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\install.ps1 -Start
   ```

   The script copies `kioskmate.exe` to `%LOCALAPPDATA%\Programs\KioskMate`, removes the downloaded-file mark and registers the scheduled task **KioskMate** that starts KioskMate hidden 15 seconds after this user logs on. If KioskMate ends with an error, the task starts it again after 10 seconds.

   If registering the task reports *access denied*, run the same command from an elevated PowerShell (*Run as administrator*). The task still runs as your user, not as administrator.

3. Read the setup token and open the Admin UI:

   ```powershell
   & "$env:LOCALAPPDATA\Programs\KioskMate\kioskmate.exe" --admin-info
   ```

   `http://localhost:33333`, or `http://<pc-ip>:33333` from another device.

To reach the Admin UI from other devices, allow it through Windows Defender Firewall. Either accept the prompt Windows shows on the first start, or run from an elevated PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\install.ps1 -FirewallRule
```

This adds an inbound rule for `kioskmate.exe` on private and domain networks.

Run `install.ps1` again to install a newer version. Remove KioskMate with `install.ps1 -Uninstall`; configuration and browser profiles are kept.

For a real kiosk, also enable automatic sign-in for the kiosk user (for example with Sysinternals Autologon), and turn off sleep, screen saver and Windows Update restarts during opening hours.

## Files

| What | Where |
| --- | --- |
| Program | `%LOCALAPPDATA%\Programs\KioskMate\kioskmate.exe` |
| Configuration | `%APPDATA%\KioskMate\config.json` |
| Logs, backups, event journal | `%APPDATA%\KioskMate\` |
| Browser profiles | `%LOCALAPPDATA%\KioskMate\Browser` |

`--config <path>` selects another configuration file, as on Linux.

## Browser

Without a configured browser command KioskMate uses the first browser it finds, in this order:

1. `msedge.exe` under `Program Files (x86)`, `Program Files` or `%LOCALAPPDATA%` (`Microsoft\Edge\Application`),
2. `chrome.exe` in the same folders (`Google\Chrome\Application`).

Set **Browser command** in the Admin UI settings (`kiosk.browser_command`) to a full path to use another Chromium-based browser. Linux-only switches (`--ozone-*`, Wayland features) are left out on Windows, also when they come from extra arguments in a configuration copied from a Linux kiosk.

Each browser KioskMate starts runs in a Windows job object together with all its helper processes. Stopping or restarting first asks the browser to close its window, then ends whatever is left after three seconds. When KioskMate itself ends, Windows closes the browser as well, so no stray process keeps the profile locked.

Do not open the KioskMate browser profile in a normal Edge or Chrome window while KioskMate runs: the browser would hand the kiosk start over to that window and exit.

## What works and what does not

| Feature | Windows |
| --- | --- |
| Browser start, stop, restart, reload, page switching, tabs, screenshots | Supported (Edge, Chrome) |
| Admin UI, MQTT, Home Assistant discovery, workflows | Supported |
| Display brightness | Supported where Windows exposes `WmiMonitorBrightnessMethods`, usually the built-in panel of laptops and tablets. External monitors are normally not controllable. |
| Battery level, memory, uptime, disk usage, device model and serial | Reported |
| Display power (on/off), schedule display-off | Unsupported |
| Audio and microphone volume | Unsupported |
| On-screen keyboard | Unsupported |
| CPU load and temperature, illuminance | Unsupported |
| Watchdog memory and CPU limits | Not active: process statistics are not read on Windows yet |
| apt jobs, service restart, reboot, shutdown, time and NTP settings, sudo/su privileges | Unsupported, reported as "unsupported on Windows" |
| Self-update | Unsupported; install a new zip with `install.ps1` |

Unsupported controls report an error instead of pretending to work, and the hardware status shows them as unsupported, so Home Assistant only gets entities that work.

## Build from source

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -o kioskmate.exe ./cmd/kioskmate
```
