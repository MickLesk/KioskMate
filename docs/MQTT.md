# MQTT and Home Assistant

KioskMate connects to an MQTT broker, publishes its state and announces its
entities through [Home Assistant MQTT discovery](https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery).
Configure it under **Automations → MQTT** or in `config.json`:

```json
"mqtt": {
  "enabled": true,
  "url": "mqtt://homeassistant.local:1883",
  "version": "3.1.1",
  "username": "kiosk",
  "password": "secret",
  "discovery": "homeassistant",
  "node": "kioskmate",
  "interval": 30000000000
}
```

`mqtts://` connects with TLS. Certificate problems are shown in plain words in
the Admin UI (unknown certificate authority, wrong host name, expired
certificate). After a lost connection KioskMate reconnects with growing pauses
(5 s doubling up to 2 min, with random jitter so several kiosks do not hit the
broker at the same moment).

## Topics

All topics start with `<base>/<node>`, by default `kioskmate/kioskmate`
(`mqtt.base_topic`, `mqtt.node`). Below, `…` stands for that prefix.

### State

| Topic | Content |
| --- | --- |
| `…/availability` | `online` / `offline` (last will) |
| `…/state` | JSON with browser state, active page, scheduler, watchdog, recovery, telemetry and override |
| `…/browser/state` | `ON` while the display browser runs |
| `…/cpu/state`, `…/rss/state` | Browser CPU (%) and memory (MB) |
| `…/page_cpu/state` | Measured browser CPU of the page on screen (%); attributes in `…/page_cpu/attributes` name the page and the most expensive page |
| `…/processor_usage/state`, `…/processor_temperature/state`, `…/memory_usage/state`, `…/disk_usage/state`, `…/up_time/state` | System values |
| `…/display/power/state`, `…/display/brightness/state`, `…/volume/state` | Hardware, where supported |
| `…/pages/<page id>/active/state`, `…/pages/<page id>/reachable/state` | Per page |
| `…/version/state`, `…/update/…` | Installed and latest version, update state |

Sensors with a unit carry `state_class: measurement` and, where it applies, a
`device_class` (temperature, duration, data size, battery, illuminance), so
Home Assistant keeps long-term statistics for them.

### Commands

| Topic | Payload |
| --- | --- |
| `…/command` | `start`, `stop`, `restart`, `refresh`, `next`, `previous`, `reboot`, `shutdown`, `apt-update`, `apt-upgrade`, `restart-service` |
| `…/<action>/execute` | Same actions as buttons: `start`, `stop`, `restart`, `refresh`, `next`, `previous`, `reset_session`, `reboot`, `shutdown`, `restart_service`, `apt_update`, `apt_upgrade` |
| `…/browser/set` | `ON` / `OFF` |
| `…/page_number/set` | 1-based page number |
| `…/page_name/set` | Page name |
| `…/page_url/set` | `http(s)://` address for the current page |
| `…/pages/<page id>/activate` | Any payload: show that page |
| `…/override/duration/set` then `…/override/page/set` | Show a page for N minutes (1-1440), then return to the programme; `…/override/clear` ends it |
| `…/inserts/<insert id>/set` | `ON` / `OFF`: enable or disable an insert |
| `…/presence/set` | Anything except `OFF`, `false`, `0`, `clear`, `no`, `not_home`, `idle` marks someone as present (presence mode) |
| `…/display/power/set`, `…/volume/set`, `…/microphone/set`, `…/keyboard/set` | Hardware controls |
| `…/scheduler_enabled/set`, `…/scheduler_mode/set`, `…/scheduler_tick/set`, `…/watchdog_enabled/set` | Programme and watchdog |
| `…/performance_profile/set`, `…/gpu_mode/set`, `…/kiosk_theme/set`, `…/reduce_motion/set`, `…/isolate_page_sessions/set` | Display settings (browser restarts) |
| `…/update/check`, `…/update/install`, `…/update/rollback` | Updater |

Workflows can also react to any topic of your own (trigger type `mqtt`), see
the README.

## Discovery

KioskMate publishes discovery configs under `<discovery prefix>/<component>/<node>/<id>/config`.
After page renames or deleted pages, use **Automations → MQTT → Reset
discovery**: it clears every topic KioskMate knows and republishes the current
set.
