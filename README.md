# Lanos

> 局域网跨平台文件共享工具 - 让你像 AirDrop 一样在 **Windows、macOS、Linux、Android、iOS** 之间自由传输文件，原生支持 IPv4/IPv6 双栈。

**当前阶段：P0 工程基线就绪（W1）**。See `IMPLEMENTATION_ROADMAP.md` for the full 16-week plan.

---

## 项目结构

```
lanos/
├── core/                  # Go Core Daemon（桌面独立进程 + 移动端 gomobile 共用）
│   ├── cmd/gcd/           # 桌面 gcd 入口
│   ├── identity/          # ed25519 密钥管理
│   ├── config/            # config.yaml 读写
│   ├── instance/          # 跨平台单实例锁
│   ├── lifecycle/         # 启动握手 + 随机端口 + API token
│   ├── discovery/         # mDNS 广播与监听
│   ├── api/               # 本地 REST API + Bearer 鉴权 + CORS
│   ├── store/             # SQLite (transfer_log.db)
│   ├── transport/         # Noise XK + SAS（P1 W3）
│   ├── transfer/          # chunk + queue + state machine（P1 W4）
│   ├── share/             # 网页分享 HTTP（P2 W6）
│   └── ...
├── mobile/bind/           # gomobile bind 入口（P4 W12）
├── ui/                    # Flutter 五端共用 UI
│   ├── lib/
│   │   ├── main.dart
│   │   ├── pages/
│   │   └── services/      # api_client / lifecycle_controller
│   ├── test/
│   └── integration_test/
├── docs/
│   ├── PROTOCOL.md        # mDNS / Noise / 帧格式 / 错误码 精确字节序
│   ├── NETWORK.md         # IPv4/IPv6/Avahi 部署排障（P3 W11）
│   └── PRIVACY.md         # 隐私声明 + iOS PrivacyInfo（P4 W14）
├── scripts/
│   ├── build/             # 五端打包脚本（P2 W8）
│   └── lanos-setup-firewall.sh
├── .github/workflows/     # CI + Release
├── PRD_v2_complete.md     # V2.1 多端扩展版 PRD
├── IMPLEMENTATION_ROADMAP.md
└── README.md
```

---

## 开发

### Go core

```bash
cd core
go mod tidy
go build ./...
go test -race ./...
go run ./cmd/gcd    # 启动 gcd，stdout 输出握手 JSON
```

握手示例输出：

```
{"port":52100,"api_token":"nc5AkzhR9mNRc8S8ajAs9sOz0r_0rQWnZrNIaxDyLL8","version":"0.1.0"}
```

curl 验证：

```bash
curl http://127.0.0.1:52100/api/v1/ping                              # {"ok":true}
curl -H "Authorization: Bearer <token>" http://127.0.0.1:52100/api/v1/version
curl -H "Authorization: Bearer <token>" http://127.0.0.1:52100/api/v1/devices
```

### Flutter UI

```bash
cd ui
flutter pub get
flutter analyze
flutter test
flutter run -d windows   # / macos / linux
```

> P0 阶段 `flutter run` 会尝试启动 `gcd` 子进程。请确保 `gcd` 已在 PATH 中，或通过 `LifecycleControllerDesktop(gcdPath: ...)` 指定路径。

### CI

GitHub Actions 三平台矩阵（`ubuntu-22.04 / macos-13 / windows-2022`）跑：

- Go: `gofmt` + `go vet` + `go test -race` + `go build`
- Flutter: `flutter pub get` + `dart format --set-exit-if-changed` + `flutter analyze --fatal-infos` + `flutter test`

---

## 路线图状态

| 阶段 | 周期 | 状态 |
|------|------|------|
| P0 准备 | W1 | ✅ 完成 |
| P1 桌面 MVP | W2-W5 | ✅ W2-W5 完成（状态机 + 集成测试 + 通知合并 + 30s 超时）；P1-24 真机 E2E 待手测 |
| P2 网页分享 + 记录 + 桌面收尾 | W6-W8 | ⏳ |
| P3 IPv6 双栈 + Linux 深度适配 | W9-W11 | ⏳ |
| P4 移动端 Android + iOS | W12-W16 | ⏳ |

详见 `IMPLEMENTATION_ROADMAP.md`。

---

## 许可证

待定（MVP 发布前确认）。
