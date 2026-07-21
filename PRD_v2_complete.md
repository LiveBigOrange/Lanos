# 局域网跨平台文件共享工具 · MVP 需求文档（V2.1 多端扩展版）

> 版本：V2.1 | 文档状态：已完成深度自检与多端扩展整合 | 适用阶段：MVP 立项完成 → 技术设计 → 实施规划

---

## 一、产品概述

打造一款零配置、全平台、安全私密的局域网文件共享工具，让用户像使用 AirDrop 一样在 **Windows、macOS、Linux、Android、iOS** 五平台之间自由传输文件，原生支持 IPv4/IPv6 双栈，并支持通过浏览器链接向未安装应用的访客分享文件。

**产品理念**：装好即忘，需要时立刻出现，不打扰、不泄露、不依赖互联网。

**平台矩阵**：

| 平台 | 形态 | 角色 | MVP 优先级 |
|------|------|------|------------|
| Windows | 桌面 | 完整对等端 | P0 首发 |
| macOS | 桌面 | 完整对等端 | P0 首发 |
| Linux | 桌面 | 完整对等端 | P0 首发 |
| Android | 移动 | 完整对等端 | P0 同期 |
| iOS | 移动 | **受限对等端**（前台完整对等，后台仅维持活跃传输） | P0 同期 |

> iOS 受限原因：系统后台限制不允许 socket 长期监听；后台时 mDNS 广播被挂起、入站连接被冻结，仅已完成握手的活跃传输可短暂继续。

---

## 二、目标用户与核心场景

**核心用户**：多设备持有者、自由职业者、小团队、技术爱好者。

**核心场景**：

1. 自己的 Mac 和 Windows 台式机之间传大文件，不用 U 盘或聊天软件。
2. 会议室里临时分享文件给同事，对方无需安装任何软件，浏览器或 curl 即可下载。
3. 向客户交付设计稿，生成限时加密下载链接，到期自动失效。
4. 无互联网环境（工地、实验室）下设备间的文件交换。
5. 技术用户在脚本或远程终端中通过命令行拉取分享文件。
6. 手机拍完照/录完视频，**即时把媒体文件原片传到笔记本**做后期（手机→电脑，避免 USB 数据线 + iTunes / Android File Transfer 等繁琐工具）。
7. 出差同事手机里临时要拿到电脑里某份文档（电脑→手机），无网环境下也能跨设备互传。
8. 网络环境只有 IPv6（运营商只发 IPv6 的家庭宽带 / 仅 IPv6 的企业内网）时，桌面与移动间仍可发现并互传。
9. 客户用 Android 手机扫描桌面端生成的下载二维码，在浏览器中拉取大文件，无需安装 App。

---

## 三、核心功能（MVP）

### 3.1 设备自动发现与识别

启动后自动通过 mDNS/DNS-SD 发现同一局域网内运行本软件的其他设备。

**3.1.1 mDNS 广播协议**

- 服务类型：`_lanos._tcp.local.`
- **每 75 秒广播一次**（桌面端，标准 mDNS TTL 120s 的一半，保持在线状态刷新）
- **移动端广播节流**：iOS/Android 在前台时按 75 秒广播；进入后台后：
  - Android（前台服务存活）：节流为 180 秒，平衡可见性与电量
  - iOS：后台广播停止，依赖对端主动连接或被拉起（见 3.6.3）
- TXT records 格式（**分层广播策略**，不广播完整公钥避免设备指纹追踪）：

| 字段 | 说明 | 示例 |
|------|------|------|
| `txt-ver` | TXT record 格式版本 | `1` |
| `proto` | 协议标识 | `lanos/1.0` |
| `platform` | 平台 | `windows` / `macos` / `linux` / `android` / `ios` |
| `port` | 直传监听端口（每设备首次启动从 52100-52999 随机选一个，持久化到 config.yaml） | `52103` |
| `pk-hash` | 公钥 SHA256 前 16 字节 hex | `3f8a2b1c...` |
| `device-name` | 设备名（URL encoded） | `My%20iPhone%2015` |
| `ip-ver` | 支持的 IP 版本 | `4` / `6` / `46`（双栈） |

- **IPv6 双栈广播**：在 `ip-ver=46` 时，设备在 mDNS 中**同时注册 A 记录（IPv4）和 AAAA 记录（IPv6）**。接收方按对方宣告的能力选择连接地址族；仅 IPv4 网络的设备只注册 A 记录，仅 IPv6 网络的设备只注册 AAAA 记录。
- **链路本地地址处理**：IPv6 链路本地地址 `fe80::`%interface 需携带 zone id（如 `fe80::1%wlan0`），协议层用方括号包裹 `[fe80::1]:52103` 传输，连接端必须保留 zone id。

完整公钥在实际直传连接时通过 Noise XK 握手第一轮消息携带，接收方校验 `pk-hash` 一致后才接受连接。

**3.1.2 设备列表展示**

- 自定义设备名、系统图标（自动识别 `platform` 字段对应 OS）、在线状态。
- 同名设备处理：自动追加设备短 ID 后缀（`My-MacBook-a3f2`），避免 UI 混淆。

**3.1.3 首次传输确认码（SAS 机制）**

两台设备第一次传输时，双方弹窗展示一个 4 位随机确认码，用户核对一致后方可传输，防止误发至陌生设备。

**握手流程**（采用 SAS + 双向确认方案）：

```
1. 双方通过 mDNS 交换公钥 hash
2. 发起方建立 Noise XK 握手，第一轮消息携带临时 DH 公钥
3. 接收方回握手消息，携带自己的临时 DH 公钥
4. 双方各自计算：code = (sha256(DH_result) mod 10000)，得到一致 4 位数
5. 双方 UI 同步弹出 code + 对方设备名，用户视觉比对数字与设备名后点击"确认"
6. 确认后才进行后续传输握手
```

- **超时机制**：30 秒未点确认自动取消，双方均收到通知。
- **碰撞应对**：4 位数在 50+ 设备网络中有中等碰撞概率，因此 UI 强制双重核对（数字 + 设备名），大幅降低实际风险。
- **信任建立**：确认后，对方公钥被记入 `trusted_devices.json`，后续传输不再弹确认码。

**3.1.4 离线检测**

采用 **mDNS + 5 秒心跳双重检测**：

- mDNS TTL 隐式离线感知：上限 120 秒
- 主动心跳：每 5 秒 ICMP ping，连续 3 次（15 秒）无响应标记离线
- 离线后 UI 头像标灰，保留 5 分钟后从列表移除（便于网络抖动恢复）

**3.1.5 可信设备管理**

可将某设备标记为"信任"，来自该设备的传输可设为自动接收（不再弹确认码和接收确认）。信任关系可在设置中查看/移除。可信设备列表上限 128 个，存储于 `trusted_devices.json`。可信设备公钥若发生变更（对端重装系统后重新生成密钥），自动降级为陌生设备并提醒用户重新确认。

**3.1.6 隐身模式连接流程**

开启隐身模式后本设备不广播 mDNS，但仍可主动连接已知设备（通过 IP 或二维码）。

- 隐身设备生成包含完整连接信息的二维码，内容自定义 URI scheme：
  ```
  lanos://connect?ip=192.168.1.50&ip6=fe80::1%wlan0&port=52103&pk-hash=3f8a2b1c...&device-name=My%20Mac
  ```
  - `ip` 为 IPv4 地址，`ip6` 为 IPv6 地址（链路本地需带 zone id），任一可选；同时存在让对端按本机网络能力择优连接。
  - 移动端扫码场景：手机浏览器中打开 `lanos://connect?...` 链接，若已安装 Lanos 则唤起 App，否则引导下载页。
- 对端扫描此二维码后手动加入设备列表，触发标准 Noise XK 握手 + SAS 确认（首次仍需 4 位数核对，防止连错设备）
- 隐身模式的设备在对方列表中以特殊标记（🌙 图标）展示

**3.1.7 设备名变更实时生效**

用户修改设备名后立即触发一次 mDNS 重新注册（不等下次 75 秒广播周期），对端设备 3 秒内更新列表中的设备名显示。

