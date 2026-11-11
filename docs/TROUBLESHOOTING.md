# Troubleshooting

Start with **Device → Browser Doctor**: it checks the browser, power supply,
touch input, video decoding, swap, disk space, time zone and the pages, and
"Copy report" puts everything into the clipboard for an issue.

From a shell: `kioskmate --doctor` and `kioskmate --admin-info`.

## Touchscreen does not react

1. Browser Doctor → *Touch input*. "Missing" means the device was there
   before and is gone now.
2. `vcgencmd get_throttled` – anything but `0x0` means undervoltage or
   throttling. `dmesg | grep -i voltage` shows "Undervoltage detected".
3. `lsusb` should list the touch controller. If it does not, use a proper
   power supply and re-plug the USB cable. See [HARDWARE.md](HARDWARE.md).

## White flicker or white page

* Short white flash when an insert appears: fixed in 1.0.0-pre.3 (pages load
  in the background and swap in when ready).
* White page with HTTP checks OK: **Overview → Snapshot** shows what the
  display really renders. Use **Pages → Check** on the page.
* Dark theme: KioskMate emulates `prefers-color-scheme`; use `force-dark` only
  for pages that ignore it.

## Home Assistant: 403, login loop, IP ban

KioskMate stops the browser after a 403 to avoid getting banned. Remove the
kiosk's IP from `ip_bans.yaml`, restart Home Assistant, then use **Reset HA
session** once and log in on the kiosk. Details in the README.

## High CPU

1. Device → Display → Runtime telemetry: CPU split into pages, graphics,
   browser and KioskMate, and CPU per page.
2. Pages with cameras are the usual cause: use substreams or MSE, see
   [HARDWARE.md](HARDWARE.md#cameras).
3. Browser Doctor → *Video decoding*: `cma_alloc failed` means too little
   video memory.
4. Rendering in software: the Doctor offers "Test hardware mode".

## Page does not load / no network

KioskMate shows an offline page with clock and countdown and opens the page
again as soon as it answers. If it stays there, check the address from the
kiosk: Pages → Check, or `curl -I <url>` on the device.

## Wrong time after boot

The Pi has no real-time clock. KioskMate waits up to two minutes for NTP
before it applies the schedule. Check Device → Time (synchronised, time zone)
and `timedatectl`.

## The Admin UI says "Session expired"

The kiosk restarted with new session keys or you signed out elsewhere. Sign
in again. "Connection lost" means the kiosk is not reachable (restart, Wi-Fi);
the page reconnects by itself.

## KioskMate restarts by itself

The systemd service has a watchdog: if KioskMate stops responding for two
minutes, systemd restarts it. `journalctl --user -u kioskmate` shows why.
A crash inside one part (MQTT, schedule, workflows, updater) restarts only
that part; the log has the stack trace.

## Logs

* Admin UI: Device → Logs (core, browser, event journal, systemd).
* Shell: `journalctl --user -u kioskmate -f` (desktop) or
  `journalctl -u kioskmate-cage@kiosk -f` (Cage/image).
* Diagnostic bundle (ZIP, secrets redacted) under Device → Logs.

The KioskMate image keeps the journal in RAM; set `PERSISTENT_LOGS=yes` in
`kioskmate.txt` (or delete `/etc/systemd/journald.conf.d/50-kioskmate.conf`)
to keep logs across reboots.
