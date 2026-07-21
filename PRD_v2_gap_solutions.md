# PRD 剩余缺口 · 方案选型分析

针对 17 项未完善/不确定项，逐项给出可选方案对比与推荐选型。

---

## 一、协议与实现细节

### 1. 确认码 SAS 握手流程

**目标**：双方首次连接时通过 4 位数验证中间人攻击。

| 方案 | 描述 | 优势 | 劣势 |
|------|------|------|------|
| **A. 双方各算一半** | 发送方和接收方各自基于 DH 结果 hash 取前 4 位，UI 展示后用户视觉比对 | 真正的 SAS 机制，密码学严谨 | 实现复杂度高，需双方同时算 hash 并展示 |
| **B. 发送方生成，接收方校验** | 发送方生成随机 4 位数，经已建立的加密通道发给接收方，接收方展示 | 实现简单，UX 清晰（一端发起，一端确认） | 密码学性弱——若加密通道已遭 MITM，确认码也会被劫持 |
| **C. STPA（Short Authentication String）+ 双向展示** | A 方案 + UI 强制要求双方都点"确认数字一致" | 兼顾严谨与易用 | 双方 UI 交互稍繁 |

**推荐：方案 A（SAS）+ 双向确认**

**理由**：
- AirDrop/Bluetooth Pairing 用的都是 SAS 思路，已被行业验证
- 密码学要求是 MVP 不能省的——局域网内可能存在恶意设备
- 实现流程：
  ```
  1. 双方通过 mDNS 交换公钥
  2. 发起方建立 Noise XK 握手，第一轮消息携带临时 DH 公钥
  3. 接收方回握手消息，携带自己的临时 DH 公钥
  4. 双方各自计算：code = (sha256(DH_result) mod 10000)
  5. 双方 UI 同步弹出 code，用户视觉比对一致后点击"确认"
  6. 确认后才进行后续传输握手
  ```

**超时机制**：30 秒未点确认自动取消，双方均收到通知

### 2. mDNS TXT Records 格式

**目标**：广播足够信息让对端发现并连接，但不泄露敏感数据。

| 方案 | 描述 | 优势 | 劣势 |
|------|------|------|------|
| **A. 完整公钥广播** | TXT record 中包含 ed25519 公钥 hex（64 字节） | 接收方无需二次握手即可校验身份 | 公钥每次广播有隐私泄露风险（同一公钥可关联同一设备） |
| **B. 公钥 hash 广播** | TXT 只含 `pk_hash=sha256(pk)[:16]`，公钥在实际连接时交换 | 隐私保护好 | 需要二次连接获取完整公钥，增加往返 |
| **C. 分层广播** | TXT 含设备名 + 公钥 hash；连接时第一轮交换完整公钥并校验 hash | 兼顾隐私与性能 | 实现稍复杂 |

**推荐：方案 C（分层广播）**

**TXT record 最终格式**：
```
txt-ver=1
proto=lanos/1.0
platform=macos          # windows/macos/linux
port=52100               # 直传监听端口（每设备固定）
pk-hash=3f8a2b1c4d5e6f7a8b9c0d1e2f3a4b5c
device-name=My MacBook Pro (URL encoded)
```
- 不广播完整公钥，避免设备指纹追踪
- 直传端口：每设备启动时从 52100-52999 中选一个固定端口（首次随机，持久化到 config.yaml）

### 3. 网页分享二维码编码内容

**建议很简单，直接确定**：
- 二维码内容 = 完整下载 URL
- 例：`http://192.168.1.100:52103/dl/a3f8c2d1e4b5f6a7b8c9d0e1f2a3b4c5`
- 由 **Flutter 端**生成二维码（`qr_flutter` 包，纯 Dart，无原生依赖）
- 已知局限：必须同局域网才能访问，文档注明

---

## 二、交互逻辑未闭环

### 4. 同时收发冲突

| 方案 | 描述 | 评估 |
|------|------|------|
| **A. 串行（拒绝并提示）** | A 给 B 发送过程中，B 拖拽给 A 时弹出"对方正在发送，请稍后" | UX 差，大文件传输期间无法回传 |
| **B. 并行传输** | 双方各自维护独立传输任务，互不干扰 | UX 好；底层 TCP 全双工，性能也好 |
| **C. 提示用户选择** | B 拖拽时弹"立即发送 / 等当前传输完成" | 给用户选择权，但打断流程 |

