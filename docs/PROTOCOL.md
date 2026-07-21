# Lanos 协议规范（草案 v0.1）

> 配套文档：`PRD_v2_complete.md` V2.1 多端扩展版
>
> 本文件是实施落地时各端必须遵循的精确字节序 / 字段格式约定。所有不一致以本文件为准。

---

## 1. mDNS / DNS-SD 服务发现

### 1.1 服务类型

```
_lanos._tcp.local.
```

### 1.2 TXT Records

| 字段 | 类型 | 必填 | 取值 | 说明 |
|------|------|------|------|------|
| `txt-ver` | string | 是 | `1` | TXT record 格式版本；本规范为 1，未来不向后兼容变更时 +1 |
| `proto` | string | 是 | `lanos/1.0` | 协议标识；major.minor 形式 |
| `platform` | string | 是 | `windows` \| `macos` \| `linux` \| `android` \| `ios` | 发布设备平台 |
| `port` | integer-as-string | 是 | `52100` - `52999` | 直传监听端口 |
| `pk-hash` | hex string | 是 | 32 字符（16 字节 hex，lower） | ed25519 公钥 SHA256 前 16 字节 |
| `device-name` | url-encoded string | 是 | UTF-8 then percent-encode | 设备名；最长 64 字节（解码后） |
| `ip-ver` | string | 是 | `4` \| `6` \| `46` | 支持的 IP 版本 |

### 1.3 广播节奏

| 平台 | 前台 | 后台（≤3min） | 后台（>3min） |
|------|------|---------------|---------------|
| Desktop | 75s | n/a | n/a |
| Android | 75s | 180s（前台服务存活） | 180s（用户未暂停） |
| iOS | 75s | 停止 | 停止 |

TTL：桌面 120s（标准 mDNS），移动端同。

### 1.4 A / AAAA 记录

- `ip-ver=4`：仅注册 A 记录
- `ip-ver=6`：仅注册 AAAA 记录
- `ip-ver=46`：同时注册 A + AAAA

链路本地 IPv6 地址需携带 zone id（如 `fe80::1%wlan0`），mDNS TXT 中**不**直接存地址，地址由 mDNS 协议层的 A/AAAA 记录承载；连接端从记录中取出地址并保留 zone id。

---

## 2. `lanos://connect` URI Scheme

用于隐身模式二维码与跨平台唤起。

