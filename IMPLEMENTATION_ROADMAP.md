# Lanos 实施路线图

> 版本：v1 | 配套 PRD：`PRD_v2_complete.md`（V2.1 多端扩展版） | 团队规模：个人 / 1-2 人 | 迭代节奏：周级

---

## 0. 总览

### 0.1 阶段划分

| 阶段 | 周期 | 目标 | 关键交付 |
|------|------|------|---------|
| P0 准备 | W1 | 工程基线就绪 | Repo + CI + 工具链 + 协议规范草案 |
| P1 桌面 MVP | W2-W5 | Windows + macOS 双机互传可用 | 桌面两端走通"发现 → SAS → 直传 → 通知" |
| P2 网页分享 + 记录 + 桌面收尾 | W6-W8 | 桌面三端完整 MVP（含 Linux） | Windows/macOS/Linux 用户可用完整 MVP |
| P3 IPv6 双栈 + Linux 深度适配 | W9-W11 | 验收 IPv6 + Avahi + deb/rpm | IPv4-only / IPv6-only / 双栈三种网络均通过 |
| P4 移动端 Android + iOS | W12-W16 | 五端 MVP 全齐 | apk/ipa 内测分发 |

> **总周期估算 16 周**（约 4 个月，含 buffer）。Solo 节奏按 12-15 有效工作日/月估算。

### 0.2 设计原则

1. **垂直切片优先**：每个阶段交付一个真实可用的二进制（能在用户机器上跑通端到端流程），而非按"先做所有 UI 再做所有后端"
2. **桌面两端先跑通核心路径**：避免一次铺五端，先在 Win+macOS 验证最难的"发现+加密+用户感知"
3. **IPv6 与 Linux 推迟做**：这俩本身不阻塞桌面互传，但打包/Avahi 调试吃周时间，单独留阶段
4. **移动端单独留阶段**：gomobile bind 的工程化、SAF/Keychain、前台服务这些都是新坑，独立冲刺
5. **测试驱动核心路径**：起手第一周就要建好 CI 与 E2E 集成测试框架，否则后续五端回归会失控

---

## 1. P0 准备（W1）

### 1.1 目标

把工程基线搭起来，让后续每周迭代可以并行开发并自动验证。

### 1.2 任务清单

| # | 任务 | 说明 | DoD |
|---|------|------|-----|
| P0-1 | 建立 monorepo 结构 | `core/`（Go）、`ui/`（Flutter）、`mobile/bind/`（gomobile）、`docs/`、`scripts/`、`.github/workflows/` | 结构图见 §6.1 |
| P0-2 | `core/go.mod` 初始化 + 关键依赖锁版本 | `chi` `flynn/noise` `grandcat/zeroconf` `modernc.org/sqlite` `yaml.v3` `log/slog` | `go mod tidy` 通过 |
| P0-3 | `ui/pubspec.yaml` 初始化 + 关键插件 | `system_tray` `flutter_local_notifications` `file_selector` `qr_flutter` `device_info_plus` `path` `flutter_localizations` | `flutter pub get` 通过 |
| P0-4 | CI 矩阵：`ubuntu-22.04 / macos-13 / windows-2022` | 每矩阵跑 `go test ./...` + `go build` + `flutter analyze` + `flutter test` | 首次提交即绿 |
| P0-5 | 写协议规范草案 `docs/PROTOCOL.md` | mDNS TXT、`lanos://connect` URI、Noise XK + SAS 流程、chunk 格式、stream ID 复用、错误码 | 团队 review 接受 |
| P0-6 | 决定 device-id 生成方案 | ed25519 公钥 SHA256 前 8 字节 hex，持久化失败重生成 | 写入 `docs/PROTOCOL.md` |
| P0-7 | 确定 lint/format 基线 | Go: `gofmt` + `golangci-lint` (revive, errcheck, gosec)；Dart: `flutter analyze --fatal-infos` + `dart format` | 配置文件提交 |

