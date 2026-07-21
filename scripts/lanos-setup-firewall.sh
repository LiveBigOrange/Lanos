#!/usr/bin/env bash
# lanos-setup-firewall.sh — configures Linux firewall rules for Lanos.
# See PRD §4.4 防火墙与系统集成.
#
# Opens:
#   - 52100-52999/tcp (v4 + v6): direct transfer + web share listeners
#   - 5353/udp (v4 + v6): mDNS discovery
#
# Detects ufw / firewalld / iptables in that order. Requires sudo.
set -euo pipefail

LANOS_TCP_RANGE="52100:52999"
MDNS_UDP="5353"

if [[ $EUID -ne 0 ]]; then
  echo "需要 root 权限，正在通过 sudo 重新执行..." >&2
  exec sudo -E "$0" "$@"
fi

configure_ufw() {
  if ! command -v ufw >/dev/null; then return 1; fi
  echo "→ 配置 ufw"
  ufw allow "${LANOS_TCP_RANGE}/tcp" comment "lanos transfer + share"
  ufw allow "${MDNS_UDP}/udp" comment "lanos mDNS"
  echo "✓ ufw 规则已添加"
  return 0
}

configure_firewalld() {
  if ! command -v firewall-cmd >/dev/null; then return 1; fi
  echo "→ 配置 firewalld"
  firewall-cmd --permanent --add-port="${LANOS_TCP_RANGE}/tcp"
  firewall-cmd --permanent --add-port="${MDNS_UDP}/udp"
  firewall-cmd --reload
  echo "✓ firewalld 规则已添加"
  return 0
}

configure_iptables() {
  if ! command -v iptables >/dev/null; then return 1; fi
  echo "→ 配置 iptables（IPv4 + IPv6）"
  # IPv4
  iptables -I INPUT -p tcp --dport 52100:52999 -j ACCEPT -m comment --comment "lanos"
  iptables -I INPUT -p udp --dport 5353 -j ACCEPT -m comment --comment "lanos mDNS"
  # IPv6
  if command -v ip6tables >/dev/null; then
    ip6tables -I INPUT -p tcp --dport 52100:52999 -j ACCEPT -m comment --comment "lanos"
    ip6tables -I INPUT -p udp --dport 5353 -j ACCEPT -m comment --comment "lanos mDNS"
  fi
  echo "✓ iptables 规则已添加（重启后可能丢失，请使用 ufw/firewalld 或持久化规则）"
  return 0
}

echo "Lanos 防火墙配置工具"
echo ""

configure_ufw || configure_firewalld || configure_iptables || {
  echo "✗ 未检测到 ufw / firewalld / iptables，请手动放行："
  echo "  TCP 52100-52999 (v4 + v6)"
  echo "  UDP 5353 (v4 + v6, mDNS)"
  exit 1
}

echo ""
echo "完成。Lanos 现在可以接收来自局域网内其他设备的连接。"