**3.1.8 IPv6 双栈支持**

MVP 原生支持 IPv4、IPv6 及双栈网络，确保在仅 IPv6 网络下仍可正常工作。

- **监听双栈**：Go core 直传监听与网页分享监听都用 `net.Listen("tcp", "[::]:port")` 一次绑定双栈，IPv4 与 IPv6 入站连接共享同端口
- **mDNS 声明**：见 3.1.1，通过 `ip-ver` TXT 字段宣告本机能力
- **连接地址族选择**：发起方按对端 TXT 中宣告的 `ip-ver` 与本机可用地址族求交：
  - 双方都 `46`：优先 IPv6（`[fe80::1]:port` 或全局地址）连接；失败回退 IPv4
  - 对端仅 `6` / 本机仅 `4`（或反之）：直接失败，UI 报"网络协议不兼容"
- **地址选择算法**：依据 RFC 6724 默认优先级（全局 IPv6 > IPv4 > 链路本地 IPv6）；多个候选地址时按延迟择优
- **NAT64/DNS64**：不依赖 DNS，mDNS 直接给地址，故 NAT64 翻译对本协议透明（无需特殊处理）
- **隐私**：不广播 MAC 地址；IPv6 用 EUI-64 派生地址时也仅作传输，不写入磁盘
- **诊断**：设置页"网络信息"显示当前所有监听 IP 与地址族，方便排查
- **椭圆网络（ Carrier-grade NAT / IPv6-only 蜂窝）**：本机连入 IPv6-only 蜂窝时，mDNS 仅在 Wi-Fi 同子网生效，蜂窝端无对等方（符合"局域网"边界，不做互联网中继，见 6.2）

---

### 3.2 端到端加密直传（App to App）

**3.2.1 发送方式**

- 主界面拖拽文件/文件夹到目标设备头像
- 系统文件管理器右键菜单"发送到附近设备"（调用 App 并弹出设备选择窗口）
- 支持将文件拖拽到托盘图标触发发送（此时弹出设备选择窗口）
- 主界面 "+" 按钮点击弹出系统文件选择对话框（作为拖拽的 fallback，照顾触摸板/无鼠标用户）
- 右键托盘 → "发送剪贴板文本"：自动将文本保存为临时 `.txt` 文件进入发送流程

**3.2.2 接收体验**

接收方弹出提示框，显示发送方设备名、文件列表、总大小（文件夹可展开结构）。

- 用户可选择接收或拒绝；若 30 秒内无操作，自动拒绝并通知发送方。
- 支持设置默认操作：始终询问 / 自动接收信任设备 / 自动接收所有设备。

**3.2.3 加密协议：Noise XK**

选择 **Noise XK 模式** + ed25519 密钥对（不使用 TLS 证书）。

| 维度 | TLS 自签证书 | Noise XK（选定） |
|------|-------------|----------|
| 首次连接 | 需用户确认证书指纹 | 确认码直接嵌入密钥确认机制 |
| 握手开销 | 2-RTT + Certificate | 1-RTT，无证书 |
| 依赖 | x509 证书管理 | 仅需 ed25519 密钥对 |
| 算法 | - | ed25519 + chacha20-poly1305 |

**3.2.4 传输过程**

- 整体进度条 + 当前传输文件名显示 + 实时速度与剩余时间（`12.3 MB/s · 剩余 2 分 15 秒`），大文件流畅不卡死。
- 速度计算采用滑动窗口平均（最近 2 秒内 bytes/s），< 1 MB/s 时只显示速度不显示 ETA。
- 文件夹传输时支持冲突处理：目标路径已有同名文件时，按用户设置执行（覆盖 / 跳过 / 保留两者，默认"跳过"并提示）。
- **保留两者命名规则**：统一 `file_1.ext`、`file_2.ext`（下划线 + 序号，跨平台一致，shell 友好），序号检测从 1 开始跳过已存在序号。

**3.2.5 传输队列与并发**

- **队列模型**：每个目标设备维护独立传输队列，不同设备间并行。
- **单设备串行**：同一时刻向同一设备只传一个任务，其余排队（避免接收方 UI 过载）。
- **同时收发并行**：A 给 B 发送时，B 也可同时拖拽文件给 A。每对设备间建立一条长连接，在其上多路复用双向传输（每个传输分配 32-bit stream ID），UI 分别显示上下行两个独立进度条。
- **优先级**：网页分享的 HTTP 下载 > 用户拖拽发起的直传（确保访客下载体验顺畅）。
- **并发上限**：最大同时处理传输任务 4（上行）+ 4（下行）；网页分享单分享最多 3 个并发 HTTP 连接。
- **取消操作**：发送方和接收方均可取消进行中的传输，取消后发送方清理临时数据。

**3.2.6 传输状态机**

```
待确认（发送方等待接收方确认码确认）
  │
  ▼
连接中（Noise XK 加密通道握手）
  │
  ▼
传输中（分块 4MB chunk 发送 + 进度回调）
  │
  ├──► 已完成
  ├──► 已取消（任意方主动取消）
  ├──► 已失败（网络断开 / 超时）
  └──► 待续传（失败后可手动重试，保留已接收片段）
```

**3.2.7 续传设计（MVP 简化版）**

- 分块大小：固定 4MB 一个 chunk
- 进度记录：接收方每收到一个 chunk 写入 `transfer_cache/<task_id>/`，记录已完成 chunk 索引列表到 `transfer_cache/<task_id>.meta`
- 重试机制：失败后 UI 显示"重试"按钮；重试时发送方读取 meta 文件跳过已完成 chunks
- 边界：MVP 仅支持手动重试（不自动重连），且接收方未点击"接收"前不提前建立连接

**3.2.8 传输完成**

- 系统通知"接收成功"，点击通知可打开文件所在目录或直接打开文件。
- 所有接收记录自动保存到"接收记录"中。
- **通知合并策略**：单次任务（可能含多个文件）只触发一个通知，例如"从 AI-MacBook 收到 5 个文件（共 234 MB）"+ 点击打开下载目录；避免批量传输收到 N 个独立通知造成打扰。
- **通知由 Flutter 端发出**（Go core 通过 SSE 推送事件 `{"type":"notify",...}` 给 Flutter，Flutter 调用原生通知 API，因为 Flutter 持有用户会话和通知权限）。
- **接收超时本地提示**：接收方 30 秒内未操作自动拒绝时，本地不弹通知（避免打扰）；发送方在共享记录中看到状态栏显示"对方未响应"。

**3.2.9 跨平台文件路径与权限处理**

- **路径映射**：传输时只传**相对路径**（去掉盘符前缀）。发送方 Windows 路径 `C:\Users\me\folder\file.txt` → 传 `<relative>/folder/file.txt`，接收方按下载目录 + 相对路径存放
- **权限保留**：MVP 不保留原文件 unix 权限，接收方一律用 `0644`（文件）/ `0755`（目录）；V2 考虑保留权限
- **跨平台非法字符**：传输到 Windows 时，将文件名中的 `<>:"/\|?*` 替换为 `_`，并在接收通知/记录中告知"已替换 N 个文件名中的非法字符"
- **文件名长度超限**：超过目标文件系统上限（255 字符）时自动截断为 248 字符并加 `_truncated` 后缀

**3.2.10 特殊文件类型传输**

| 文件类型 | 处理策略 |
|----------|----------|
| symbolic link | **默认不跟随**：复制 symlink 本身（保留语义）。设置中可选"展开 symlink 内容"。跨平台传到 Windows 时，NTFS symlink 保留为普通文件 |
| hard link | 跟随复制文件内容 |
| FIFO / device file / socket | 拒绝传输并提示"不支持的文件类型"。不阻塞同任务其他文件的传输 |
| Windows `.lnk` 快捷方式 | 当普通文件传输 |
| 空文件 / 空目录 | 正常支持传输 |
| 隐藏文件（`.` 开头） | 正常支持传输，不特殊处理 |

**3.2.11 启动残留恢复与取消清理**