### 1.3 风险

- `grandcat/zeroconf` 在 Windows 上行为差异较大 → P0 周内做一个最小 mDNS echo demo 跑三平台
- `flynn/noise` 的 XK 模式示例少 → P0 周内写一个 100 行噪声测试，验证 DH → SAS code 计算一致

---

## 2. P1 桌面 MVP（W2-W5）

### 2.1 P1 进度图

```
W2: Go core 基础（identity/config/单实例锁/mDNS 注册）+ Flutter 骨架 UI
W3: Noise XK + SAS 4 位确认 + 桌面两端 SSE 通路
W4: 文件直传（4MB chunk + 多 stream 复用）+ 进度 UI
W5: 收发状态机 + 集成测试 + 桌面两端真机验收
```

### 2.2 任务清单（按周）

**W2：地基**

| # | 任务 | DoD |
|---|------|-----|
| P1-1 | `core/identity/`：生成 ed25519 密钥对 + 平台加密存储（Win DPAPI / macOS Keychain / Linux 0600）| 三平台通过单元测试 |
| P1-2 | `core/config/`：`config.yaml` 读写 + 设备名 + 下载路径 + 端口随机持久化 | yaml schema 文档化 |
| P1-3 | `core/instance/`：跨平台单实例锁（Win mutex / macOS flock `/tmp` / Linux flock） | 双开第二次自动退出 |
| P1-4 | `core/discovery/mdns.go`：注册 + 监听 `_lanos._tcp.local.`，TXT 含 §3.1.1 全字段 | 单机 echo 测试 |
| P1-5 | `core/api/`：`chi` 路由 + Bearer token 中间件 + `/ping` `/devices` 端点 | curl 测试通过 |
| P1-6 | `core/lifecycle/`：Go core 启动握手 §5.1.3，stdout JSON 输出 port+token | Flutter 读到并连上 |
| P1-7 | Flutter UI 骨架：主窗口 + 设备列表 widget + LifecycleController | 启动后展示本机设备卡片 |

**W3：握手与连通**

| # | 任务 | DoD |
|---|------|-----|
| P1-8 | `core/transport/noise.go`：Noise XK 握手 + 1-RTT 通道建立 | 双端握手 unittest |
| P1-9 | `core/transport/sas.go`：双方计算 `(sha256(DH) mod 10000)` 4 位数 | 与对端值一致 |
| P1-10 | SSE `/api/v1/events` 通路 + 100ms 节流 | Flutter `EventSource` 收到 `device.online` |
| P1-11 | Flutter 4 位确认弹窗 UI + 30s 超时取消 | 真机双端对照数字一致点确认 → 收到 `transfer.request` |
| P1-12 | `trusted_devices.json` 读写 + 公钥变更降级陌生 | 单元测试覆盖降级 |
| P1-13 | 离线检测：mDNS TTL + 5s ICMP ping + 3 次失败标灰 | 拔网线 15s 后 UI 正确标记 |

**W4：传输管道**

| # | 任务 | DoD |
|---|------|-----|
| P1-14 | `core/transport/stream.go`：长连接 + 32-bit stream ID 多路复用 | 单连接并发双 stream 传两个文件 |
| P1-15 | `core/transfer/chunk.go`：4MB chunk + 序号 + SHA256 校验 | 1GB 基准 ≥ 80 MB/s |
| P1-16 | `core/transfer/queue.go`：每设备独立队列 + 4 上/4 下并发上限 | 单元测试覆盖排队 |
| P1-17 | Flutter 传输进度 UI（双进度条 + 速度 + ETA + 文件名） | 60fps 不卡顿 |
| P1-18 | 取消操作 + 临时数据清理 | 任一方取消后 chunk cache 清空 |
| P1-19 | 文件路径映射（去盘符 + 相对路径）+ Windows 非法字符替换 | 跨平台接收测试 |

