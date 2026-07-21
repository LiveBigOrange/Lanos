# lanos-setup-firewall.ps1 — configures Windows Firewall rules for Lanos.
# See docs/NETWORK.md. Linux uses ./lanos-setup-firewall.sh, macOS uses
# ./lanos-setup-firewall-macos.sh.
#
# Opens:
#   - 52100-52999/tcp (v4 + v6): direct transfer + web share listeners
#   - 5353/udp (v4 + v6): mDNS discovery
#
# Requires Administrator privileges. Run from an elevated PowerShell.

#Requires -RunAsAdministrator

$ErrorActionPreference = "Stop"

$TcpRange = "52100-52999"
$UdpPort = 5353
$RulePrefix = "Lanos"

function Add-LanosRule {
  param(
    [string]$Name,
    [string]$Protocol,
    [string]$LocalPort,
    [string]$Direction = "Inbound",
    [string]$Profile = "Any"
  )
  if (Get-NetFirewallRule -Name $Name -ErrorAction SilentlyContinue) {
    Set-NetFirewallRule -Name $Name -Enabled True -Direction $Direction -Protocol $Protocol -LocalPort $LocalPort -Profile $Profile
    Write-Host "✓ Updated rule '$Name'"
  } else {
    New-NetFirewallRule -Name $Name -DisplayName $Name -Direction $Direction -Protocol $Protocol -LocalPort $LocalPort -Action Allow -Profile $Profile | Out-Null
    Write-Host "✓ Created rule '$Name'"
  }
}

Add-LanosRule -Name "$RulePrefix-Transfer-TCP" -Protocol TCP -LocalPort $TcpRange
Add-LanosRule -Name "$RulePrefix-mDNS-UDP" -Protocol UDP -LocalPort $UdpPort

# Both Windows Firewall v4 and v6 stacks share these rules; the LocalPort
# ranges apply automatically to both IP families (Windows Firewall has a
# single AddressFamily = Any by default).

Write-Host ""
Write-Host "完成。Lanos 现在可以接收来自局域网内其他设备的连接。"
Write-Host "  - TCP $TcpRange (v4 + v6)"
Write-Host "  - UDP $UdpPort  (v4 + v6, mDNS)"