**推荐：方案 B（并行传输）**

**理由**：
- TCP 本身全双工，加密通道双向独立
- 千兆网带宽双向各 1Gbps，互不挤占
- 实现：每对设备间建立**一条**长连接，在其上多路复用双向传输（类似 HTTP/2 stream 模型，但更简单——给每个传输分配一个 32-bit stream ID）
- UI 上分别显示上下行两个进度条

### 5. "保留两者"文件名规则

| 方案 | 规则 | 跨平台一致性 |
|------|------|--------------|
| **A. macOS/Windows 原生风格** | `file.ext` → `file (1).ext` → `file (2).ext` | 不一致（macOS 用 `file 2.ext`，Windows 用 `file (1).ext`） |
| **B. 统一规则** | `file.ext` → `file_1.ext` → `file_2.ext`（下划线 + 序号） | 跨平台一致 |
| **C. 时间戳后缀** | `file_20260721_153022.ext` | 唯一性强，但文件名变长，UX 差 |

**推荐：方案 B（统一规则）**

**规则**：
```
原始: 报告.docx
冲突1: 报告_1.docx
冲突2: 报告_2.docx
冲突3: 报告_3.docx
```
- 选下划线而非空格，避免 shell 处理麻烦
- 序号检测从 1 开始，跳过已存在的序号

### 6. 传输记录保留策略

| 方案 | 策略 | 评估 |
|------|------|------|
| **A. 永久保留** | 记录一直存在 | 简单，但 SQLite 文件会持续增长，多年后可能几百 MB |
| **B. 按时间清理** | 保留最近 30/90/365 天 | 自动管理，但用户可能想看历史记录 |
| **C. 按数量限制** | 保留最近 1000 条 | 自动管理，可预测 |
| **D. 按数量 + 设置可选** | 默认 1000 条，设置中可改"永久保留" | 灵活 |

**推荐：方案 D**

**默认行为**：
- 保留最近 1000 条
- 超出后删除最旧记录（异步清理，不阻塞 UI）
- 设置中可选"永久保留" / "保留最近 100 / 1000 / 10000 条"
- 接收的文件本身不删除，仅清理记录

### 7. 传输速度与剩余时间显示

**建议直接确定**：
- 进度条下方显示：`12.3 MB/s · 剩余 2 分 15 秒`
- 速度计算：滑动窗口平均（最近 2 秒内的 bytes/s），避免抖动
- 剩余时间：`剩余大小 / 平均速度`
- < 1 MB/s 时只显示速度不显示 ETA（速度太低时 ETA 不准）

---

## 三、技术实现未定

### 8. Go HTTP 框架选型

| 选项 | 体积影响 | 性能 | 中间件生态 | 交叉编译 |
|------|----------|------|-----------|----------|
| **net/http + http.ServeMux** | 0 额外依赖 | 足够 | 自己写 | 极简 |
| **chi** | +200KB | 高 | 丰富 | 简单 |
| **gin** | +6MB | 高 | 极丰富 | 中等 |
| **echo** | +3MB | 高 | 丰富 | 中等 |

**推荐：标准库 `net/http` + `chi` 路由器**

**理由**：
- chi 仅 200KB，纯 Go，零反射魔法，交叉编译零变化
- 提供中间件支持（日志、限流、CORS、Recover）
- API 风格与标准库兼容，迁移成本低
- 不选 gin/echo：体积偏大、引入额外依赖、本项目 API 数量少（~20 个端点），过度设计

### 9. Flutter 原生插件清单

| 能力 | 推荐插件 | 备选 |
|------|---------|------|
| 系统托盘 | `system_tray` (跨三平台) | `tray_manager` |
| 原生通知 | `flutter_local_notifications` | `local_notifier`（桌面专门） |
| 开机自启 | `launch_at_startup` (Windows/macOS) | Linux 用 `~/.config/autostart/*.desktop` 文件操作 |
| 文件对话框 | `file_selector` (官方维护) | `file_picker` |
| 二维码生成 | `qr_flutter` (纯 Dart) | - |
| 系统信息 | `device_info_plus` (官方) | - |
| 路径处理 | `path` (官方) | - |

**注意**：
- `launch_at_startup` 不支持 Linux，需手写 `desktop` 文件创建/删除逻辑（约 20 行代码）
- 一律使用 pub.dev 上 **官方/高 star 包**，避免早夭项目

### 10. 日志脱敏规则