异常退出或崩溃后 `transfer_cache/` 可能残留 chunks：

- **启动清理**：每次启动自动扫描 `transfer_cache/`，发现未完成的 `<task_id>/` 直接删除整个目录及其 `.meta` 文件（已知丢弃，MVP 不做自动恢复）
- **对应记录处理**：在"接收记录"中将该任务标记为"已中断"状态（保留记录，不保留片段）
- **取消清理**：用户主动取消 50% 进度的传输时，立即删除 `transfer_cache/<task_id>/`（已知丢弃，下次重传从头开始）
- **MVP 进度持久化策略**：所有进行中的传输在 App 强制退出后**优雅失败**为"已取消"状态，下次启动不自动续传；自动续传为 V2 范围

**3.2.12 多文件批量拖拽**

明确支持将多个零散文件一次性拖拽到设备头像：

- UI 接收方提示框显示为"5 个文件 + 文件夹 1 个"（数量汇总）
- 确认后整体作为一个任务进入传输队列（同设备仍串行处理，但 chunk 合并连贯）
- 进度条按总字节数计算总进度，下方列表显示每个文件当前状态（待传 / 传输中 / 已完成 / 失败）

**3.2.13 设备名与文件名校验**

- **设备名**：1-32 字符，允许 emoji 与任意 Unicode，超长自动截断；mDNS TXT record 体积有 200 字节上限，URL encoded 后保留余量
- **文件名**：保留原样传输，接收时按 3.2.9 规则处理非法字符与超长
- **分享密码**：4-32 字符可见 ASCII（数字、字母、符号），不限复杂度（用户自定）

---

### 3.3 网页链接分享（访客下载）

用户可在已安装 App 的设备上，右键文件（或主界面入口）→ "生成下载链接"。

**3.3.1 服务架构：单一 HTTP 服务路由所有 token**

一个 HTTP 服务监听一个端口，按路径路由所有活跃分享：

```
/dl/<token>            — 文件下载（校验 token + 可选密码）
/api/share/<token>/status — 返回文件名、大小、剩余次数/时间（JSON）
/qr/<token>            — 二维码页面（HTML，可选）
```

不采用每链接独立服务的方案（端口资源有限、同时分享上限受限于端口数、资源清理复杂）。

系统在本地启动一个临时 HTTP 服务（MVP 不使用 HTTPS，自签证书会导致浏览器警告，局域网 + 128 位随机 token 已有足够安全保障），生成含随机 token 的链接：

`http://本机IP:端口/dl/随机token`

- **IPv6 链接**：当本机仅 IPv6 或对端访问者更优走 IPv6 时，链接用 `[IPv6]:端口` 包裹地址（如 `http://[fe80::1%wlan0]:52103/dl/...` 或全局地址去掉 zone id）。展示在 UI 上时也提供二维码与复制按钮
- **移动端作为发布方**：手机端也可生成分享链接，由 Go core 内嵌进程直接在小随机端口监听 HTTP；访客在另一台手机或电脑浏览器输入链接即可下载（移动端进入分享状态会影响电量，UI 上提示"分享期间会增加耗电"）

**3.3.2 分享设置**

- 有效期：10 分钟 / 30 分钟 / 1 小时 / 12 小时 / 24 小时
- 下载次数限制：1 次 / 5 次 / 10 次 / 无限次
- 访问密码（可选，为空则无需密码；密码经哈希存储在本地内存中，不落盘）

**3.3.3 界面呈现**

- 显示下载链接，一键复制
- 显示二维码（由 Flutter 端用 `qr_flutter` 包生成，纯 Dart 无原生依赖；二维码内容 = 完整下载 URL）
- 命令行示例标签页：展示 `curl -O "链接"` 或 `wget "链接"`，方便技术用户复制使用

**3.3.4 访问体验**

- 任何浏览器或 HTTP 客户端（curl/wget）访问链接，校验 token 和密码后即可下载
- 浏览器访问时返回极简 HTML 下载页：
  - 文件名 + 文件大小（文件夹显示"ZIP 压缩包，共 N 个文件"）
  - 密码输入框（如有设密码）
  - 下载按钮 + 进度提示
  - 剩余下载次数 / 有效时间倒计时
- **文件夹打包下载**：分享文件夹时服务端动态生成 ZIP 流并发送，浏览器端收到压缩包

**3.3.5 ZIP 流式与大文件夹处理**

- 方案：`archive/zip` + `io.Pipe` 边压缩边发送
- 中文文件名：ZIP 条目使用 `utf-8` + `language encoding flag (0x0800)` 确保跨平台解压正常
- 4GB 上限提示：分享文件夹总大小超过 4GB 时提示"部分浏览器可能无法处理"，允许继续
- 单次 ZIP 流式传输超时 30 分钟自动断开

**3.3.6 同时分享上限**

- 默认同时活跃分享：64 个
- 达到上限后提示"已达同时分享上限（64），请先停止一些分享后再试"
- 设置中可调整（建议范围 16-256）

**3.3.7 安全控制**

- 分享者可随时在"共享记录"中手动停止分享，token 立即失效
- 达到有效期或下载次数上限后自动停止服务并清理资源
- App 退出后所有分享链接自动失效，下次启动不残留

**3.3.8 密码复杂度与防枚举**

- **密码规则**：4-32 字符可见 ASCII，不限复杂度，用户自定；密码经 SHA256 + per-share salt 哈希后存内存，不落盘
- **token 防枚举**：单 IP 连续 10 次访问不存在的 token 后临时封禁该 IP 5 分钟，返回 429 状态码
- **token 熵值**：32 字节（256 位）随机数，URL-safe base64 编码后 43 字符；穷举爆破 2^256 不可行
- **密码爆破防护**：单 IP 连续 10 次密码错误后临时封禁该 IP 5 分钟
- **路径穿越防护**：token 路径校验必须为已知分享 id 列表中的值，拒绝任何带 `..` 或绝对路径的输入

---

### 3.4 个人传输记录管理

分为两个并列面板，都位于主界面的侧边栏或标签页中。

**3.4.1 共享记录（我发出的）**

记录本机发起的所有直传任务和网页分享。

- 每条记录显示：文件名/文件夹名、分享方式（直传/链接）、对方设备名（直传时）或链接状态（等待下载/传输中/已完成/已过期/已停止）、剩余时间或次数
- 可操作项：复制链接（链接分享）、停止分享、删除记录

**3.4.2 接收记录（我收到的）**

记录本机接收到的所有直传文件/文件夹。

- 每条记录显示：文件/文件夹名、来自哪台设备、接收时间、保存路径、文件大小
- 可操作项：打开文件、打开所在文件夹、删除记录

**3.4.3 记录管理增强**

- **搜索**：列表上方搜索框，按文件名模糊匹配
- **批量操作**：支持多选 → 批量删除
- **排序**：按时间 / 文件大小 / 设备名排序
- **导出 CSV**：设置页提供"导出传输记录为 CSV"
- **保留策略**：默认保留最近 1000 条，超出后异步删除最旧记录（不阻塞 UI），接收的文件本身不删除；设置中可选"永久保留" / "保留最近 100 / 1000 / 10000 条"
- 记录存储于 `transfer_log.db`（SQLite，见 5.2.3）

---

### 3.5 设置与个性化

- **设备名称**：可自定义，默认使用电脑主机名
- **下载保存路径**：可修改，默认为系统下载文件夹/用户指定
- **自动接收策略**：始终询问 / 仅信任设备自动接收 / 所有设备自动接收
- **文件夹传输冲突处理**：覆盖 / 跳过 / 保留两者
- **网页分享端口**：可设置首选端口或端口范围（如 52000-53000），或选择"自动"
- **隐身模式**：开启后本设备不通过 mDNS 广播自己，但仍可主动连接已知设备（通过 IP 或二维码）
- **网络接口选择**：多网卡时可指定使用哪个网络（如仅 Wi-Fi）
- **同时分享上限**：默认 64，范围 16-256
- **语言**：下拉切换简体中文 / English，立即生效（不重启），使用 `flutter_localizations` + 运行时 `setLocale`
- **通知开关**（三类统一控制，不细分单个文件）：
  - ☑ 接收文件通知（有新文件收到时）
  - ☑ 传输完成通知（自己发送完成时）
  - ☑ 链接状态通知（分享过期/被下载时）