**W5：状态机 + 收尾**

| # | 任务 | DoD |
|---|------|-----|
| P1-20 | `core/transfer/state.go`：状态机（待确认/连接中/传输中/已完成/已取消/已失败/待续传） | 覆盖全状态迁移 |
| P1-21 | 集成测试：`go test ./integration/` 双实例 127.0.0.1 互传 | CI 自动跑 |
| P1-22 | 通知：Flutter 发原生通知 + 合并策略"5 个文件一条通知" | 单元 + 手测 |
| P1-23 | 30s 接收超时自动拒绝 + 发送方"对方未响应" | 手测 |
| P1-24 | 桌面两端真机 E2E：Win ↔ macOS 5GB 文件零失败 | bug-hunt 记录 |

### 2.3 P1 验收

- [ ] Win-Mac 互传 1GB 单文件 ≤ 13s（≥ 80 MB/s）
- [ ] Win-Mac 互传 100 × 10MB 文件 ≤ 25s
- [ ] 拔网线 15s 内正确标离线
- [ ] 首次 SAS 验证流程两用户实测 ≤ 30s 完成
- [ ] 同一设备连续传输 100 次零失败

---

## 3. P2 网页分享 + 记录 + 桌面收尾（W6-W8）

### 3.1 进度图

```
W6: 网页分享 HTTP 服务（token + 密码 + ZIP 流式）
W7: 共享/接收记录 + 设置页 + 首次引导
W8: Linux 适配（Avahi/deb/rpm/AppImage）+ 三端打包
```

### 3.2 关键任务

**W6：网页分享**

- `core/share/server.go`：单 HTTP 路由 `/dl/<token>` `/api/share/<token>/status` `/qr/<token>`
- token 32 字节随机 + 密码 SHA256+salt 内存存储
- `archive/zip` + `io.Pipe` 流式打包 + UTF-8 文件名 + 0x0800 flag
- 防枚举：单 IP 10 次错 token / 密码 → 封禁 5 分钟 → 429
- 64 同时分享上限 + 自动过期清理
- 端口冲突自动 / 退出清理

**W7：记录与设置**

- `core/store/transfer_log.db` sqlite schema：shares、transfers 两表
- 共享记录 / 接收记录 UI + 搜索 + 多选删除 + 排序 + CSV 导出
- 设置页：设备名 / 下载路径 / 自动接收策略 / 冲突处理 / 端口范围 / 隐身模式 / 语言 / 通知三开关 / 同时分享上限
- 首次引导 3 步页（欢迎 / 功能介绍 / 设置）

**W8：Linux + 打包**

- Avahi 检测：启动 `pidof avahi-daemon`，未运行弹说明
- `lanos-setup-firewall.sh`：ufw/firewalld/iptables 三选一
- `.desktop` 文件 + hicolor SVG + MimeType 关联
- 打包脚本：
  - Win：Inno Setup → `Lanos-Setup-x.y.z.exe`
  - macOS：Universal Binary dmg（arm64 + x86_64）
  - Linux：AppImage + deb（`Depends: avahi-daemon`）+ rpm
- GitHub Actions Release 工作流（tag 推送自动产五包）

### 3.3 P2 验收（桌面三端完整 MVP）

- [ ] 同事在浏览器（手机/PC 任意）输入链接 + 密码下载文件夹 zip 可用
- [ ] 共享 100 个文件 zip 流式打包 ≤ 30 分钟，进度可见
- [ ] ZIP 文件名跨 Win/macOS/Linux 正确显示中文
- [ ] 网页分享退出 App 后自动失效
- [ ] Avahi 检测在 Ubuntu 22.04 GNOME 默认安装下零额外配置可用
- [ ] 三端打包产物在干净 VM 上一键安装可运行

---

## 4. P3 IPv6 双栈 + Linux 深度适配（W9-W11）

### 4.1 进度图