| 方案 | 描述 | 适用场景 |
|------|------|----------|
| **A. 单一脱敏** | 所有日志中文件名替换为 `<FILE>` | 最严格，开发不友好 |
| **B. 分级脱敏** | 三级日志：生产/调试/全量，由日志级别控制 | 灵活 |
| **C. 配置驱动** | 配置文件中显式列出脱敏字段 | 最灵活，但增加配置复杂度 |

**推荐：方案 B（分级脱敏）**

**实现**：
```go
type LogLevel int
const (
    ProdLevel LogLevel = iota  // 文件名 → <FILE>，路径 → <PATH>
    DebugLevel                 // 文件名 → report_***.pdf（保留后缀+前 6 字符+***）
    VerboseLevel               // 原始输出（仅开发环境）
)
```
- 默认 `ProdLevel`，通过环境变量 `LANOS_LOG_LEVEL=debug` 切换
- 脱敏发生在写入前，不在输出层过滤，避免日志文件中残留敏感信息

### 11. 通知谁发的问题

**确定方案**：**Flutter 端发原生通知**

**架构**：
```
Go core 检测到事件 → SSE 推送 → Flutter 接收 → 调用原生通知 API
```
- Go core 不直接调系统通知 API（不在用户会话中，权限受限）
- 这个决策已基本确定，无需多方案对比

### 12. 数据库选型

| 选项 | 依赖 | 交叉编译 | 性能 |
|------|------|----------|------|
| **modernc.org/sqlite** | 纯 Go | 简单 | 中等（比 CGO 慢 2-3x） |
| **mattn/go-sqlite3** | CGO | 需交叉编译工具链 | 最快 |
| **JSON 文件** | 无 | 极简 | 写入频繁时慢 |
| **bbolt** | 纯 Go | 简单 | KV 结构，查询能力弱 |

**推荐：`modernc.org/sqlite`**

**理由**：
- 纯 Go，无 CGO，交叉编译零成本
- 性能对于本场景足够（记录写入频率低，每天几百条）
- SQL 查询能力强（按时间排序、按设备筛选都简单）
- 不选 `mattn/go-sqlite3`：CGO 让交叉编译复杂度提升一个量级，不值得
- 不选 JSON：传输记录含时间字段和过滤需求，JSON 检索性能差
- 不选 bbolt：键值结构和 SQLite 表结构差不多，但查询能力不如 SQL

### 13. 设备离线检测机制

| 方案 | 检测延迟 | CPU 开销 | 实现复杂度 |
|------|----------|----------|-----------|
| **A. 仅依赖 mDNS TTL** | 最差 120 秒 | 零 | 零 |
| **B. mDNS + 心跳探测** | 最差 15 秒 | 极低（每 5 秒发一个 ping） | 中等 |
| **C. TCP 长连接保活** | 最差 10 秒 | 极低 | 高（需管理所有连接） |

**推荐：方案 B（mDNS + 心跳探测）**

**实现**：
- 基础：mDNS 发现 + TTL 包含的隐式离线检测（120s 上限）
- 增强：每 5 秒发一个 mDNS "goodbye" packet 或者 ICMP ping 心跳
- 判定：连续 3 个心跳（15 秒）无响应标记离线
- 设备从在线变为离线时，UI 标灰头像，不立即从列表移除（保留 5 分钟，便于网络抖动恢复）

### 14. 大文件 UI 卡顿问题

**确认方案：节流推送**

**实现**：
- Go core 在 chunk 接收循环中累积进度，最多每 100ms 推送一次 SSE 事件
- 而非每个 chunk 一次

**对比**：
| 推送频率 | 网络流量 | UI 帧率 | 10GB 文件事件数 |
|----------|----------|---------|---------------|
| 每 chunk | 高 | 卡顿 | 2560 次 |
| 每 100ms | 低 | 流畅 | 6 分钟 × 600 = 3600 次（适中） |
| 每 500ms | 极低 | 流畅 | 720 次（进度可能滞后） |

**推荐：100ms 节流**，UX 与流量平衡

---

## 四、工程与质量

### 15. 测试策略

| 层级 | 工具 | 覆盖范围 | CI 集成 |
|------|------|----------|--------|
| 单元测试 | Go `testing` | 加密、端口分配、token 生成、文件名规则、冲突处理 | 必须 |
| 集成测试 | Go + 本地回环 | 两个 Go 实例在 `127.0.0.1` 上互传 | 必须 |
| E2E 测试 | 物理机/VM | 局域网真实发现 + 跨平台传输 | 手动为主 |
| UI 测试 | Flutter `integration_test` | 拖拽、按钮点击、设置页 | 可选 |

