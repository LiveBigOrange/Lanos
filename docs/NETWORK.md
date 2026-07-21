# Lanos Network Behavior & IPv6

> 配套文档：`PROTOCOL.md`（发现、握手、文件头）；本文聚焦 IPv4 / IPv6 / 双栈寻址与排障。

Lanos 在 P3 阶段完成跨栈寻址能力：本机只装 v4、只装 v6、或双栈都能找到对端可达地址，依据 **RFC 6724** 默认策略表挑出最高优先级的目的/源地址对，再调用 `core/net.Dial` 建 Noise 加密连接。本文描述实现细节、调用层错误码与排障入口。

---

## 1. 地址选择（RFC 6724）

### 1.1 输入

| 来源 | 输入 | 由谁产生 |
|------|------|----------|
| 对端候选目的地址 | `Device.IPv4` + `Device.IPv6` | mDNS A/AAAA 记录（`core/discovery/`） |
| 本机候选源地址 | `discovery.LocalSourceIPs()` | 遍历 `net.Interfaces`，过滤 multicast，保留 loopback / link-local / global |
| 端口 | `Device.Port` | mDNS TXT `port` |

### 1.2 实现

`core/net/addrselect.go::SelectAddresses(dsts, sources, port)`：

1. 解析每个 dst 的 IP、zone、scope、precedence（RFC 6724 §3.1 默认策略表）。
2. 为每个 dst 在 `sources` 中按 §5 简化规则挑出最佳源（兼容版本、同 scope、最长公共前缀）。
3. 按 §6 对 dst 排序：可达 > 不可达 / 同 scope 优先 / 与源最长公共前缀 / 高 precedence 优先 / v6 tiebreak。
4. 输出 `[]AddrPair{Destination, Source, IsV6}`，已剔除无兼容源的不可达候选。

### 1.3 输出与上层调用

`core/api/handlers.go::peerAddress(d)`：

- 调用 `SelectAddresses(Device.IPv4+IPv6, LocalSourceIPs(), Device.Port)`。
- 取 `pairs[0]` 作为 dial 地址，`Version` 字段标 `"4"` 或 `"6"` 用于 UI 提示与日志。
- 无任何候选 IP → `ErrNoPeerAddress`（HTTP 503, code `PEER_UNREACHABLE`）。
- 候选与本地源地址无匹配 IP 家族 → 返回 `ErrIncompatibleIPVersion`（HTTP 503, code `INCOMPATIBLE_IP_VERSION`）。

错误响应体（`writeErrorCode`）：

```json
{
  "error": {
    "code": "INCOMPATIBLE_IP_VERSION",
    "message": "peer has no reachable address: INCOMPATIBLE_IP_VERSION"
  }
}
```

UI 应当在收到该 code 时引导用户切换网络或对端地址族。

---

## 2. `lanos://connect` URI 适用边界

完整 schema 见 `PROTOCOL.md §2`。Go 实现在 `core/net/uri.go::ParseConnectURI`：

- 严格校验：scheme 必须 `lanos://`；path 必须 `connect`；任一未知参数报错。
- `ip` 仅接受 IPv4 字面量（`::1` 写进 `ip` 会被拒）。
- `ip6` 仅接受 IPv6 字面量（`192.0.2.1` 写进 `ip6` 会被拒）。
- 链路本地 IPv6（`fe80::/10`）必须带 zone id：`fe80::1%wlan0`。URI 中 `%` 必须按 RFC 3986 percent-encode 为 `%25`，即 `fe80::1%25wlan0`；解析后还原为 `fe80::1%wlan0`。
- `port` 范围 1..65535；`pk-hash` 32 位小写 hex；`device-name` 必须可 `url.QueryUnescape`。
- `single()` 把重复同参数视为错误。

`ConnectURI.Dests()` 把 `IP`/`IP6` 拼成 `[]string`，可直接喂给 `SelectAddresses`，便于 QR 唤起后立即用同一寻轨路径选择出口地址。

`ConnectURI.String()` 重新生成规范形式（顺序参数、`%` 转义），用于发文方生成二维码。

---

## 3. 跨平台防火墙规则

每平台脚本接受 root/Administrator 权限后写入永久规则。全部开放同一段端口：

| 协议 | 端口 | 用途 |
|------|------|------|
| TCP | 52100–52999 | 直传监听 + Web 共享 |
| UDP | 5353 | mDNS 发现 |

| 操作系统 | 脚本 | 后端 |
|----------|------|------|
| Linux | `scripts/lanos-setup-firewall.sh` | ufw / firewalld / iptables + ip6tables（按可用次序级联） |
| macOS | `scripts/lanos-setup-firewall-macos.sh` | PF（`/etc/pf.anchors/lanos` + `pfctl -a lanos`）v4 + v6 |
| Windows | `scripts/lanos-setup-firewall.ps1` | `New-NetFirewallRule`（Windows Firewall 单规则对 v4/v6 同时生效） |

---

## 4. 诊断接口

`GET /api/v1/diagnostics`（Bearer token，30s 超时组）

返回本机网络栈快照，便于双栈不兼容或链路选择异常时排查：

```json
{
  "ip_version": "46",
  "interfaces": [
    {
      "name": "en0",
      "flags": ["up", "broadcast"],
      "ipv4": ["192.168.1.10"],
      "ipv6": ["fe80::1%en0", "2001:db8::10"],
      "mtu": 1500,
      "hardware": "aa:bb:cc:dd:ee:ff"
    }
  ],
  "source_ips": ["192.168.1.10", "2001:db8::10", "fe80::1%en0", "127.0.0.1", "::1"]
}
```

`ip_version` 取值 `"4" / "6" / "46"`（来自 `discovery.LocalIPVersion()`，仅统计非 loopback、非 link-local 的 unicast 地址）。
`source_ips` 即 `peerAddress()` 喂给 `SelectAddresses` 的源地址池。

调用示例：

```bash
curl -s -H "Authorization: Bearer $LANOS_TOKEN" http://127.0.0.1:52100/api/v1/diagnostics | jq .
```

---

## 5. 已知限制 / 文档一致性

- `PROTOCOL.md §3` 标注握手模式为 Noise **XK**，但实现实际采用 Noise **XX**（受 mDNS 广播 pk-hash 而非全公钥所限，发起方无法预知响应方静态密钥）。以 `core/transport/noise.go` 头注释与代码为准；该文档将在后续迭代统一修订。
- 直传监听端口绑定规则：API 监听 127.0.0.1:cfg.Port，Web 共享在 `:cfg.Port+1`。**P2P transfer listener 尚未接入主入口（`cmd/gcd/main.go`）**——详见 `AGENTS.md` "P2P listener gap"。在补全前 mDNS TXT 中 `port=` 指向的是 loopback API 端口，跨主机实际不可达。本阶段 P3 交付的寻址能力仅影响出站发起（POST `/transfers`），不影响入站接收。
- 链路本地 IPv6 的 zone id 在跨主机场景意义有限：对端若在另一物理接口将无法直接复用 `fe80::1%en0`，应在双栈发布的设备上同时分配 global unicast v6 地址。
- `addrselect` 当前不实现 RFC 6724 §2.2 中所有源选择规则（例如 Rule 7 偏好原生地址、Rule 8 CellNAT）；缺失规则对常见 LAN 场景无影响，未来支持移动流量场景时再补。