- **传输记录保留策略**：永久保留 / 最近 100 / 1000 / 10000 条
- **当前版本**：设置页底部显示版本号 + "检查更新"按钮（详见第 9 节更新机制）
- **网络模式偏好**（移动端用户可见）：仅 Wi-Fi / Wi-Fi + 蜂窝（默认仅 Wi-Fi，避免蜂窝流量）

---

### 3.6 移动端支持（Android / iOS）

移动端作为**完整对等端**参与：mDNS 发现、直传收/发、网页分享生成、可信设备管理、传输记录、设置入口。无系统托盘 / 右键菜单等桌面专属能力；交互以触屏范式重新设计。

**3.6.1 进程模型：Go core 内嵌**

移动端无法运行独立的 Go 后台进程。改为：

- 用 **`gomobile bind`** 将 Go core 编译为 iOS Framework / Android AAR，由 Flutter 插件层直接调用 Go 函数（无 HTTP 桥，无 stdout 握手）
- Flutter ↔ Go core 在同进程内通过 Dart FFI + Go 导出函数通信，省去 5.1.3 的 stdout 启动握手与 5.1.5 的 Bearer token 鉴权（鉴权在内存边界内天然成立）
- API 形态：Go 侧暴露与桌面 REST API 等价的 Go 函数 `Lanos_ListDevices() ...`，Flutter 用 `MethodChannel` 包装；事件用 Go 注册回调 → Dart `EventChannel` 推送（替代桌面 SSE）

**3.6.2 前后台行为矩阵**

| 平台 | 前台 | 后台（≤3 分钟） | 后台（>3 分钟） |
|------|------|----------------|-----------------|
| Android | 完整对等端：广播 + 监听 + 收发 | **前台服务**持续广播节流为 180s、保持 TCP 监听 | 仍保活，电量低时可用户选"省电"暂停广播 |
| iOS | 完整对等端：广播 + 监听 + 收发 | mDNS 广播被系统挂起；进行中传输可短暂续传（≤30s） | 全部停止，回前台后自动恢复发现 |

- **Android 前台服务**：通过 `flutter_local_notifications` 显式声明"附近设备监听中"持久通知（用户可关），保证 mDNS 广播与入站 TCP 不被系统杀掉
- **iOS 受限对等端**：后台不能拉起；桌面端用户可在桌面 UI 中"显式 ping 唤醒"（iOS 端仅当 App 在前台时响应；类似 AirDrop 在锁屏+蓝牙开时才被发现，Lanos iOS 需 App 在前台）

**3.6.3 文件访问**

| 平台 | 选择文件 | 写入文件 | 读取权限 |
|------|---------|---------|---------|
| Android (API ≥ 30) | `file_picker` 走 SAF（Storage Access Framework），选中文件返回 `content://` URI | 写入用户指定的 SAF 目录（推荐 Downloads/Lanos/） | `READ_EXTERNAL_STORAGE` / `MANAGE_EXTERNAL_STORAGE` 仅在用户选"全盘模式"时申请；MVP 默认 SAF |
| iOS | `file_picker` 走 UIDocumentPicker，返回 `app-extension/...` 安全书签 | 写入 App Container Documents 目录；用户在"文件"App 可见 | 无需特殊权限；首次访问 iCloud Drive 文件按系统弹窗授权 |

- **SAF URI 复用**：保留 SAF 树 URI 持久化权限（`takePersistableUriPermission`），重启后仍可读写
- **大文件流式**：移动端不做"先全量拷贝到 App 缓存再传"的反模式，直接从 SAF URI 流式读 → Noise 连接 → 对端写盘，省一倍磁盘空间与时间

**3.6.4 触屏交互差异**

- 主界面：设备列表用**头部大头像**展示替代桌面端密集列表；下拉刷新手动触发"重新发现"
- 发送：底部 sheet 选择"从相册选 / 文件 App 选 / 文件夹"，无右键菜单与拖拽
- 接收：全屏弹窗替代桌面端小弹窗，按钮至少 44pt（符合 HIG/Material）
- 通过系统 share sheet：把图片视频直接"用 Lanos 发送"（Android `Intent.ACTION_SEND` / iOS Share Sheet 扩展）

**3.6.5 网络与省电策略**

- 默认仅在 Wi-Fi 下启用发现与监听；蜂窝下用户手动开启后才广播（设置项见 3.5 末尾）
- 后台超过 10 分钟无设备交互时自动暂停广播，回前台或被系统唤醒再恢复
- 低电量（< 20%）提示关闭后台监听

**3.6.6 分享链接访问**

访客无需 App 即可在手机浏览器中扫码访问桌面端生成的分享链接，移动端 App 还可同时生成自己的分享链接（手机变成临时 HTTP 服务器，端口与桌面端规则一致 52100-52999，通过二维码分发）。

### 4.1 性能与资源

**量化性能基准**（桌面端·千兆以太网）：

| 指标 | 目标 | 测量方法 |
|------|------|----------|
| 单文件传输速度 | ≥ 80 MB/s | 1GB 文件，TCP 吞吐量 |
| 100 个文件批量传输速度 | ≥ 40 MB/s | 100 个 10MB 文件，含握手+加密 |
| 内存占用（空闲） | ≤ 40 MB | go 进程 rss + flutter 进程 rss |
| 内存占用（传输中） | ≤ 120 MB | 传输 10GB 文件期间峰值 |
| CPU 占用（传输中） | ≤ 1 核 | ed25519 + chacha20 软件实现 |
| 网页分享首次响应 | ≤ 500ms | 从 HTTP 请求到首字节 |
| 设备发现延迟 | ≤ 3s | 从启动到列表中看到同局域网已在线设备 |
| 双击图标到窗口可见 | ≤ 1.5s | Flutter desktop 启动 + 第一帧渲染 |
| Go core 启动并开始 mDNS 广播 | ≤ 0.5s | Go 进程启动到 `registerService()` 完成 |

**量化性能基准**（移动端·Wi-Fi 6 / 5GHz）：

| 指标 | 目标 | 测量方法 |
|------|------|----------|
| 手机→电脑传输速度 | ≥ 25 MB/s | 1GB 视频文件，跨 Wi-Fi |
| 100 个照片批量传速度 | ≥ 15 MB/s | 100 个 3MB JPEG，含握手+加密 |
| 内存占用（空闲） | ≤ 30 MB | Android procfs rss / iOS memory footprint |
| 内存占用（传输中） | ≤ 80 MB | 传输 2GB 视频期间峰值 |
| 启动到第一帧 | ≤ 2s | 含 gomobile 初始化 + mDNS 注册 |
| 移动端发起发现到看到设备 | ≤ 3s | 含权限弹窗已通过场景 |

**IPv6 性能基线**：IPv6 单栈局域网场景，桌面端单文件传输速度应不低于 IPv4 场景的 85%（链路开销与地址族选择成本可忽略）。
| 设备列表出现第一台在线设备 | ≤ 3s | 从启动到 `device.online` 事件首次触发 |

CPU 占用空闲接近 0；支持单文件大于 10GB，内存使用稳定。

**可靠性目标**：
- 连续传输 100 个文件零失败
- 传输过程中 Flutter UI 不卡顿（帧率 ≥ 30fps，通过 Go core 进度推送 100ms 节流实现）
- 单次会话不重启连续运行 7 天不崩溃

### 4.2 易用性与交互

- **界面极简**：主窗口设备列表 + 发送/接收提示，无技术术语
- **首次引导**（3 步页，仅首次安装展示，可在设置中"重新查看引导"）：
  - **Step 1**：欢迎 → "Lanos 让局域网传文件像 AirDrop 一样简单"
  - **Step 2**：功能介绍 → 三张插图卡片（发现设备 / 拖拽发送 / 生成链接）
  - **Step 3**：设置 → 设备名 + 下载路径 + 自动接收策略 + 开机自启
