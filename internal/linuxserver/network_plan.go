package linuxserver

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/pathmtu"
)

type FirewallBackend string

const (
	FirewallAuto     FirewallBackend = "auto"
	FirewallNFT      FirewallBackend = "nft"
	FirewallIPTables FirewallBackend = "iptables"
)

const (
	tunNameSize  = 16
	DNSUnchanged = "host-dns-unchanged"
)

var ErrNetworkPlan = errors.New("linuxserver: invalid shared-TUN network plan")

type Command struct {
	Name string
	Args []string
}

type SysctlChange struct {
	Key          string
	Value        string
	RestoreSaved bool
}

type FirewallRule struct {
	Purpose string
	Marker  string

	Direction string
	Source    string
	Dest      string
	Conntrack string
	NAT       string
}

type FirewallPlan struct {
	Backend    FirewallBackend
	NFTForward string
	Rules      []FirewallRule
}

type NetworkPlan struct {
	TUNName     string
	LeasePrefix netip.Prefix
	MTU         int

	Setup    []Command
	Teardown []Command
	Sysctls  []SysctlChange
	Firewall FirewallPlan

	DNSMode string
}

// BuildNetworkPlan describes only state owned by the shared-TUN server core.
// It intentionally does not execute commands. The later runtime executor must
// snapshot RestoreSaved sysctls before Apply and restore them on Cleanup.
func BuildNetworkPlan(tunName string, leasePrefix netip.Prefix, mtu int, backend FirewallBackend, nftForward string) (NetworkPlan, error) {
	if tunName == "" || len(tunName) >= tunNameSize || strings.ContainsAny(tunName, " /\t\r\n") {
		return NetworkPlan{}, fmt.Errorf("%w: tun=%q", ErrNetworkPlan, tunName)
	}
	if !leasePrefix.IsValid() || !leasePrefix.Addr().Is4() || leasePrefix.Bits() > 30 {
		return NetworkPlan{}, fmt.Errorf("%w: lease_prefix=%s", ErrNetworkPlan, leasePrefix)
	}
	if err := pathmtu.ValidateConnectionMTU(mtu); err != nil {
		return NetworkPlan{}, fmt.Errorf("%w: mtu=%d: %v", ErrNetworkPlan, mtu, err)
	}
	switch backend {
	case FirewallAuto, FirewallNFT, FirewallIPTables:
	default:
		return NetworkPlan{}, fmt.Errorf("%w: backend=%q", ErrNetworkPlan, backend)
	}
	if backend != FirewallNFT && strings.TrimSpace(nftForward) != "" {
		return NetworkPlan{}, fmt.Errorf("%w: nft-forward requires nft backend", ErrNetworkPlan)
	}
	if backend == FirewallNFT && strings.TrimSpace(nftForward) != "" {
		parts := strings.Split(nftForward, ":")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return NetworkPlan{}, fmt.Errorf("%w: nft-forward=%q", ErrNetworkPlan, nftForward)
		}
	}

	leasePrefix = leasePrefix.Masked()
	prefix := leasePrefix.String()
	plan := NetworkPlan{
		TUNName:     tunName,
		LeasePrefix: leasePrefix,
		MTU:         mtu,
		Setup: []Command{
			{Name: "ip", Args: []string{"link", "set", tunName, "mtu", strconv.Itoa(mtu), "up"}},
			{Name: "ip", Args: []string{"route", "replace", prefix, "dev", tunName}},
		},
		Teardown: []Command{
			{Name: "ip", Args: []string{"route", "del", prefix, "dev", tunName}},
		},
		Sysctls: []SysctlChange{
			{Key: "net.ipv4.ip_forward", Value: "1", RestoreSaved: true},
			{Key: "net.ipv4.conf." + tunName + ".rp_filter", Value: "0", RestoreSaved: true},
		},
		Firewall: FirewallPlan{
			Backend:    backend,
			NFTForward: strings.TrimSpace(nftForward),
			Rules: []FirewallRule{
				{
					Purpose:   "shared-tun-egress-forward",
					Marker:    "wbd-shared-tun-out",
					Direction: "from-tun",
					Source:    prefix,
				},
				{
					Purpose:   "shared-tun-return-forward",
					Marker:    "wbd-shared-tun-in",
					Direction: "to-tun",
					Dest:      prefix,
					Conntrack: "ESTABLISHED,RELATED",
				},
				{
					Purpose:   "shared-tun-host-nat",
					Marker:    "wbd-shared-tun-nat",
					Direction: "from-tun",
					Source:    prefix,
					NAT:       "MASQUERADE outside " + tunName,
				},
			},
		},
		// The server does not rewrite host resolv.conf/resolved state. Client DNS
		// policy is a separate platform concern; DNS packets here are ordinary
		// inner IPv4 traffic and use the same forwarding/NAT path.
		DNSMode: DNSUnchanged,
	}
	return plan, nil
}