**推荐方案**：
- **CI 自动化**：单元 + 集成测试（GitHub Actions 上跑 Linux 容器）
- **集成测试技巧**：在同一测试进程里启动两个 Go core 实例，分别绑定 `127.0.0.1:52100` 和 `127.0.0.1:52101`，互相发现并传输
- **跨平台真机测试**：先在团队内部定期做"传文件bug hunt"，CI 难覆盖 macOS/Windows 实机场景

### 16. 启动时间目标

| 阶段 | 目标 | 测量方式 |
|------|------|----------|
| 双击图标到窗口可见 | ≤ 1.5s | Flutter desktop 启动 + 第一帧渲染 |
| Go core 启动并开始 mDNS 广播 | ≤ 0.5s | Go 进程启动到 `registerService()` 调用完成 |
| 设备列表出现第一台在线设备 | ≤ 3s | 从启动到 `device.online` 事件首次触发 |

**推荐**：直接采用上述目标作为非功能需求

### 17. 错误码体系

**推荐方案：统一前缀 + 全大写下划线**

```go
const (
    ErrCodeDeviceOffline          = "DEVICE_OFFLINE"
    ErrCodeTransferInProgress     = "TRANSFER_IN_PROGRESS"
    ErrCodeShareExpired           = "SHARE_EXPIRED"
    ErrCodeShareDownloadLimit     = "SHARE_DOWNLOAD_LIMIT"
    ErrCodePortUnavailable        = "PORT_UNAVAILABLE"
    ErrCodeFileNotFound           = "FILE_NOT_FOUND"
    ErrCodePermissionDenied       = "PERMISSION_DENIED"
    ErrCodeConfirmCodeMismatch    = "CONFIRM_CODE_MISMATCH"
    ErrCodeConfirmCodeTimeout     = "CONFIRM_CODE_TIMEOUT"
    ErrCodeTransferRejected       = "TRANSFER_REJECTED"
    ErrCodeTransferCanceled       = "TRANSFER_CANCELED"
    ErrCodeShareLimitExceeded     = "SHARE_LIMIT_EXCEEDED"
    ErrCodeTrustedDeviceLimit     = "TRUSTED_DEVICE_LIMIT"
)
```

**API 错误响应格式**：
```json
{
  "error": {
    "code": "DEVICE_OFFLINE",
    "message": "目标设备已离线",
    "details": {"device_id": "abc123"}
  }
}
```

**Flutter 端处理**：建立 `error_code.dart` 枚举，按 code 分支展示对应中文/英文 i18n 提示

---

## 五、推荐选型汇总表

| # | 项 | 推荐方案 |
|---|-----|----------|
| 1 | 确认码握手 | 方案 A：SAS + 双向确认 |
| 2 | mDNS TXT record | 方案 C：分层广播（hash + 连接时交换全公钥） |
| 3 | 二维码内容 | 完整下载 URL，Flutter 端用 qr_flutter 生成 |
| 4 | 同时收发冲突 | 方案 B：并行传输（单连接多路复用） |
| 5 | 保留两者命名 | 方案 B：`file_1.ext`、`file_2.ext` |
| 6 | 记录保留 | 方案 D：默认 1000 条，设置可改 |
| 7 | 速度/ETA 显示 | 滑动窗口平均，100ms 节流 |
| 8 | Go HTTP 框架 | `net/http` + `chi` 路由器 |
| 9 | Flutter 原生插件 | system_tray / flutter_local_notifications / file_selector 等 |
| 10 | 日志脱敏 | 方案 B：分级脱敏（Prod/Debug/Verbose） |
| 11 | 通知发送方 | Flutter 端发原生通知 |
| 12 | 数据库 | modernc.org/sqlite（纯 Go） |
| 13 | 离线检测 | 方案 B：mDNS + 5 秒心跳 |
| 14 | UI 节流 | 100ms 推送一次进度事件 |
| 15 | 测试策略 | CI 单元+集成；真机 E2E 手动 |
| 16 | 启动时间 | 窗口 ≤ 1.5s，mDNS ≤ 0.5s |
| 17 | 错误码 | 统一大写下划线前缀 |