- **系统集成**：
  - 支持开机自启（可选）。各平台实现：Windows 写注册表 `HKCU\...\Run`；macOS 写 LaunchAgent plist；Linux 写 `~/.config/autostart/*.desktop`
  - 系统托盘/菜单栏图标常驻，显示在线状态和网页分享指示灯
  - 系统原生通知（发送完成、收到新文件、链接过期等）
- **拖拽支持**：支持将文件拖入主窗口、拖到设备头像、拖到托盘图标

### 4.3 安全与隐私

- **加密**：所有 App 间传输强制 Noise XK + chacha20-poly1305，证书自签，通过 4 位 SAS 确认码建立初次信任。
- **网页分享 token**：128 位以上随机数，防止猜测；密码哈希存储在本地内存，不落盘。
- **密钥存储**：每台设备首次启动生成 ed25519 密钥对
  - Windows: `%APPDATA%/Lanos/identity.key`（DPAPI 加密）
  - macOS: `~/Library/Application Support/Lanos/identity.key`（Keychain 封装）
  - Linux: `~/.config/lanos/identity.key`（0600 权限）
  - Android: App 私有目录 `identity.key`，配合 Android Keystore 包装对称密钥加密存储；私钥本身不进 Keystore（Keystore 不支持 ed25519 通用导入）
  - iOS: App Keychain 中以 Data Class `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` 存储；私钥备份用户禁用
- **隐私声明**：在应用内明确告知用户：本软件无账号体系、不收集数据、文件不经过任何中间服务器。
- **日志分级脱敏**（默认 `ProdLevel`，通过环境变量 `LANOS_LOG_LEVEL` 切换）：

| 级别 | 默认 | 文件名脱敏规则 |
|------|------|---------------|
| Prod | 是 | 所有日志中文件名 → `<FILE>`，路径 → `<PATH>` |
| Debug | - | `report_***.pdf`（保留后缀 + 前 6 字符 + ***） |
| Verbose | - | 原始输出，仅开发环境 |

脱敏发生在写入前（非输出层过滤），避免日志文件残留敏感信息；支持导出日志用于故障排查。

### 4.4 稳定性与容错

- **网络变化适应**：监听系统网络变化事件，Wi-Fi 切换、IP 变更后自动重新广播发现、更新分享链接中的 IP；IPv6 地址变更（RA 重新分配）同样触发
- **传输中断处理**：失败后保留已接收片段，提示手动重试（自动重连续传为 V2 范围）
- **端口冲突处理**：网页分享启动时自动检测端口，被占用则选择下一可用端口；退出时优雅关闭监听，释放端口
- **异常退出保护**：分享链接在 App 退出后自动全部失效，下次启动不残留
- **单实例控制**：启动时尝试创建临时文件锁（`%TEMP%/lanos.lock` 或对应平台临时目录），若锁已存在且持有进程存活则弹出"Lanos 已在运行"并退出；不同用户账户下可同时运行各自实例
- **移动端后台生存**：
  - Android：前台服务（`foregroundServiceType=dataSync`）维持 mDNS 广播与 TCP 监听；系统 Doze 时通过用户豁免名单提示"加入电池优化白名单"
  - iOS：后台 socket 监听被冻结不可避免；活跃传输用 `URLSession backgroundSession`（短时间）+ 回前台续传；mDNS 广播靠前台保活
- **IPv6 邻居发现受阻**：部分企业 Wi-Fi 开启 client isolation，IPv6 邻居广播与 mDNS 多播都被丢包。检测方法：30 秒内未发现已知在线设备时，提示"当前网络可能禁用多播/隔离客户端，请尝试 IP 直接连接或二维码直连"
- **防火墙与系统集成**：

| 平台 | 端口放行策略 | 代码实现 |
|------|-------------|----------|
| Windows | 首次启动自动添加放行规则（TCP + mDNS，v4 + v6 双栈） | CGO 调用 `FirewallAddApp` 或 shell `netsh advfirewall` 分别对 IPv4/IPv6 添加规则 |
| macOS | 系统自动弹窗询问，用户点"允许" | 无需额外操作 |
| Linux | 检测 `ufw`/`firewalld`/`iptables`，提示手动放行或自动执行 | 提供 `lanos-setup-firewall.sh` 脚本，同时开放 `52100-52999/tcp` v4+v6 与 mDNS `5353/udp` |
| Android | 无系统防火墙；接入 Wi-Fi 时若路由器有 AP isolation，仅靠同 App 直连 mDNS | 无需代码处理 |
| iOS | 无系统防火墙；同 Android | 无需代码处理 |

所有桌面平台网页分享端口范围（52000-53000）需要在防火墙中开放（v4+v6），设置向导中引导。

- **信号处理**：除 `POST /shutdown` 外，Go core 需处理 SIGINT/SIGTERM（Linux/macOS）/ Ctrl-C 事件（Windows），确保退出时清理分享链接、释放端口、保存未完成任务状态。移动端在 App 生命周期回调 `onTerminate` / `applicationWillTerminate` 中调用等价清理函数。

### 4.5 跨平台一致性

- UI 使用 Flutter 确保 Windows、macOS、Linux、Android、iOS 五端视觉与逻辑共享同一份 Dart 代码，桌面端共享约 90%、移动端共享约 75%（差异在文件选择与后台策略），同时尊重平台习惯（如 Mac 菜单栏、Windows 托盘右键菜单、Android Material 3 大头像列表、iOS Large Title Navigation）
- **安装包格式**：

| 平台 | 格式 | 备注 |
|------|------|------|
| Windows | exe / msi | MSIX 为 V2 |
| macOS | dmg（Universal Binary arm64+x86_64） | 不做 notarization 的安装包签名说明见 5.6 |
| Linux | AppImage（首发）+ .deb（Debian/Ubuntu）+ .rpm（Fedora/RHEL） | 三种全部 MVP 内提供 |
| Android | apk（侧载）+ aab（Google Play） | minSdk 24（Android 7.0），targetSdk 34 |
| iOS | ipa | min iOS 15.0（覆盖后台上架要求）；TestFlight 公测分发 |

- **Linux 桌面深度适配**：
  - **Avahi 依赖**：mDNS 在 Linux 上依赖 `avahi-daemon` 守护进程；`grandcat/zeroconf` 通过 DBus 与 Avahi 通信
    - 启动顶时检测 `avahi-daemon` 是否运行（`pidof avahi-daemon`）；未运行时弹出说明："mDNS 发现需要 Avahi 守护进程，请安装并启用：`sudo apt install avahi-daemon && sudo systemctl enable --now avahi-daemon`"
    - 在 README、AppImage 启动脚本、deb/rpm 的 `Depends:` 中写明 `avahi-daemon` 推荐/依赖
  - **桌面环境兼容矩阵**：

| 桌面 | 系统 | 状态 |
|------|------|------|
| GNOME 40+（无 AppIndicator） | Ubuntu 22+ | 提示安装 `gnome-shell-extension-appindicator`，或回退到常驻最小化窗口 |
| KDE Plasma | Kubuntu / openSUSE | 原生支持系统托盘，无需额外组件 |
| XFCE / Cinnamon / MATE | Linux Mint / Debian | 原生 libappindicator 支持 |
| WSL2 / 服务器无 X | - | MVP 不支持无图形桌面，未安装 X/Wayland 时退化为 CLI-only ?（V2 考虑） |

  - **应用菜单条目**：deb/rpm 安装时附带 `/usr/share/applications/lanos.desktop`（含图标、Name、MimeType 关联），让用户可右键文件"用 Lanos 发送"
  - **图标主题**：随包内置 SVG 图标到 `/usr/share/icons/hicolor/scalable/apps/lanos.svg`，多分辨率适配
