#!/usr/bin/env bash
# lanos-setup-firewall.sh — configures macOS PF firewall rules for Lanos.
# See docs/NETWORK.md. Linux uses ./lanos-setup-firewall.sh, Windows uses
# ./lanos-setup-firewall.ps1.
#
# Opens:
#   - 52100-52999/tcp (v4 + v6): direct transfer + web share listeners
#   - 5353/udp (v4 + v6): mDNS discovery
#
# Uses pfctl + an anchor (LANOS) under /etc/pf.anchors. Requires sudo.
set -euo pipefail

LANOS_TCP_RANGE="52100:52999"
MDNS_UDP="5353"

if [[ $EUID -ne 0 ]]; then
  echo "需要 root 权限，正在通过 sudo 重新执行..." >&2
  exec sudo -E "$0" "$@"
fi

ANCHOR_FILE="/etc/pf.anchors/lanos"
ANCHOR_LINE='anchor "lanos"'

mkdir -p "$(dirname "$ANCHOR_FILE")"

cat > "$ANCHOR_FILE" <<EOF
# Lanos firewall rules — added by lanos-setup-firewall-macos.sh
# IPv4 + IPv6 inbound for direct transfer listeners and mDNS.

# Direct transfer + web share listeners (TCP 52100-52999)
pass in inet  proto tcp from any to any port 52100:52999
pass in inet6 proto tcp from any to any port 52100:52999

# mDNS discovery (UDP 5353)
pass in inet  proto udp from any to any port 5353
pass in inet6 proto udp from any to any port 5353
EOF

PF_CONF="/etc/pf.conf"
if ! grep -qF "$ANCHOR_LINE" "$PF_CONF" 2>/dev/null; then
  # Insert anchor load line at the top of pf.conf via a temp file.
  TMP="$(mktemp)"
  echo "$ANCHOR_LINE" | cat - "$PF_CONF" 2>/dev/null > "$TMP"
  mv "$TMP" "$PF_CONF"
fi

pfctl -v -f "$PF_CONF"
pfctl -a "lanos" -f "$ANCHOR_FILE"

echo "✓ macOS PF rules loaded: TCP ${LANOS_TCP_RANGE}, UDP ${MDNS_UDP} (v4 + v6)"