```
W9: 核心层 IPv6（双栈监听 + 地址族择优 + AAAA mDNS）
W10: 跨端用例验证（IPv4-only / IPv6-only / 双栈三种矩阵）
W11: 打包收尾（防火墙 v6 规则 + 文档 + 网络诊断页）
```

### 4.2 关键任务

**W9：核心层**

- `core/net/listen.go`：`net.Listen("tcp", "[::]:port")` 双栈监听
- `core/discovery/aaAa.go`：mDNS 同时注册 A + AAAA；ip-ver TXT = `4` / `6` / `46`
- `core/net/addrselect.go`：RFC 6724 地址优先级；链路本地带 zone id 保留
- `lanos://connect` URI 新增 `ip6=` 参数（§3.1.6）
- 隐身二维码同时携带 v4 + v6

**W10：用例矩阵**

| 测试矩阵 | 本机 | 对端 | 预期 |
|---------|------|------|------|
| IPv4-only | 仅 v4 | 仅 v4 | 成功 |
| IPv6-only | 仅 v6 | 仅 v6 | 成功（链路本地 + 全局都验） |
| 双栈双栈 | v4 + v6 | v4 + v6 | 优先 v6 回退 v4 |
| v4↔v6 不兼容 | 仅 v4 | 仅 v6 | UI 报"网络协议不兼容" |
| NAT64 透明 | v4-only | v6-only+NAT64 | 成功通过翻译 |

- E2E 测试用 GitHub Actions 矩阵 + docker IPv6 网络（`docker network create --ipv6`）

**W11：收尾**

- 防火墙 §4.4 表：Win 同时添加 v4+v6 规则、deb/rpm 脚本同开
- 设置页"网络信息"诊断：展示所有监听 IP + 地址族 + 邻居发现受阻提示
- IPv6 性能基线 §4.1：≥ IPv4 85%
- 文档：README 增加 IPv6 排障 / `docs/NETWORK.md`

### 4.3 P3 验收

- [ ] 仅 IPv6 网络（关闭 v4 协议栈）Win ↔ macOS 互传 1GB ≥ 70 MB/s
- [ ] 链路本地 `fe80::...%eth0` 形式连接成功
- [ ] ip-ver 不兼容时 UI 错误码清晰
- [ ] Win 防火墙规则一次性写入 v4 + v6 双规则

---

## 5. P4 移动端 Android + iOS（W12-W16）

### 5.1 进度图

```
W12: Go core 模块重构 + gomobile bind 出 AAR/iOS Framework
W13: Android Flutter UI + SAF 适配
W14: iOS Flutter UI + Keychain + 安全书签
W15: 后台策略（Android 前台服务 + iOS URLSession）+ 端到端联调
W16: 打包发布（apk/aab/ipa/TestFlight）+ 移动端 bug hunt
```

### 5.2 关键任务

**W12：gomobile bind**

- 把 `core/api/` 中的 HTTP handlers 抽象为 `core/usecase/`（接口），HTTP 层与 gomobile 层都调用同一套 usecase
- gomobile 接口定义 `mobile/bind/bind.go`：`Lanos_*` 函数签名（listDevices / send / accept / reject / cancel / createShare / ...）
- 事件 payload struct 通过 gomobile-friendly 类型（基本类型 + 字符串 + JSON bytes）
- Flutter 侧 EventChannel 包装 Go callback
- CI 输出 AAR + iOS Framework 到 Release 资产

**W13：Android**

- `flutter_local_notifications` 前台服务（`foregroundServiceType=dataSync`）
- SAF `file_picker` + `takePersistableUriPermission` 持久化目录
- 从 content:// URI 流式读 → Noise → 写对端（不做 cache 反模式）
- 扫码：`mobile_scanner` 解析 `lanos://connect?...`，唤起 App 或跳下载页
- Material 3 大头像列表 + Bottom Sheet 选相册/文件
- 网络模式偏好：仅 Wi-Fi / + 蜂窝

