# Lanos - Agent guide

## Structure

Monorepo: `core/` (Go daemon gcd) + `ui/` (Flutter desktop/mobile) + `mobile/bind/` (gomobile facade for Android/iOS). No vendored deps.

Entrypoints:
- `core/cmd/gcd/main.go` — startup: instance lock → config/identity → SQLite → API token → port bind → mDNS → inbound P2P (`receive.NewInboundServer` on `:Port+2`) → web share → API server → stdout handshake
- `ui/lib/main.dart` — spawns gcd subprocess, reads one JSON line from stdout
- `mobile/bind/bind.go` — gomobile-bind facade for Android/iOS; exposes `Bridge.SendFile`/`ReceiveFile`/`ParseConnectURI`/`SelectBestAddress` over `usecase.Sender`/`usecase.Receiver` interfaces with hex-encoded static keys

Module paths: `github.com/lanos/lanos/core` (Go 1.22, no CGO) and `github.com/lanos/lanos/mobile` (separate `go.mod` with `replace github.com/lanos/lanos/core => ../core`). CI uses go 1.22 + flutter stable.

## Commands (workdir: core/ or ui/)

```bash
# Go core
go vet ./...
go test -race -timeout 120s ./...           # includes integration/ pkg
go test -race -count=1 ./net/               # single pkg, bypass cache
gofmt -l .                                  # CI fails on unformatted files
go build ./cmd/gcd

# Flutter UI
flutter pub get
dart format --set-exit-if-changed lib test integration_test
flutter analyze --fatal-infos
flutter test
flutter gen-l10n                              # after editing .arb in lib/l10n/
```

CI order (`.github/workflows/ci.yml`, 3‑OS matrix):
  Go: `gofmt` → `go vet` → `go test -race` → `golangci-lint` → `go build ./cmd/gcd`
  Flutter: `dart format` → `flutter analyze --fatal-infos` → `flutter test`

No Flutter SDK or golangci-lint in local dev environment — CI validates both.

## Architecture

- **Ports**: Random pick 52100–52999, persisted in `config.yaml`. API binds `127.0.0.1:Port` (loopback). Web share on `0.0.0.0:Port+1`. Inbound P2P on `0.0.0.0:Port+2` via `receive.InboundServer` (`core/receive/server.go`).
- **Noise**: Handshake uses **XX** pattern (not XK as PROTOCOL.md §3 says). ed25519 identity → X25519 via `transport.DeriveStaticKeys` (libsodium-style clamp).
- **Auth**: In-memory random Bearer token at startup, emitted via stdout JSON. All API calls carry `Authorization: Bearer <token>` (except `/api/v1/ping`).
- **Config.Apply**: maps are keyed by yaml tag, applied via reflection (float64→int conversion), then `Save()`. `Config.mu` is internal — use `Apply()` or `SetDeviceName()` for concurrent updates.
- **Transport layering** (bottom → top):
  - `core/net/dial.go` / `listen.go` — Noise XX handshake, 4B BE length-prefixed ciphertext frames
  - `core/transport/frame.go` — 16B frame header (stream_id + frame_type + reserved + payload_len + crc32), 4MB max payload
  - `core/transport/stream.go` — `Mux` stream mux (even=initiator, odd=responder)
  - `core/transfer/chunk.go` — 4MB chunk + seq + SHA256
  - `core/usecase/send.go` + `receive.go` — custom `LANOS_FILEv1\n{id}\n{size}\n{name}\n` text header over bare `EncryptedConn` (bypasses Mux/Frame/Chunk)