- **移动端 UI 差异化**：
  - 头部 favicon 大头像列表（Material 3 风格）替代桌面端密集列表
  - 全屏弹窗 + Bottom Sheet 替代桌面端窗口弹窗
  - 无右键菜单、无系统托盘；通过系统 Share Sheet 接入"分享到 Lanos"
  - 触屏点击区 ≥ 44×44 dp，符合 iOS HIG / Material 触控目标

---

## 五、技术架构

### 5.1 整体架构：Go Core + Flutter UI

**5.1.1 进程间通信方案**

采用 **本地 HTTP REST API**（Go 作为 HTTP 服务端，Flutter 作为客户端）：

```
┌──────────────────┐      HTTP REST API      ┌──────────────────────┐
│  Flutter UI       │ ◄──────────────────────► │  Go Core Daemon      │
│  (桌面图形界面)     │     localhost: 随机端口   │  (mDNS/传输/分享服务)  │
│                  │       JSON over HTTP     │                      │
│  进程 B           │                          │  进程 A               │
└──────────────────┘                          └──────────────────────┘
```

| 方案 | 优势 | 劣势 | 结论 |
|------|------|------|------|
| **HTTP API（选此）** | 解耦彻底，Go 崩溃不影响 UI，可独立调试、热重启；支持第三方客户端 | 环回 HTTP 有轻微开销 | **MVP 选型** |
| cgo/FFI 编译为共享库 | 无进程间通信开销，单进程部署 | 三端交叉编译复杂，任一语言 panic 导致整个进程崩溃 | 不采用 |
| Unix Socket / stdin/stdout | 比 HTTP 更轻量 | 跨平台差异大（Windows 无 Unix Socket）；结构化程度低 | 不采用 |

**5.1.2 生命周期管理**

- **Go Core Daemon（gcd）**：独立后台进程，不在用户会话中（因此不发系统通知，由 Flutter 代发）
- **启动方式**：Flutter 启动时通过 `Process.start` 自动拉起 gcd；若已运行则复用
- **退出处理**：Flutter 窗口关闭时可选 (a) 保留 gcd 后台运行（托盘模式）或 (b) 发送 `POST /shutdown` 优雅退出
- **健康检查**：Flutter 每 5 秒 `GET /ping`，连续 3 次无响应则重启 gcd

**5.1.3 Go core ↔ Flutter 启动握手协议**

由于"自动选端口"策略，gcd 监听端口是动态的，Flutter 需在启动后获取。流程：

1. Flutter 启动 gcd 子进程，传入专属 stdout pipe（Flutter 读、gcd 写）
2. gcd 启动时选定端口并生成 32 字节随机 `API_TOKEN`（用作本地 API 鉴权，见 5.1.5）
3. gcd 完成监听后向 stdout 输出一行 JSON：
   ```json
   {"port":52103,"api_token":"<base64>","version":"0.1.0"}
   ```
4. Flutter 解析后开始用此 port + token 调 API
5. **超时判定**：5 秒内未收到此行则判定 gcd 启动失败，UI 报错并允许"重试启动"
6. **冲突场景处理**：若 gcd 已在运行（单实例锁提示），gcd 立即向 stdout 输出 `{"already_running":true,"port":...,"api_token":...}` 让 Flutter 直接连入

**5.1.4 API 设计原则**

- RESTful，`/api/v1/` 前缀，请求/响应均为 JSON
- 关键事件（文件接收状态、传输进度）通过 Server-Sent Events（SSE）`GET /api/v1/events` 推送，Flutter 用 `EventSource` 接收
- **进度事件节流**：Go core 在 chunk 接收循环中累积进度，最多每 100ms 推送一次（不每 chunk 一次），保证 10GB 文件传输时 Flutter UI 60fps 不卡顿
- SSE 事件类型：
  - `device.online` / `device.offline`
  - `transfer.request`（接收方收到发送请求）
  - `transfer.progress`（进度更新，100ms 节流）
  - `transfer.complete` / `transfer.failed`
  - `share.downloaded` / `share.expired`
  - `notify`（Go core 通知 Flutter 弹原生通知）

**5.1.5 本地 API 鉴权与跨域防护** ⚠️ 必须实现

所有 `/api/v1/*` 请求必须在 HTTP Header 中携带 `Authorization: Bearer <API_TOKEN>`，否则返回 401。`API_TOKEN` 在 Go core 启动时随机生成 32 字节（base64 编码 43 字符），通过启动握手协议（5.1.3）传递给 Flutter，**不写入文件**（仅内存中持有，进程退出即失效）。

| 威胁 | 防护措施 |
|------|----------|
| 本地其他进程恶意调 API（停止分享/读 token/删记录） | Bearer Token 鉴权，token 仅 Flutter 持有 |
| 浏览器跨站请求（CSRF / CORS 攻击） | `Access-Control-Allow-Origin` 仅允许 `http://localhost` 和 `http://127.0.0.1`，拒绝其他 Referer |
| 子进程被偷 token | gcd 退出时清理临时文件 `lanos.endpoint`（如启用文件回退方案） |
| token 被嗅探（root 用户） | 仅绑定 `127.0.0.1` 监听，不暴露到局域网 |

**例外端点**：`GET /api/v1/ping`（健康检查）不要求鉴权，但不返回敏感数据。

### 5.2 技术选型

**5.2.1 后端核心（Go）**

| 用途 | 选型 | 说明 |
|------|------|------|
| HTTP 框架 | 标准库 `net/http` + `chi` 路由器 | chi 仅 200KB，纯 Go，零反射；API 数量约 20 个端点不需要 gin/echo 的重型栈 |
| mDNS | **`grandcat/zeroconf`** | API 简洁、社区活跃度更高、文档完善；优于 `hashicorp/mdns`。集成前做三平台兼容测试 |
| Noise 协议 | `flynn/noise` | ed25519 + chacha20-poly1305 |
| HTTP 客户端 | 标准库 `net/http` | - |
| SQLite 驱动 | `modernc.org/sqlite` | **纯 Go 无 CGO**，交叉编译零成本；性能比 CGO 版慢 2-3 倍但本场景足够 |
| YAML 配置 | `gopkg.in/yaml.v3` | - |
| 日志 | `log/slog`（标准库，Go 1.21+） | 含分级脱敏 hook，支持文件滚动（见 5.7） |
| 移动端 Go 绑定 | `gomobile bind -target=ios/android` | 把 Go core 编译为 iOS Framework / Android AAR，Flutter 通过 FFI 同进程调用 |
| 移动端 mDNS | `grandcat/zeroconf` 同上 | iOS 用 `net.SRV` 类机制 / Android 直接挂前台服务调用同库；不引第二套依赖 |

> **桌面端独有**：`chi` 路由器、SSE、Bearer Token 鉴权仅在桌面端启用（Go core 独立进程模式）。移动端 Go core 编入应用进程，不暴露本地 HTTP API，通信通过 `gomobile bind` 生成的绑定（见 3.6.1 与 5.1.3 例外说明）。

**5.2.2 用户界面（Flutter）**

| 能力 | 推荐插件 | 备选 |
|------|---------|------|
| 系统托盘 | `system_tray`（跨三平台） | `tray_manager` |
| 原生通知 | `flutter_local_notifications` | `local_notifier`（桌面专门） |
| 开机自启 | `launch_at_startup`（Win/macOS） | Linux 用 `~/.config/autostart/*.desktop` 文件操作（约 20 行） |
| 文件对话框 | `file_selector`（官方维护） | `file_picker` |
| 二维码生成 | `qr_flutter`（纯 Dart） | - |
| 系统信息 | `device_info_plus`（官方） | - |
| 路径处理 | `path`（官方） | - |

原则：一律使用 pub.dev 上 **官方/高 star 包**，避免早夭项目。

**5.2.2.1 移动端 Flutter 额外插件**

| 能力 | 推荐插件 | 平台 |
|------|---------|------|
| 相册选择 | `image_picker`（官方） | Android + iOS |
| 文件选择（含 SAF） | `file_picker` 或 `file_selector` | Android 用 SAF content:// URI |
| iOS 文件目录书签 | `path_provider` + 调用 Security-Scoped Bookmark API | iOS |
| Android 前台服务 | Flutter 自定义平台通道调用 `startForeground()` | Android |
| iOS 后台 URLSession | Flutter 平台通道调用 `URLSession backgroundSession` | iOS |
| 触控屏幕扫码 | `mobile_scanner`（ML Kit / Vision 包装） | Android + iOS |
| URI Scheme 唤起 | `uni_links` 或官方 Android intent / iOS Universal Link | Android + iOS |

