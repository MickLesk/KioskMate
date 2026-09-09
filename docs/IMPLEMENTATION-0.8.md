# Implementation tracker: 0.8.0

Branch: `dev-kiosk-0.8`. Baseline: `89ba3c1` (0.7.7).

| Work package | State | Verification |
| --- | --- | --- |
| Config snapshots, migrations and stable identities | Complete | Deep-copy and stable profile migration tests pass |
| Browser lifecycle, recovery and CDP | In progress | Serialized operation, CDP multiplexing and recovery tests pass locally |
| HA authentication classification and request protection | In progress | Structured token/WebSocket/resource classification tests pass |
| Admin security and operation feedback | In progress | Hashed session, CSRF and operation history tests pass |
| Frontend modules, AIO workflows and translations | In progress | API transport module and dirty-form refresh guard added |
| MQTT transport, diagnostics and HA entities | In progress | TLS/mTLS/SNI, packet-bound and capability-aware discovery tests pass |
| Privileges, updater and Debian packaging | In progress | Installer rewrite removed; package verifier added |
| Event journal, performance and release automation | In progress | Persistent event journal, bounded retention, sensitive-field filtering and release gates pass; performance soak pending |
| Browser E2E and Linux integration | Pending | Pending |
| Raspberry Pi 24-hour soak | Requires target device | Not run |

Only mark a work package complete after its behavior has been verified. Hardware-dependent results must remain separate from local fixture results.