- **Async send**: `POST /transfers` returns 201 immediately, runs `SendFileUseCase.Execute` in goroutine. UI polls `GET /transfers/{id}`.
- **Async receive**: `InboundServer` accepts TCP + Noise handshake, reads header, registers with `receive.Manager`. `handleAcceptIncoming` sets status → `InboundServer.waitForAccept` polls → saves file. All async.
- **SSE**: Go side `/events` (100ms throttle, device-only events). Flutter `SseClient` (`sse_client.dart`) streams device presence; `DeviceService` hybrid SSE + 5s poll fallback. `TransferService` and `IncomingService` still poll (2s / 3s).
- **Cancel wiring**: `transfer.CancelRegistry` + context cancellation. `SendFileUseCase` registers with `CancelReg`; `Manager.Cancel` calls `CancelRegistry.Cancel` → cancels ctx → `streamFile` checks `ctx.Done()` per iteration.
- **State machines**: `transfer.StateMachine` (7 states: pending/connecting/transferring/completed/cancelled/failed/awaiting_resume) validates transitions. `receive.Manager` has 9 states (pending/prompting/accepting/receiving/completed/failed/rejected/cancelled/expired), 30s expiry.
- **Network** (`core/net/`): `"tcp"` network means dual-stack when addr has no explicit IP (`:port`). `NewListener` returns `AcceptResult{Conn, PeerStatic}`. Integration tests use `"127.0.0.1:0"` (v4 loopback).
- **Connect URIs**: `net/uri.go` parses `lanos://connect?ip=...&ip6=...&port=...&pk-hash=...&device-name=...` — dual-stack addressing for QR handoff. `ip` is IPv4-only, `ip6` is IPv6-only; link-local `ip6` (`fe80::/10`) MUST carry a zone id. Zone `%` MUST be percent-encoded as `%25` in the URI (raw `%wlan0` is rejected by `url.ParseQuery`).
- **Address selection**: `net/addrselect.go` implements RFC 6724 destination/source selection (`SelectAddresses(dsts, sources, port) []AddrPair`, best-first). Source selection enforces same IP version; link-local dst requires link-local src. `core/api/handlers.go` `peerAddress()` wraps this and surfaces `ErrIncompatibleIPVersion` (`INCOMPATIBLE_IP_VERSION`) at HTTP 503, `ErrNoPeerAddress` (`PEER_UNREACHABLE`) at 503, both via the `writeErrorCode(w, status, code, msg)` shape `{"error":{"code":...,"message":...}}` (distinct from `writeError`'s StatusText body). DSL `mobile/bind.SelectBestAddress` is a CSV adapter.

## API routes (chi router, 127.0.0.1 only, bearer auth)

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/ping` | no auth |
| GET | `/api/v1/version` | |
| GET | `/api/v1/devices` | `self` + `peers` from mDNS |
| GET | `/api/v1/diagnostics` | local IP version, interfaces, source IPs (auth required) |
| GET | `/api/v1/events` | SSE stream (device events only) |
| GET | `/api/v1/settings` | |
| POST | `/api/v1/settings` | also accepts PUT |
| GET/POST/DELETE | `/api/v1/shares[/{id}]` | web share CRUD |
| GET | `/api/v1/shares/history` | |
| GET | `/api/v1/shares/export` | CSV |
| GET | `/api/v1/transfers` | transfer log |
| POST | `/api/v1/transfers` | body: `{"peer_id","file_path"}` |
| GET | `/api/v1/transfers/export` | CSV |
| GET | `/api/v1/transfers/{id}` | detail |
| POST | `/api/v1/transfers/{id}/cancel` | |
| DELETE | `/api/v1/transfers/{id}` | |
| GET | `/api/v1/incoming` | pending prompts |
| POST | `/api/v1/incoming/{id}/accept` | body: `{"save_path"}` |
| POST | `/api/v1/incoming/{id}/reject` | |
| POST | `/api/v1/incoming/{id}/cancel` | |

Routes use chi — URL params via `chi.URLParam(r, "id")`.

## Package map

| Path | Purpose |
|------|---------|
| `core/identity/` | ed25519 keygen + OS keystore (DPAPI/Keychain/flock-0600) |
| `core/config/` | `config.yaml` via yaml.v3; `Apply()` for bulk update |
| `core/instance/` | single-instance lock (flock / LockFileEx) |
| `core/discovery/` | mDNS via grandcat/zeroconf; also implements `EventSource` for SSE |
| `core/api/` | chi router, bearer middleware, CORS, SSE broker, all RPC handlers |
| `core/lifecycle/` | startup handshake (stdout JSON), random port bind |
| `core/store/` | SQLite (modernc.org/sqlite, no CGO); CRUD + CSV export |
| `core/transport/` | Noise XX, SAS code, 16B frame, `Mux` stream mux |
| `core/net/` | TCP dial/listen, `EncryptedConn`, connect-URI parser, address selector |
| `core/transfer/` | outgoing: `Manager`, `StateMachine`, `Chunk`, `Queue` (4/device), `CancelRegistry`, `path` (Win-safe) |
| `core/receive/` | incoming: `Manager` (30s expiry, 9 states), `InboundServer` (P2P listener) |
| `core/usecase/` | `SendFileUseCase` + `ReceiveFileUseCase` (bare TCP, text-header protocol); `interfaces.go` defines `Sender`/`Receiver` consumed by `mobile/bind` |
| `core/integration/` | E2E tests: dual-instance XX handshake + file transfer on 127.0.0.1 (v4 loopback + v6 `[::1]` + addrselect cases) |
| `core/share/` | HTTP web share: password form, ZIP streaming, IP ban, QR |
| `mobile/bind/` | gomobile facade: `Bridge.SendFile`/`ReceiveFile`/`ParseConnectURI`/`SelectBestAddress` (separate `go.mod`, `replace ../core`) |
| `scripts/build/` | Per-OS packaging: `package-linux.sh` (AppImage/deb/rpm), `package-macos.sh`, `package-windows.ps1` |
| `scripts/lanos-setup-firewall-*` | PF (macOS) / PowerShell (Windows) firewall rules for TCP 52100-52999 + mDNS UDP 5353 (v4+v6) |
| `ui/lib/services/` | ApiClient, SseClient, DeviceService (SSE+poll), TransferService (2s poll), IncomingService (3s poll), NotificationService, BatchedNotifier, LifecycleControllerDesktop, ShareHistoryService |
| `ui/lib/utils/` | `format` (bytes/speed/duration), `transfer_stats` (speed/ETA) |
| `ui/lib/pages/` | HomePage, TransferPage, ReceivePage, SettingsPage, RecordsPage, OnboardingPage |
| `ui/lib/widgets/` | TransferProgressCard, DeviceCard, SasConfirmDialog |
| `ui/lib/l10n/` | ARB files (`app_en.arb`, `app_zh.arb`); run `flutter gen-l10n` after edits |

## Conventions & gotchas

- **Go**: golangci-lint enables errcheck/revive/gosec/gosimple/govet/ineffassign/staticcheck/typecheck/unused/misspell. Gosec excludes G104. All errors must be handled.
- **Flutter**: `require_trailing_commas: true`, `avoid_print: true`, `prefer_const_constructors: true` (analysis_options.yaml). `flutter analyze --fatal-infos` treats info-level as errors.
- **Tests**: `t.Helper()` for helpers; race-detector always on; `t.Parallel()` where safe. `integration_test/` is empty — E2E in `core/integration/`.
- **Import collision**: `core/net` (lanos) shadows stdlib `net` inside that package — aliased as `stdnet`. Packages importing `core/net` alias it as `corenet` to avoid collision.
- **ApiClient**: only has `get`/`post`/`delete`. No `put`. Settings page sends POST; server accepts both PUT and POST on `/settings`.
- **LifecycleController**: resolves gcd path by checking `LANOS_GCD_PATH` env var, then CWD, then `/usr/local/bin`, `/opt/homebrew/bin`, `/usr/bin`, then PATH.
- **ARB edits**: After adding/changing keys in `app_en.arb` / `app_zh.arb`, run `flutter gen-l10n`. Generated code is at `lib/l10n/app_localizations*.dart`. Formatter and analyzer run against generated output.
- **`net/uri.go`**: syntax errors there break all Go builds/tests that transitively import `core/net`. If Go fails, check this file first.
- **Avahi build tags**: `core/net/avahi_linux.go` (`CheckAvahi` requires `avahi-daemon` running) + `avahi_stub.go` (no-op on non-Linux). Linux mDNS discovery needs `sudo systemctl start avahi-daemon`.
- **Mobile module**: `mobile/` has its own `go.mod`. Test with `go test ./...` from `mobile/` (after `go mod tidy`); no NDK required for non-bind build/test.
- **Error response shapes**: API has two distinct error shapes — `writeError` (plain `StatusText` body) and `writeErrorCode` (`{"error":{"code","message"}}`). Address-selection failures use `writeErrorCode` with `INCOMPATIBLE_IP_VERSION` or `PEER_UNREACHABLE`.
- **Async handler goroutines MUST use `s.appCtx`, not `r.Context()`** (`api/server.go`). `r.Context()` is canceled the instant the handler returns (immediately after `writeJSON(201)`), which aborts any background work. `Server.Serve(ctx)` stores `ctx` as `s.appCtx`; handlers falling back to `context.Background()` only happen outside `Serve` (e.g. tests). `handleCreateTransfer` is the canonical example.
- **`share.Manager.Stop()` must run on shutdown** (`main.go` has `defer shareMgr.Stop()`). `NewManager` spawns a `cleanupLoop` goroutine; without `Stop()` you leak it (breaks tests). `Stop()` does NOT touch active shares — call `StopAll()` separately if needed.
- **`discovery.Discovery.Stop()` `wg.Wait()`s worker goroutines** (consumeEntries + runReaper + newProber.run) before `close(d.events)`. After Stop, the `Events()` channel is closed and any receive will yield `{_, false}`. Do not call `emit()` after `Stop()`.
- **`transfer.Manager.ActiveCount` and the Create-side active counter MUST agree** — both count `StatusPending|StatusConnecting|StatusTransferring` (NOT `StatusAwaitingResume`, which is a quiescent paused state that doesn't hold a connection slot). If you add a new active state, update both.
- **`CancelRegistry.Register` runs the previous entry's `cleanup` (idempotently) before overwriting** — re-registering an ID mid-flight will purge the old transfer's chunk cache. It does NOT invoke the old `cancel` func; the caller owns the new context lifecycle.
- **`guard`s use `url.Parse` not `strings.HasPrefix`**: `isLocalhostOrigin(origin)` (CORS guard) parses the URL hostname against `{localhost, 127.0.0.1, ::1}`. Any new origin check must use `url.Parse` + `Hostname()`, not prefix matching, or attackers can use `http://localhost.evil.com`-style bypasses.
