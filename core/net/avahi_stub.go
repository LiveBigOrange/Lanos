//go:build !linux

package net

// CheckAvahi is a no-op on non-Linux platforms.
func CheckAvahi() error { return nil }

// CheckFirewall is a no-op on non-Linux platforms.
func CheckFirewall() string { return "" }