**5.2.3 数据库：SQLite（`modernc.org/sqlite`）**

选 SQLite 而非 JSON 的理由：
- 传输记录按时间序频繁写入和检索，SQLite 性能远优于读写 JSON 文件
- 支持按时间排序、按设备筛选等查询
- 选 `modernc.org/sqlite` 而非 `mattn/go-sqlite3`：纯 Go 无 CGO，交叉编译零成本
- 不选 bbolt：键值结构和 SQLite 表结构差不多，但查询能力不如 SQL

### 5.3 数据目录结构

```
用户数据根目录：
  Windows: %APPDATA%/Lanos/
  macOS:   ~/Library/Application Support/Lanos/
  Linux:   ~/.config/lanos/

├── identity.key         # ed25519 私钥（平台加密）
├── config.yaml          # 用户设置（设备名、下载路径、策略、端口等）
├── trusted_devices.json # 可信设备列表（公钥 → 设备名 map）
├── transfer_cache/      # 正在接收的临时文件片段
│   └── <task_id>/
│       ├── chunk_00001
│       ├── chunk_00002
│       └── ...
├── transfer_meta/       # 传输元数据（续传状态）
│   └── <task_id>.meta
├── transfer_log.db      # 共享记录 + 接收记录（SQLite）
└── logs/                # 调试日志（脱敏）
    └── lanos_20260721.log
```

**移动端数据目录**：

| 平台 | 根目录 | 接收文件默认目录 | 备份策略 |
|------|--------|------------------|----------|
| Android | `getExternalFilesDir()/Lanos/` 或内部 `getFilesDir()/Lanos/`（私有） | SAF 持久 Uri 推荐到 `Download/Lanos/`，调用 `takePersistableUriPermission` | Android 自动备份排除 `identity.key` 与 `transfer_cache/`（`allowBackup="false"` 或 `fullBackupContent` 排除规则） |
| iOS | `NSDocumentDirectory/Lanos/` 或 `NSApplicationSupportDirectory` | `UIDocumentPickerViewController` + 安全书签持久化（Universal Scope 或 target IP，App 退出仍保留权限） | `identity.key` 写入 Keychain 而非文件；`config.yaml` 设置 `NSURLIsExcludedFromBackupKey` 限制 iCloud 备份 |

### 5.4 API 接口清单

**设备**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/devices` | 获取在线设备列表 |
| POST | `/api/v1/devices/:id/trust` | 标记为可信 |
| DELETE | `/api/v1/devices/:id/trust` | 移除可信 |
| GET | `/api/v1/devices/trusted` | 可信设备列表 |

**传输**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/transfers/send` | 发起直传（指定设备 + 文件路径） |
| POST | `/api/v1/transfers/:id/accept` | 确认接收 |
| POST | `/api/v1/transfers/:id/reject` | 拒绝 |
| POST | `/api/v1/transfers/:id/cancel` | 取消 |
| GET | `/api/v1/transfers/:id/status` | 传输状态 |
| POST | `/api/v1/transfers/:id/retry` | 重试失败传输 |

**网页分享**

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/shares` | 创建分享 |
| GET | `/api/v1/shares` | 我的共享记录列表 |
| POST | `/api/v1/shares/:id/stop` | 停止分享 |
| DELETE | `/api/v1/shares/:id` | 删除记录 |

**事件 + 系统**

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/events` | SSE 事件流 |
| GET | `/api/v1/ping` | 健康检查 |
| GET | `/api/v1/settings` | 获取设置 |
| PUT | `/api/v1/settings` | 更新设置 |
| POST | `/api/v1/shutdown` | 优雅退出 |
| GET | `/api/v1/logs` | 获取日志（脱敏） |
| GET | `/api/v1/version` | 当前版本 |

> **移动端 API 路径**：以上 HTTP 端点仅适用于桌面端（Go core 独立进程模式）。移动端通过 `gomobile bind` 生成等价 Go 函数（如 `Lanos_ListDevices()`、`Lanos_Send(deviceID, paths)`、`Lanos_CreateShare(...)` 等），函数签名与表内 HTTP 路径一一对应，参数与返回值用 protobuf 或 Go struct 序列化为 Dart 类型的 JSON 桥接；事件推送用 Go callback → Dart EventChannel，与桌面端 SSE 等价。详见 3.6.1。

### 5.5 错误码体系

统一前缀 + 全大写下划线，Flutter 端建立 `error_code.dart` 枚举按 code 分支展示对应中/英文 i18n 提示。

