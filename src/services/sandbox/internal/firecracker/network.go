package firecracker

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const DefaultNetworkInterfaceID = "eth0"

type NetworkConfig struct {
	AllowEgress     bool
	EgressInterface string
	TapPrefix       string
	AddressPoolCIDR string
	Nameservers     []string
}

type GuestNetwork struct {
	InterfaceID   string
	InterfaceName string
	HostDevice    string
	GuestMAC      string
	GuestCIDR     string
	Gateway       string
	Nameservers   []string
	hostCIDR      string
	subnet        netip.Prefix
}

type NetworkManager struct {
	cfg          NetworkConfig
	mu           sync.Mutex
	pool         netip.Prefix
	nextSubnet   uint32
	maxSubnets   uint32
	activeByKey  map[string]GuestNetwork
	activeSubnet map[netip.Prefix]string
}

func NewNetworkManager(cfg NetworkConfig) (*NetworkManager, error) {
	pool, err := netip.ParsePrefix(strings.TrimSpace(cfg.AddressPoolCIDR))
	if err != nil {
		return nil, fmt.Errorf("parse firecracker tap cidr: %w", err)
	}
	pool = pool.Masked()
	if !pool.Addr().Is4() {
		return nil, fmt.Errorf("firecracker tap cidr must be ipv4")
	}
	if pool.Bits() > 30 {
		return nil, fmt.Errorf("firecracker tap cidr must be /30 or larger pool")
	}
	maxSubnets := uint32(1) << uint32(30-pool.Bits())
	if maxSubnets == 0 {
		return nil, fmt.Errorf("firecracker tap cidr has no usable /30 subnets")
	}
	return &NetworkManager{
		cfg:          cfg,
		pool:         pool,
		maxSubnets:   maxSubnets,
		activeByKey:  make(map[string]GuestNetwork),
		activeSubnet: make(map[netip.Prefix]string),
	}, nil
}

func (m *NetworkManager) ValidateHost(ctx context.Context) error {
	if _, err := exec.LookPath("ip"); err != nil {
		return fmt.Errorf("ip command not found: %w", err)
	}
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return fmt.Errorf("/dev/net/tun unavailable: %w", err)
	}
	if strings.TrimSpace(m.cfg.TapPrefix) == "" {
		return fmt.Errorf("tap prefix is required")
	}
	if m.cfg.AllowEgress {
		if _, err := exec.LookPath("iptables"); err != nil {
			return fmt.Errorf("iptables command not found: %w", err)
		}
		if _, err := net.InterfaceByName(strings.TrimSpace(m.cfg.EgressInterface)); err != nil {
			return fmt.Errorf("egress interface %s not found: %w", m.cfg.EgressInterface, err)
		}
		value, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
		if err != nil {
			return fmt.Errorf("read ip_forward: %w", err)
		}
		if strings.TrimSpace(string(value)) != "1" {
			return fmt.Errorf("net.ipv4.ip_forward must be enabled")
		}
	}
	return nil
}

func (m *NetworkManager) Setup(ctx context.Context, key string) (GuestNetwork, error) {
	m.mu.Lock()
	if existing, ok := m.activeByKey[key]; ok {
		m.mu.Unlock()
		return existing, nil
	}
	network, err := m.allocateLocked(key)
	m.mu.Unlock()
	if err != nil {
		return GuestNetwork{}, err
	}
	if err := m.configureHost(ctx, network); err != nil {
		m.mu.Lock()
		delete(m.activeByKey, key)
		delete(m.activeSubnet, network.subnet)
		m.mu.Unlock()
		_ = m.cleanupHost(context.Background(), network)
		return GuestNetwork{}, err
	}
	return network, nil
}

