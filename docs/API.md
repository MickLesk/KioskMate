# Admin HTTP API

The Admin UI talks to KioskMate through a JSON API on the admin port (default
`33333`). Everything the UI does is available to scripts.

## Authentication

* **Token**: send the admin token (`admin.token` in `config.json`, shown by
  `kioskmate --admin-info`) as `Authorization: Bearer <token>` or
  `X-KioskMate-Token: <token>`. Token requests need no CSRF header.
* **Session**: the Admin UI signs in with `POST /api/auth/login` and gets a
  cookie. State-changing session requests must come from the same origin and
  carry the `X-KioskMate-CSRF` header from `/api/auth/status`.

Request bodies are limited to 64 KB for login, setup and webhook triggers and
to 8 MB for the rest of the API. Too many wrong passwords or tokens answer
`429` with `Retry-After`.

```bash
TOKEN=...   # kioskmate --admin-info
curl -H "Authorization: Bearer $TOKEN" http://kiosk.local:33333/api/status
curl -X POST -H "Authorization: Bearer $TOKEN" http://kiosk.local:33333/api/browser/next
```

## Without login

| Method | Path | |
| --- | --- | --- |
| GET | `/healthz` | `status`, `version`, `uptime_seconds`, browser state |
| GET/POST | `/api/auth/status`, `/api/auth/setup`, `/api/auth/login`, `/api/auth/logout` | Sign-in |
| POST | `/api/rules/<id>/trigger?token=<rule token>` | Fire a webhook workflow (token per rule) |
| GET | `/local/<page id>` | Local pages; only for the device itself or a signed-in admin |

## Status and diagnostics

| Method | Path | |
| --- | --- | --- |
| GET | `/api/status` | Everything the overview shows |
| GET | `/api/stream` | Server-sent events with live status |
| GET | `/api/hardware` | Device, system, display, audio, network |
| GET | `/api/time`, `/api/time/zones` | Clock, NTP, time zones |
| GET | `/api/program/timeline` | Programme for the next hours, sun times |
| GET/POST | `/api/presence` | Presence state; POST marks someone as present |
| GET | `/api/browser/doctor` | Browser Doctor checks and advice |
| GET/DELETE | `/api/browser/telemetry` | CPU/RAM history, CPU per page; DELETE resets |
| GET | `/api/browser/operations`, `/api/browser/diagnostics`, `/api/browser/recovery` | Browser internals |
| GET | `/api/browser/profile-recommendation` | Suggested performance profile |
| GET | `/api/logs`, `/api/logs/download`, `/api/events` | Logs and event journal |
| GET | `/api/diagnostics/export` | ZIP with redacted config, status and logs |
| GET | `/api/pages/thumbnails/<page id>` | Page thumbnail (PNG) |

## Display control

| Method | Path | Body |
| --- | --- | --- |
| POST | `/api/browser/start`, `stop`, `restart`, `reload`, `next`, `previous`, `reset-session` | – |
| POST | `/api/browser/page` | `{"index": 0}` (enabled pages, 0-based) |
| POST/DELETE | `/api/browser/override` | `{"page": 0, "duration_seconds": 600}`; DELETE returns to the programme |
| POST | `/api/browser/check-page` | `{"index": 0}` or `{"url": "https://…"}` |
| POST | `/api/browser/render-check` | Render check of a page |
| GET | `/api/browser/snapshot` | Screenshot of the real display |
| POST | `/api/browser/recover` | Run the recovery ladder |
| POST | `/api/browser/safe-mode` | `{"restart": true}` |
| POST | `/api/browser/gpu-mode` | `{"mode": "auto"}` |
| POST | `/api/hardware/<display|brightness|volume|microphone|keyboard>` | `{"value": …}` |

## Configuration

| Method | Path | |
| --- | --- | --- |
| GET/POST | `/api/config` | Read (secrets redacted) / save. Send the `revision` you read; a newer config on the kiosk answers `409` |
| GET | `/api/config/export` | Download |
| POST | `/api/config/import` | Upload, with a preview of the changed areas |
| GET | `/api/config/backups` | List backups |
| POST | `/api/config/restore` | Restore a backup |
| POST | `/api/mqtt/test`, `/api/mqtt/test/live`, `/api/mqtt/discovery`, `/api/mqtt/discovery-reset` | MQTT |

## System and updates

| Method | Path | |
| --- | --- | --- |
| POST | `/api/system/<reboot|shutdown|restart-service|apt-update|apt-upgrade>` | Privileged jobs (`{"mode","password","remember"}` when sudo needs a password) |
| GET | `/api/jobs`, `/api/jobs/<id>` | Job output |
| POST | `/api/terminal/run` | One command; logged in the event journal with program, exit code and duration |
| GET/POST | `/api/update`, `/api/update/channel`, `/api/update/preflight`, `/api/update/install`, `/api/update/rollback`, `/api/update/history`, `/api/update/jobs/<id>` | Updater |
| POST | `/api/repair` | Repair config and user service defaults |
| GET/POST | `/api/auth/sessions`, `/api/auth/logout-all`, `/api/auth/password` | Sessions and password |