```go
const (
    ErrCodeDeviceOffline         = "DEVICE_OFFLINE"
    ErrCodeTransferInProgress    = "TRANSFER_IN_PROGRESS"
    ErrCodeShareExpired          = "SHARE_EXPIRED"
    ErrCodeShareDownloadLimit    = "SHARE_DOWNLOAD_LIMIT"
    ErrCodePortUnavailable       = "PORT_UNAVAILABLE"
    ErrCodeFileNotFound           = "FILE_NOT_FOUND"
    ErrCodePermissionDenied       = "PERMISSION_DENIED"
    ErrCodeConfirmCodeMismatch   = "CONFIRM_CODE_MISMATCH"
    ErrCodeConfirmCodeTimeout    = "CONFIRM_CODE_TIMEOUT"
    ErrCodeTransferRejected      = "TRANSFER_REJECTED"
    ErrCodeTransferCanceled      = "TRANSFER_CANCELED"
    ErrCodeShareLimitExceeded    = "SHARE_LIMIT_EXCEEDED"
    ErrCodeTrustedDeviceLimit    = "TRUSTED_DEVICE_LIMIT"
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

### 5.6 安装包与签名

| 平台 | 安装包 | 签名/公证 | 用户安装方式 |
|------|--------|-----------|-------------|
| Windows | `Lanos-Setup-x.y.z.exe`（Inno Setup）+ `Lanos-x.y.z.msi`（可选） | 可选 code signing cert（EV/OV），未签名时 Windows SmartScreen 警告由用户点"仍运行" | 双击安装，桌面快捷方式 + 开机自启选项 |
| macOS | `Lanos-x.y.z.dmg`（Universal Binary arm64+x86_64） | **MVP 不做 notarization**：用户首次打开需"系统设置 → 隐私安全 → 仍打开"，README 文档 accompanying | 拖入 Applications；首次启动右键打开绕过 Gatekeeper |
| Linux | `Lanos-x.y.z.AppImage` + `.deb` + `.rpm` | AppImage 内嵌 desktop 文件；deb/rpm 声明 `Depends: avahi-daemon`；不做 GPG 签名（V2） | AppImage 直接可执行；deb/rpm 包管理器安装 |
| Android | `Lanos-x.y.z.apk`（侧载）+ `Lanos-x.y.z.aab`（Play） | APK 自签名 debug/release keystore；aab 走 Google Play 应用签名 | 开发者分发或 Play 商店 |
| iOS | `Lanos-x.y.z.ipa`（侧载/内部 release）+ TestFlight | Apple Developer 账号签名 + Notarization（iOS 强制）；Enterprise/Personal Team 仅内部 | TestFlight 公测链接；企业内部分发可选 |

> macOS notarization 与 Windows code signing 待 MVP 验收后视用户反馈再决定是否购置证书（成本与时间投入权衡，非技术阻塞）。

### 5.7 日志与文件滚动

- **日志框架**：Go core 用 `log/slog`（标准库，Go 1.21+），Flutter 端用自研轻量 file logger
- **日志文件位置**：<数据目录>/logs/
  - Linux/macOS: `~/.config/lanos/logs/` 或 `~/Library/Application Support/Lanos/logs/`
  - Windows: `%APPDATA%/Lanos/logs/`
  - Android: `getExternalFilesDir()/Lanos/logs/`
  - iOS: `NSDocumentDirectory/Lanos/logs/`（关闭 iCloud 备份）
- **滚动策略**：按天 `yyyy-MM-dd.log` 切割；单文件 ≥ 10 MB 强制滚动到 `lanos_yyyyMMdd_N.log`；保留最近 7 天日志；超过 100 MB 自动清理最旧
- **脱敏**：写入前在 slog handler 内对 path/filename 做替换（见 4.3），不通过输出层过滤
- **导出诊断**：设置 → "导出诊断包"按钮，把最近 7 天日志 + 网络信息 + 设备列表快照打包为 zip（脱敏后的）供用户提交给开发者

---

## 六、MVP 明确边界

### 6.1 包含（Must Have）

- 上述所有标为"核心功能"的描述，包括设备发现、直传、网页分享、共享/接收记录、设置、首次引导、移动端 3.6 节、IPv6 双栈 3.1.8 节
- 安装包支持 **Windows、macOS、Linux（AppImage + deb + rpm）三端首发**，**Android（apk + aab）与 iOS（ipa + TestFlight）同期**
- 界面语言：先支持简体中文和英文（移动端随系统语言切换）
- ✅ Go Core 作为独立进程管理（桌面端）
- ✅ Go Core 嵌入应用进程（移动端 gomobile bind）
- ✅ 单实例锁
- ✅ 防火墙自动/半自动配置（含 IPv4 + IPv6 双栈规则）
- ✅ 支持 **IPv4 + IPv6 双栈局域网**（双栈、单栈 IPv4、单栈 IPv6 均可工作）
- ✅ 手动检查更新
- ✅ 记录搜索 + 批量删除
- ✅ 通知三开关
- ✅ 首次引导 3 步页（移动端简化为 2 步：欢迎 → 权限申请）

### 6.2 不包含（Nice to Have / V2）

- ❌ 用户注册、云端同步、中心化服务器
- ❌ 管理员全局仪表盘、全团队文件管理
- ❌ 文件夹实时双向同步、版本管理
- ❌ 远程互联网中继传输（需要服务器，违反"纯局域网"边界）
- ❌ 剪贴板文本直接分享（可通过发送 .txt 文件变通）
- ❌ 复杂 Web 管理后台
- ❌ 自动更新 / delta 更新
- ❌ 自动续传（MVP 仅手动重试）
- ❌ 网页分享 HTTPS（HTTP + token 在局域网环境中可接受）
- ❌ cgo / FFI 方案（仅在桌面端；移动端 gomobile bind 不算此条）
- ❌ Windows MSIX 打包（V2 考虑）
- ❌ macOS notarization（待 MVP 后视反馈决定）
- ❌ iOS 后台长期 socket 监听（系统限制不可避免，仅在 WebSocket / URLSession 之内做有限续传）
- ❌ WSL2 / 无 X 服务器的纯 CLI 模式（V2 考虑）
- ❌ Linux flatpak / snap 打包（V2 考虑）

---

## 七、用户故事简要流程

**第一天使用**：安装后打开，看到引导。勾选开机启动，设置设备名。另一台电脑也安装后，双方立刻在主界面看到对方头像。

**首次发送**：拖拽一个文件到对方头像，双方屏幕上弹出 4 位数字 + 对方设备名，核对一致点击确认。文件开始加密传输，进度条走完（含 `12.3 MB/s · 剩余 2 分 15 秒` 实时速度），通知弹出，点击通知打开文件。

**分享给客人**：右键一个项目文件夹，选择"生成下载链接"，设置 1 小时有效、密码 1234。界面显示链接和二维码，同事手机扫码，输入密码，浏览器开始下载 zip 压缩包。同时"共享记录"中出现该分享，状态为等待下载。同事下载完成后，状态变为已完成。发送者也可以提前手动停止。

**双向传输**：A 正在给 B 发送大文件（进度条显示中），此时 B 拖拽一个文件到 A 的头像，立即进入并行传输，A 和 B 的界面分别显示上下行两个独立进度条，互不阻塞。

**日常管理**：周末整理文件，进入"接收记录"，用搜索框按文件名模糊匹配，多选批量删除不需要的记录，点击打开路径定位文件。

---

## 八、测试与质量保障

### 8.1 测试策略

| 层级 | 工具 | 覆盖范围 | CI 集成 |
|------|------|----------|--------|
| 单元测试 | Go `testing` | 加密、端口分配、token 生成、文件名规则、冲突处理、地址族选择 | 必须 |
| 集成测试 | Go + 本地回环 | 两个 Go 实例在 `127.0.0.1` 与 `::1` 上互传（IPv4 + IPv6 同测） | 必须 |
| 集成测试（Linux） | docker container `avahi-daemon` 启用 | 测试 Linux Avahi 依赖路径与 deb/rpm 安装基线 | 必须 |
| 移动端单元 | Go `gomobile bind` 测试 + Flutter `flutter test` | gomobile 接口签名、Flutter widget、SAF/Keychain 适配 | 必须 |
| 移动端集成 | Android emulator API 34 / iOS simulator iOS 17+ | gomobile 调用 + 桌面端跨平台传输 | 手动为主 |
| E2E 测试 | 物理机/VM | 局域网真实发现 + 跨平台传输（含 IPv6-only 网络） | 手动为主 |
| UI 测试 | Flutter `integration_test` | 拖拽、按钮点击、设置页、移动端 Bottom Sheet、扫码 | 可选 |

**集成测试技巧**：在同一测试进程里启动两个 Go core 实例，分别绑定 `[::1]:52100` 与 `[::1]:52101`（IPv6）和 `127.0.0.1:52100` 与 `127.0.0.1:52101`（IPv4），互相发现并传输，验证双栈回退行为；可针对仅 IPv4 / 仅 IPv6 模式做单独矩阵。

**CI 配置**：GitHub Actions 矩阵 `os=[ubuntu-22.04, macos-13, windows-2022]`，跨平台真机测试由团队内部定期做"传文件 bug hunt"。Android CI 用 `macos-latest` 跑 emulator，iOS CI 跑 simulator（Apple Silicon runner）。

**Avahi 测试基线**：CI 中跑 `sudo apt install -y avahi-daemon && sudo service avahi-daemon start` 后再启动测试，确保 Linux mDNS 路径在 RTC 与容器中均工作。

---

## 九、更新机制

- **MVP 方案**：手动检查更新
  - 桌面端（Windows/macOS/Linux）设置页底部显示当前版本号 + "检查更新"按钮
  - 点击后请求 GitHub Releases API（`api.github.com/repos/xxx/lanos/releases/latest`）
  - 发现新版本时，打开浏览器跳转到下载页（用户手动下载替换）
- **移动端更新**：
  - Android Play Store / iOS App Store 自动负责更新，App 内不实现独立更新逻辑
  - 侧载 APK / TestFlight 之外的 Beta 渠道仅在 GitHub Release 页面附说明文档，App 内不打开浏览器跳转下载（移动端体验差且 Google/Apple 政策限制）
- **暂不实现**：自动下载安装、增量更新、Windows `squirrel` 等自动更新框架、Linux AppImage 自更新（AppImage Update + zsync）（V2 考虑）

---

## 十、修订记录

| 日期 | 版本 | 变更 |
|------|------|------|
| 2026-07-21 | V2.0 | 初版 |
| 2026-07-21 | V2.0 完善版 | 集成 17 项补充与选型决策（架构、传输框架、网页分享架构、安全协议、防火墙集成、单实例、更新机制、体验细节、性能指标、MVP 边界、数据目录、API 清单、错误码、测试策略、mDNS 协议、SAS 握手、并发与续传、Linux 托盘兼容） |
| 2026-07-21 | V2.1 多端扩展版 | 集成 IPv6 双栈、Linux 深度适配、Android/iOS 移动端、本地 API 鉴权 + 启动握手、gomobile bind 集成模式、5.6 安装签名、5.7 日志滚动、MVP 边界与平台矩阵升级 |