func (m *NetworkManager) Release(ctx context.Context, key string) error {
	m.mu.Lock()
	network, ok := m.activeByKey[key]
	if ok {
		delete(m.activeByKey, key)
		delete(m.activeSubnet, network.subnet)
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return m.cleanupHost(ctx, network)
}

func (m *NetworkManager) allocateLocked(key string) (GuestNetwork, error) {
	for attempt := uint32(0); attempt < m.maxSubnets; attempt++ {
		index := (m.nextSubnet + attempt) % m.maxSubnets
		subnet, hostAddr, guestAddr, err := subnetAt(m.pool, index)
		if err != nil {
			return GuestNetwork{}, err
		}
		if _, inUse := m.activeSubnet[subnet]; inUse {
			continue
		}
		m.nextSubnet = index + 1
		tapName := fmt.Sprintf("%s%x", m.cfg.TapPrefix, index)
		if len(tapName) > 15 {
			tapName = tapName[:15]
		}
		network := GuestNetwork{
			InterfaceID:   DefaultNetworkInterfaceID,
			InterfaceName: DefaultNetworkInterfaceID,
			HostDevice:    tapName,
			GuestMAC:      macForIP(guestAddr),
			GuestCIDR:     netip.PrefixFrom(guestAddr, subnet.Bits()).String(),
			Gateway:       hostAddr.String(),
			Nameservers:   append([]string(nil), m.cfg.Nameservers...),
			hostCIDR:      netip.PrefixFrom(hostAddr, subnet.Bits()).String(),
			subnet:        subnet,
		}
		m.activeByKey[key] = network
		m.activeSubnet[subnet] = key
		return network, nil
	}
	return GuestNetwork{}, fmt.Errorf("firecracker tap address pool exhausted")
}

func (m *NetworkManager) configureHost(ctx context.Context, network GuestNetwork) error {
	commands := [][]string{
		{"ip", "tuntap", "add", "dev", network.HostDevice, "mode", "tap"},
		{"ip", "addr", "replace", network.hostCIDR, "dev", network.HostDevice},
		{"ip", "link", "set", "dev", network.HostDevice, "up"},
	}
	for _, args := range commands {
		if err := runHostCommand(ctx, args...); err != nil {
			return err
		}
	}
	if !m.cfg.AllowEgress {
		return nil
	}
	forwardOut := []string{"iptables", "-w", "-A", "FORWARD", "-i", network.HostDevice, "-o", m.cfg.EgressInterface, "-j", "ACCEPT"}
	forwardIn := []string{"iptables", "-w", "-A", "FORWARD", "-i", m.cfg.EgressInterface, "-o", network.HostDevice, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	masquerade := []string{"iptables", "-w", "-t", "nat", "-A", "POSTROUTING", "-s", network.subnet.String(), "-o", m.cfg.EgressInterface, "-j", "MASQUERADE"}
	for _, args := range [][]string{forwardOut, forwardIn, masquerade} {
		if err := runHostCommand(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

func (m *NetworkManager) cleanupHost(ctx context.Context, network GuestNetwork) error {
	var errs []string
	if m.cfg.AllowEgress {
		for _, args := range [][]string{
			{"iptables", "-w", "-D", "FORWARD", "-i", network.HostDevice, "-o", m.cfg.EgressInterface, "-j", "ACCEPT"},
			{"iptables", "-w", "-D", "FORWARD", "-i", m.cfg.EgressInterface, "-o", network.HostDevice, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
			{"iptables", "-w", "-t", "nat", "-D", "POSTROUTING", "-s", network.subnet.String(), "-o", m.cfg.EgressInterface, "-j", "MASQUERADE"},
		} {
			if err := runHostCommand(ctx, args...); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	for _, args := range [][]string{
		{"ip", "link", "del", network.HostDevice},
	} {
		if err := runHostCommand(ctx, args...); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func runHostCommand(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func subnetAt(pool netip.Prefix, index uint32) (netip.Prefix, netip.Addr, netip.Addr, error) {
	base := pool.Masked().Addr()
	base4 := base.As4()
	value := binary.BigEndian.Uint32(base4[:]) + index*4
	var subnetBytes [4]byte
	binary.BigEndian.PutUint32(subnetBytes[:], value)
	subnetAddr := netip.AddrFrom4(subnetBytes)
	subnet := netip.PrefixFrom(subnetAddr, 30)
	if !pool.Contains(subnet.Addr()) {
		return netip.Prefix{}, netip.Addr{}, netip.Addr{}, fmt.Errorf("subnet %s escapes pool %s", subnet, pool)
	}
	hostAddr := subnetAddr.Next()
	guestAddr := hostAddr.Next()
	if !pool.Contains(guestAddr) {
		return netip.Prefix{}, netip.Addr{}, netip.Addr{}, fmt.Errorf("guest address escapes pool %s", pool)
	}
	return subnet, hostAddr, guestAddr, nil
}

func macForIP(addr netip.Addr) string {
	ip := addr.As4()
	return fmt.Sprintf("06:fc:%02x:%02x:%02x:%02x", ip[0], ip[1], ip[2], ip[3])
}
