# Changelog

## v0.0.1-alpha.3

- Rebuilt the Debian packages with valid control metadata for strict `dpkg` parsing.
- Added a portable Debian package builder to avoid malformed release assets on non-Linux build hosts.

## v0.0.1-alpha.2

- Finalized the project rename to KioskMate for the `MickLesk/KioskMate` repository.
- Added migration from previous alpha config paths into `~/.config/kioskmate/config.json`.
- Added backup handling for previous alpha configs during Debian package upgrades.
- Added Home Assistant MQTT discovery cleanup for previous alpha entities.

## v0.0.1-alpha.1

- Rebranded the Go supervisor package, binary, service, config paths and Home Assistant MQTT discovery to KioskMate.
- Added Home Assistant MQTT discovery cleanup for retained legacy entities so stale firmware/device metadata is removed automatically.
- Changed the Home Assistant device identifiers to create a clean KioskMate device instead of merging with older kiosk devices.
- Added MQTT controls for service restart and KioskMate update installation.
- Added MQTT update entity discovery and update version state topics.
- Improved Admin UI language switching, action toasts and button busy feedback.
- Added migration from the previous v2 config path into `~/.config/kioskmate/config.json`.

