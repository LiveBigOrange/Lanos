# Lanos

> Secure peer-to-peer file transfer over local network — encrypted, fast, cross-platform.
> 局域网安全点对点文件传输——端到端加密、快速、跨平台。

---

## Features / 特性

- **End-to-end encrypted / 端到端加密** — Noise XX handshake with ed25519 identity / 基于 ed25519 身份的 Noise XX 握手
- **Cross-platform / 跨平台** — Windows, macOS, Linux (Android/iOS planned / 计划支持)
- **Dual-stack / 双栈支持** — Native IPv4/IPv6 with RFC 6724 address selection / 原生 IPv4/IPv6，RFC 6724 地址选择
- **Web sharing / 网页分享** — Share files via browser with password protection & QR code / 通过浏览器分享文件，支持密码保护和二维码
- **mDNS discovery / 自动发现** — Auto-detect peers on the same LAN / 自动发现局域网内设备
- **Async transfers / 异步传输** — Non-blocking send/receive with progress tracking / 非阻塞收发，支持进度追踪

---

## Project Structure / 项目结构

```
lanos/
├── core/                  # Go daemon (gcd) / Go 守护进程
│   ├── cmd/gcd/           # Desktop entrypoint / 桌面入口
│   ├── identity/          # ed25519 key management / 密钥管理
│   ├── config/            # config.yaml read/write / 配置读写
│   ├── instance/          # Single-instance lock / 单实例锁
│   ├── lifecycle/         # Startup handshake / 启动握手
│   ├── discovery/         # mDNS broadcast & browse / mDNS 发现
│   ├── api/               # Local REST API + Bearer auth / 本地 API + 认证
│   ├── store/             # SQLite (transfer_log.db) / 数据存储
│   ├── transport/         # Noise XX + frame mux / 传输层
│   ├── transfer/          # Chunk + state machine / 传输状态机
│   ├── receive/           # Inbound P2P server / 入站 P2P 服务
│   ├── share/             # HTTP web share / 网页分享
│   └── net/               # TCP dial/listen, address selector / 网络层
├── mobile/bind/           # gomobile facade / 移动端桥接层
├── ui/                    # Flutter desktop + mobile UI / Flutter 界面
│   ├── lib/
│   │   ├── main.dart
│   │   ├── pages/
│   │   ├── services/
│   │   └── widgets/
│   └── test/
├── docs/
│   ├── PROTOCOL.md        # Protocol spec / 协议规范
│   └── NETWORK.md         # Network troubleshooting / 网络排障
├── scripts/
│   ├── build/             # Per-OS packaging / 各平台打包脚本
│   └── lanos-setup-firewall.*
└── .github/workflows/     # CI + Release / 持续集成与发布
```

---

## Getting Started / 快速开始

### Prerequisites / 前置条件

- Go 1.22+
- Flutter 3.27+ (stable)

### Go Core / Go 核心

```bash
cd core
go mod tidy
go build ./cmd/gcd
go test -race ./...
./gcd    # prints handshake JSON to stdout / 输出握手 JSON
```

Handshake output / 握手输出：

```json
{"port":52100,"api_token":"nc5AkzhR9mNRc8S8ajAs9sOz0r_0rQWnZrNIaxDyLL8","version":"0.1.0"}
```

Verify with curl / 使用 curl 验证：

```bash
curl http://127.0.0.1:52100/api/v1/ping                              # {"ok":true}
curl -H "Authorization: Bearer <token>" http://127.0.0.1:52100/api/v1/version
curl -H "Authorization: Bearer <token>" http://127.0.0.1:52100/api/v1/devices
```

### Flutter UI / Flutter 界面

```bash
cd ui
flutter pub get
flutter analyze --fatal-infos
flutter test
flutter run -d windows   # or macos / linux / 或 macos / linux
```

> The Flutter app spawns `gcd` as a subprocess. Ensure `gcd` is in PATH or set `LANOS_GCD_PATH`.
> Flutter 应用会启动 `gcd` 子进程。请确保 `gcd` 在 PATH 中或设置 `LANOS_GCD_PATH` 环境变量。

---

## API Overview / API 概览

| Method | Path | Notes / 说明 |
|--------|------|-------|
| GET | `/api/v1/ping` | No auth / 无需认证 |
| GET | `/api/v1/devices` | Self + peers from mDNS / 本机 + mDNS 发现的设备 |
| GET | `/api/v1/events` | SSE stream (device presence) / SSE 事件流 |
| POST | `/api/v1/transfers` | Body: `{"peer_id","file_path"}` / 创建传输 |
| GET | `/api/v1/transfers/{id}` | Transfer detail / 传输详情 |
| POST | `/api/v1/transfers/{id}/cancel` | Cancel transfer / 取消传输 |
| GET/POST/DELETE | `/api/v1/shares[/{id}]` | Web share CRUD / 网页分享管理 |
| GET | `/api/v1/incoming` | Pending incoming prompts / 待处理入站请求 |
| POST | `/api/v1/incoming/{id}/accept` | Accept incoming / 接收入站 |
| POST | `/api/v1/incoming/{id}/reject` | Reject incoming / 拒绝入站 |

All endpoints (except `/ping`) require `Authorization: Bearer <token>`.
所有接口（`/ping` 除外）均需 `Authorization: Bearer <token>` 认证。

---

## License / 许可证

MIT
