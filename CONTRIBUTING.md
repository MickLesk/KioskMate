# Contributing

## Layout

* `cmd/kioskmate` – main program, flags, systemd notify/watchdog.
* `internal/supervisor` – Chromium process, DevTools control, scheduler,
  tabs, telemetry, recovery.
* `internal/admin` – HTTP server, API, local pages; the Admin UI lives in
  `internal/admin/web` (plain ES modules, no build step).
* `internal/integration` – MQTT and Home Assistant discovery.
* `internal/hardware`, `internal/system` – device information and controls.
* `internal/config` – configuration, validation, migration.
* `image/` – Raspberry Pi OS image (pi-gen stage), `packaging/` – deb and
  Windows files, `scripts/` – packaging and checks.

## Development

```bash
go run ./cmd/kioskmate -config /tmp/kioskmate-dev/config.json
```

The Admin UI is embedded: after changing files under `internal/admin/web`,
rebuild or restart `go run`.

## Checks (same as CI)

```bash
gofmt -l .                       # must print nothing
go vet ./... && GOOS=windows go vet ./...
go test -race ./...
for f in internal/admin/web/assets/*.js; do node --check "$f"; done
node scripts/check-i18n.mjs      # German and English keys must match
git ls-files '*.sh' | xargs shellcheck -S warning
bash tests/image/firstboot-test.sh
bash scripts/check-changelog.sh
```

### Browser tests

```bash
npm install --prefix tests/e2e
npx --prefix tests/e2e playwright install chromium
go build -o /tmp/kioskmate ./cmd/kioskmate
node tests/e2e/run.mjs /tmp/kioskmate /tmp/e2e-shots
E2E_ONLY=pre4 node tests/e2e/run.mjs /tmp/kioskmate /tmp/e2e-shots   # one suite
```

CI also runs the suite against the minified UI
(`go run ./tools/minify && go build -tags minified …`).

## Conventions

* Every user-visible text goes through `t("key")` with a German and an English
  entry.
* The Admin UI has a strict Content Security Policy: no inline styles or
  scripts; set sizes through `data-width`/`data-left` attributes.
* Mind the Raspberry Pi: no polling loops faster than needed, no frequent
  writes to the SD card.
* Add a line to `CHANGELOG.md` under the upcoming version.
