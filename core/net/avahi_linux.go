//go:build linux

package net

import (
	"fmt"
	"os/exec"
)

// CheckAvahi verifies that the Avahi daemon is running on Linux.
// Returns nil if avahi-daemon is found, or an error with guidance if not.
func CheckAvahi() error {
	// Check if avahi-daemon is running
	cmd := exec.Command("pidof", "avahi-daemon")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf(`Avahi daemon is not running.

Lanos requires Avahi for mDNS service discovery on Linux.

To start Avahi:
  sudo systemctl start avahi-daemon
  sudo systemctl enable avahi-daemon

To install Avahi:
  Ubuntu/Debian: sudo apt install avahi-daemon
  Fedora:        sudo dnf install avahi
  Arch:          sudo pacman -S avahi

After starting Avahi, restart Lanos.`)
	}
	return nil
}

// CheckFirewall returns a message if common firewall tools are blocking
// Lanos ports. This is informational only; the setup script handles rules.
func CheckFirewall() string {
	// Check if ufw is active
	if out, err := exec.Command("ufw", "status").Output(); err == nil {
		if string(out) != "Status: inactive\n" {
			return "ufw is active. Run: sudo ./lanos-setup-firewall.sh"
		}
	}
	// Check if firewalld is running
	if err := exec.Command("firewall-cmd", "--state").Run(); err == nil {
		return "firewalld is active. Run: sudo ./lanos-setup-firewall.sh"
	}
	return ""
}
