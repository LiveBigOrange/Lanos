# Lanos

> Secure peer-to-peer file transfer over local network — encrypted, fast, cross-platform.
> 局域网跨平台文件传输工具，原生支持 IPv4/IPv6 双栈。

---

## Features

- **End-to-end encrypted** — Noise XX handshake with ed25519 identity
- **Cross-platform** — Windows, macOS, Linux (Android/iOS planned)
- **Dual-stack** — Native IPv4/IPv6 support with RFC 6724 address selection
- **Web sharing** — Share files via browser with password protection & QR code
- **mDNS discovery** — Auto-detect peers on the same LAN
- **Async transfers** — Non-blocking send/receive with progress tracking

---

## Project Structure

```
lanos/
├── core/                  # Go daemon (gcd) — desktop process + gomobile shared
│   ├── cmd/gcd/           # Desktop entrypoint
│   ├── identity/          # ed25519 key management
│   ├── config/            # config.yaml read/write
│   ├── instance/          # Cross-platform single-instance lock
│   ├── lifecycle/         # Startup handshake + random port + API token
│   ├── discovery/         # mDNS broadcast & browse
│   ├── api/               # Local REST API + Bearer auth + CORS + SSE
│   ├── store/             # SQLite (transfer_log.db)
│   ├── transport/         # Noise XX + SAS + frame mux
│   ├── transfer/          # Chunk + queue + state machine + cancel
│   ├── receive/           # Inbound P2P server + manager
│   ├── share/             # HTTP web share (ZIP streaming, IP ban)
│   └── net/               # TCP dial/listen, connect-URI, address selector
├── mobile/bind/           # gomobile-bind facade (Android AAR / iOS XCFramework)
├── ui/                    # Flutter desktop + mobile UI
│   ├── lib/
│   │   ├── main.dart
│   │   ├── pages/
│   │   ├── services/
│   │   └── widgets/
│   └── test/
├── docs/
│   ├── PROTOCOL.md        # mDNS / Noise / frame format / error codes
│   └── NETWORK.md         # IPv4/IPv6 / Avahi troubleshooting
├── scripts/
│   ├── build/             # Per-OS packaging scripts
│   └── lanos-setup-firewall.*
└── .github/workflows/     # CI + Release
```

---

## Getting Started

### Prerequisites

- Go 1.22+
- Flutter 3.27+ (stable)

### Go Core

```bash
cd core
go mod tidy
go build ./cmd/gcd
go test -race ./...
./gcd    # prints handshake JSON to stdout
```

Handshake output:

```json
{"port":52100,"api_token":"nc5AkzhR9mNRc8S8ajAs9sOz0r_0rQWnZrNIaxDyLL8","version":"0.1.0"}
```

Verify with curl:

```bash
curl http://127.0.0.1:52100/api/v1/ping                              # {"ok":true}
curl -H "Authorization: Bearer <token>" http://127.0.0.1:52100/api/v1/version
curl -H "Authorization: Bearer <token>" http://127.0.0.1:52100/api/v1/devices
```

### Flutter UI

```bash
cd ui
flutter pub get
flutter analyze --fatal-infos
flutter test
flutter run -d windows   # or macos / linux
```

> The Flutter app spawns `gcd` as a subprocess. Ensure `gcd` is in PATH or set `LANOS_GCD_PATH`.

---

## API Overview

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/ping` | No auth |
| GET | `/api/v1/devices` | Self + peers from mDNS |
| GET | `/api/v1/events` | SSE stream (device presence) |
| POST | `/api/v1/transfers` | Body: `{"peer_id","file_path"}` |
| GET | `/api/v1/transfers/{id}` | Transfer detail |
| POST | `/api/v1/transfers/{id}/cancel` | Cancel transfer |
| GET/POST/DELETE | `/api/v1/shares[/{id}]` | Web share CRUD |
| GET | `/api/v1/incoming` | Pending incoming prompts |
| POST | `/api/v1/incoming/{id}/accept` | Accept incoming |
| POST | `/api/v1/incoming/{id}/reject` | Reject incoming |

All endpoints (except `/ping`) require `Authorization: Bearer <token>`.

---

## License

MIT