**W14：iOS**

- `file_picker` → `UIDocumentPicker`，安全书签 `startAccessingSecurityScopedResource`
- `identity.key` 写 Keychain，Data Class `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`
- 扫码 / URI Scheme / Universal Link
- 大头像列表 + Large Title Navigation
- 触屏目标 ≥ 44×44 pt 全自查

**W15：后台策略**

- Android：前台服务持久通知"附近设备监听中"，Doze 时提示加入豁免白名单
- iOS：活跃传输用 `URLSession backgroundSession` 短续（≤30s），后台 mDNS 挂起；桌面端 UI 显式提示对方 iOS 不在前台
- 横向场景：手机 ↔ 桌面、手机 ↔ 手机 互传 1GB 大文件稳定性
- 电量：传输期间耗电数据采集（Android BatteryHistorian）

**W16：发布**

- Android：apk（自签 debug/release keystore）+ aab（Play Store）
- iOS：Apple Developer 签名 + TestFlight 公测链接
- 移动端 README：权限说明（存储、本地网络、通知）+ 隐私清单 `PrivacyInfo.xcprivacy`

### 5.3 P4 验收

- [ ] Android ↔ Windows 互传 1GB ≥ 25 MB/s
- [ ] iOS ↔ macOS 互传 100 张照片 ≥ 15 MB/s
- [ ] Android 后台 5 分钟仍可被发现（前台服务存活）
- [ ] iOS 后台 1 分钟内传输不中断，>1 分钟被冻结有友好的 UI 恢复
- [ ] SAF URI 重启后仍可读写
- [ ] iOS Keychain 备份关闭验证（越狱或设备迁移后 key 不可恢复）

---

## 6. 工程结构（脚手架预览，P0 落地）

```
lanos/
├── core/                              # Go core（桌面独立进程 + 移动端 gomobile 共用）
│   ├── go.mod
│   ├── cmd/gcd/                       # 桌面 Go Core Daemon 入口
│   ├── identity/                      # ed25519 密钥管理 + 平台加密
│   ├── config/                        # config.yaml schema + 读写
│   ├── instance/                      # 单实例锁
│   ├── discovery/                     # mDNS 注册 + 监听 + AAAA + ip-ver 协商
│   ├── net/                           # 双栈 listen + 地址族择优（RFC 6724）
│   ├── transport/                     # Noise XK + SAS + stream 多路复用
│   ├── transfer/                      # chunk + queue + state machine
│   ├── share/                         # 网页分享 HTTP 路由 + ZIP 流式 + token + 密码
│   ├── receive/                       # 接收方 chunk 写盘 + 续传 meta
│   ├── store/                         # SQLite (transfer_log.db) + 记录管理
│   ├── api/                           # chi 路由 + Bearer token + SSE /events
│   ├── usecase/                       # 业务用例接口（HTTP 与 gomobile 共用）
│   └── lifecycle/                     # 启动握手 stdout JSON + 信号处理
├── mobile/
│   └── bind/                          # gomobile bind 出 AAR / iOS Framework
│       └── bind.go                    # Lanos_* 函数签名单点出口
├── ui/                                # Flutter（五端共用）
│   ├── pubspec.yaml
│   ├── lib/
│   │   ├── main.dart
│   │   ├── lifecycle_controller.dart  # 桌面：拉起 gcd/读 stdout；移动：gomobile bind
│   │   ├── pages/
│   │   ├── widgets/
│   │   ├── services/
│   │   │   ├── api_client.dart         # 桌面 HTTP client + Bearer
│   │   │   ├── ffi_client.dart         # 移动 gomobile 调用
│   │   │   ├── event_source.dart       # 桌面 SSE / 移动 EventChannel 抽象
│   │   │   └── platform_file.dart      # 桌面 OS path / 移动 SAF URI 抽象
│   │   └── l10n/                        # zh / en
│   ├── test/                           # widget test
│   └── integration_test/              # 跨平台 UI 集成测试
├── docs/
│   ├── PROTOCOL.md                     # mDNS + Noise + SAS + lanos:// 协议
│   ├── NETWORK.md                     # IPv4/IPv6/Avahi 部署排障
│   └── PRIVACY.md                     # 隐私声明 + iOS PrivacyInfo
├── scripts/
│   ├── build/                          # 五端打包脚本
│   └── lanos-setup-firewall.sh        # Linux 防火墙配置
├── .github/workflows/
│   ├── ci.yml                          # 三平台 lint/test/build
│   ├── release.yml                     # tag 推送产五端产物
│   └── mobile.yml                      # Android/iOS build + 分发
└── README.md
```