```
lanos://connect?ip=<v4>&ip6=<v6>&port=<int>&pk-hash=<hex16>&device-name=<urlenc>
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `ip` | 否 | IPv4 地址，如 `192.168.1.50` |
| `ip6` | 否 | IPv6 地址，链路本地需带 zone id，如 `fe80::1%wlan0` |
| `port` | 是 | 直传监听端口 |
| `pk-hash` | 是 | 32 字符 hex，用于握手前预校验 |
| `device-name` | 是 | URL-encoded 设备名，UI 展示用 |

`ip` 与 `ip6` 至少一个；同时存在时由对端按本机地址族能力择优连接。

---

## 3. Noise XK 握手 + SAS 验证

### 3.1 协议模式

**Noise XK**（flynn/noise 包 `noise.NewHandshake` XK 模式）

- 静态密钥：ed25519 长期密钥对（identity.key）
- 临时密钥：X25519，握手内生成，使用后丢弃
- 加密：chacha20-poly1305
- 哈希：SHA256

### 3.2 握手消息流

```
-> e                          (initiator ephemeral pubkey)
<- e, ee, s, es               (responder ephemeral + static encrypted)
-> s, se                      (initiator static encrypted)
```

1-RTT 完成后双方持有相同加密密钥与握手哈希。

### 3.3 SAS 4 位确认码计算

握手完成后，双方各自计算：

```
code = int.from_bytes(SHA256(handshake_hash), "big") mod 10000
code_str = "%04d" % code
```

`handshake_hash` 由 flynn/noise 包在握手结束时暴露（`CipherState`/`HandshakeState` 的 `ChannelBinding` 或等价 API）。

UI 同步弹出 4 位数 + 对方设备名，用户视觉比对后双方点击"确认"才进入传输阶段。

### 3.4 信任建立

确认后，对方 ed25519 公钥被写入本机 `trusted_devices.json`：

```json
{
  "<device_id_hex_8>": {
    "pubkey": "<ed25519_pubkey_hex>",
    "name": "My MacBook Pro",
    "trusted_at": 1737500000,
    "auto_receive": false
  }
}
```

后续握手发起方静态公钥与 `trusted_devices.json` 中的记录匹配时，跳过 SAS 弹窗，直接进入传输。公钥不匹配（对端重装系统）时降级为陌生设备，重新触发 SAS。

#### 3.4.1 设备标识符（device-id）

每个设备由 ed25519 公钥计算唯一标识：

```
device-id = SHA256(ed25519_pubkey)[:8]  // 前 8 字节
device-id-hex = hex(device-id)           // 16 字符小写 hex
```

用途：
- `trusted_devices.json` 的顶级 key（见 §3.4）
- 日志/事件中的设备标识

持久化策略：`device-id` 不持久化，每次启动从 `identity.key` 在线计算。若 `identity.key` 持久化失败，应重新生成新密钥对并广播新 device-id（旧 device 会降级为陌生设备）。

---

## 4. 直传帧格式

### 4.1 长连接复用

每对设备间建立**一条** Noise 加密长连接，所有传输任务在该连接上以 32-bit stream ID 多路复用。

### 4.2 帧头（每帧固定 16 字节，明文长度之前由 Noise 加密）

```
+--------+--------+--------+--------+
| stream_id (4B BE)                  |
+--------+--------+--------+--------+
| frame_type (1B)                    |
+--------+--------+
| reserved (2B, must be 0)           |
+--------+--------+--------+--------+
| payload_len (4B BE)                |
+--------+--------+--------+--------+
| crc32 (4B BE)                      |
+--------+--------+--------+--------+
```

| frame_type | 值 | 说明 |
|------------|----|------|
| DATA | 0x01 | 数据帧，payload 为文件 chunk |
| META | 0x02 | 传输元数据，payload 为 JSON `{"task_id":"...","file_path":"...","size":N,"relative_path":"..."}` |
| ACK | 0x03 | 接收确认，payload 为 JSON `{"task_id":"...","chunk_idx":N,"ok":true}` |
| END | 0x04 | 传输结束，payload 为 JSON `{"task_id":"..."}` |
| CANCEL | 0x05 | 取消传输，payload 为 JSON `{"task_id":"...","reason":"..."}` |
| ERROR | 0x06 | 错误，payload 为 JSON `{"task_id":"...","code":"...","message":"..."}` |

### 4.3 Chunk

- 固定大小 4 MB（最后一个 chunk 可不足）
- chunk_idx 从 0 开始递增
- 接收方每收到一个 chunk 立即写入 `transfer_cache/<task_id>/chunk_<idx>`，并更新 `transfer_meta/<task_id>.meta`
- 重传时发送方读取 meta，跳过已 ACK 的 chunks

### 4.4 双向并发

A → B 与 B → A 可在同一条长连接上同时进行，使用**不同**的 stream ID 范围：

- 主动发起方分配 stream ID 偶数（0, 2, 4, ...）
- 被动响应方分配 stream ID 奇数（1, 3, 5, ...）

UI 上分别显示上行 / 下行两个独立进度条。

---

## 5. 错误码

见 PRD §5.5。错误码 JSON schema：

```json
{
  "error": {
    "code": "<UPPER_SNAKE_CASE>",
    "message": "<human readable, localized>",
    "details": { "<key>": "<value>" }
  }
}
```

HTTP 状态码映射：

| HTTP | 错误码示例 |
|------|-----------|
| 400 | `CONFIRM_CODE_MISMATCH` `TRANSFER_CANCELED` |
| 401 | `UNAUTHORIZED`（Bearer token 缺失/错误） |
| 403 | `PERMISSION_DENIED`（CORS 拒绝 / 文件无权限） |
| 404 | `DEVICE_OFFLINE` `FILE_NOT_FOUND` `SHARE_EXPIRED` |
| 409 | `TRANSFER_IN_PROGRESS` |
| 410 | `SHARE_EXPIRED`（已过期） |
| 429 | `SHARE_LIMIT_EXCEEDED` `SHARE_DOWNLOAD_LIMIT`（封禁） |
| 500 | `INTERNAL`（兜底） |

---

## 6. 待定字段（实施过程补全）

- [ ] gomobile bind 函数签名清单（P4 W12）
- [ ] SSE 事件 payload schema（P1 W3）
- [ ] 网页分享 HTML 页模板（P2 W6）
- [ ] ZIP 流式打包的 metadata 头（P2 W6）
