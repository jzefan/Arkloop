package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

type GuestNetworkRequest struct {
	Interface   string   `json:"interface"`
	GuestCIDR   string   `json:"guest_cidr"`
	Gateway     string   `json:"gateway"`
	Nameservers []string `json:"nameservers,omitempty"`
}

func configureGuestNetwork(req GuestNetworkRequest) error {
	iface := strings.TrimSpace(req.Interface)
	if iface == "" {
		return fmt.Errorf("interface is required")
	}
	if _, _, err := net.ParseCIDR(strings.TrimSpace(req.GuestCIDR)); err != nil {
		return fmt.Errorf("invalid guest_cidr: %w", err)
	}
	if ip := net.ParseIP(strings.TrimSpace(req.Gateway)); ip == nil {
		return fmt.Errorf("invalid gateway")
	}
	for _, ns := range req.Nameservers {
		if ip := net.ParseIP(strings.TrimSpace(ns)); ip == nil {
			return fmt.Errorf("invalid nameserver %q", ns)
		}
	}
	commands := [][]string{
		{"ip", "link", "set", "dev", iface, "up"},
		{"ip", "addr", "flush", "dev", iface},
		{"ip", "addr", "add", strings.TrimSpace(req.GuestCIDR), "dev", iface},
		{"ip", "route", "replace", "default", "via", strings.TrimSpace(req.Gateway), "dev", iface},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("run %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	if len(req.Nameservers) == 0 {
		return nil
	}
	var builder strings.Builder
	for _, ns := range req.Nameservers {
		builder.WriteString("nameserver ")
		builder.WriteString(strings.TrimSpace(ns))
		builder.WriteString("\n")
	}
	if err := os.WriteFile("/etc/resolv.conf", []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("write resolv.conf: %w", err)
	}
	return nil
}