---

## 7. 里程碑与发布节点

| 里程碑 | 周次 | 标志 | 形式 |
|--------|------|------|------|
| **M1 桌面两端 alpha** | W5 末 | Win ↔ macOS 互传可用（无网页分享、无记录） | 内部 demo 二进制 |
| **M2 桌面三端 beta** | W8 末 | Win/macOS/Linux 完整 MVP（含网页分享 + 记录 + 设置） | GitHub Release 私有 tag |
| **M3 IPv6 GA** | W11 末 | 桌面三端 IPv6 双栈全过矩阵 | 公开 alpha（testflight/closed beta） |
| **M4 移动 alpha** | W14 末 | Android + iOS 能与桌面互传（核心路径） | 内测 |
| **M5 MVP 全齐 GA** | W16 末 | 五端 MVP 全通过 §0.1 验收清单 | 公开发布 v0.1.0 |

---

## 8. 跨阶段共享风险与对策

| 风险 | 受影响阶段 | 对策 |
|------|-----------|------|
| `flynn/noise` 的 XK 实现细节文档少 | P1 W3 | P0 提前写 100 行 demo 验证 DH → SAS 计算路径 |
| `grandcat/zeroconf` Windows 行为差异 | P1 W2 | P0 周必跑三平台 echo |
| gomobile bind 重构 API 层时改动大 | P4 W12 | P1 起就把 usecase 接口分离，HTTP 层与 gomobile 共用，避免 P4 大重构 |
| iOS 后台限制不可逾越 | P4 W15 | PRD §3.6 已明确"受限对等端"，UI 提示用户保持前台 |
| IPv6 测试网络在 CI 稀缺 | P3 W10 | docker `--ipv6` network + `iproute2` 工具模拟单栈 v6 |
| 打包 / 签名耗时被低估 | P2/P4 末 | P0 起就在 CI 试产出空包，每周迭代；签名证书决策在 M4 之前定 |
| 独立单人节奏断档 | 全程 | 每周末做"周报 + 下周任务卡片"防止丢失上下文 |

---

## 9. 每周仪式

- **周一首晨会（即使是 solo）**：写下本周 5-7 个任务卡，明确 DoD
- **周三中检**：跑一次完整 CI，看是否红，红则当天修
- **周五收尾**：在 GitHub Release Notes 草稿上贴"本周交付"，截图归档；下周卡整理
- **每 M 里程碑末**：做一次"bug hunt party"（自己 / 同事拉五台机器互传，记录所有 bug）

---

## 10. 下一步（请用户确认后执行）

- [ ] 是否同意 P0 W1 即开始落 monorepo 脚手架（按 §6 结构树创建目录 + go.mod + pubspec.yaml + CI）
- [ ] P0 W1 是否需要先产出 `docs/PROTOCOL.md` 协议规范细化（mDNS TXT 字段、Noise XK 握手字节序、chunk header 格式、错误码 JSON schema）
- [ ] 是否启用 GitHub Project 板做任务卡追踪（可选）

> 路线图到此为止。等用户确认 P0 范围后开始执行。