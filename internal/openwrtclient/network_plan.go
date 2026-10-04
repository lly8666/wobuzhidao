package openwrtclient

import (
	"errors"
	"fmt"
	"github.com/lly8666/wobuzhidao/internal/splitroute"
	"net/netip"
	"strconv"
	"strings"
)

const OwnedNFTTable = "wbd_tproxy"

var ErrNetworkPlan = errors.New("openwrtclient: invalid TPROXY network plan")

type Command struct {
	Name string
	Args []string
}

type CaptureRule struct {
	Purpose string
	Marker  string
	Match   string
	Action  string
}

type NetworkPlan struct {
	ListenPort uint16
	Mark       uint32
	Table      uint32
	Priority   uint32
	Underlay4  netip.Addr

	NFTFamily string
	NFTTable  string
	Rules     []CaptureRule
	Direct4   []netip.Prefix
	DNSHijack bool

	PolicyRule    Command
	LocalRoute    Command
	IPv6Rule      Command
	IPv6Blackhole Command
}

// BuildNetworkPlan describes owned IPv4 interception and IPv6 blackhole state.
// IPv6 is never admitted into the IPv4-only logical data plane.
func BuildNetworkPlan(listenPort uint16, mark, table, priority uint32, underlay4 netip.Addr) (NetworkPlan, error) {
	if listenPort == 0 || mark == 0 || table == 0 || priority == 0 {
		return NetworkPlan{}, ErrNetworkPlan
	}
	if mark > 0x7fffffff || table > 0x7fffffff || priority > 0x7fffffff {
		return NetworkPlan{}, ErrNetworkPlan
	}
	if !underlay4.IsValid() || !underlay4.Is4() {
		return NetworkPlan{}, fmt.Errorf("%w: underlay4=%s", ErrNetworkPlan, underlay4)
	}
	underlay4 = underlay4.Unmap()
	if underlay4.IsUnspecified() || underlay4.IsMulticast() {
		return NetworkPlan{}, fmt.Errorf("%w: underlay4=%s", ErrNetworkPlan, underlay4)
	}

	markText := fmt.Sprintf("0x%x", mark)
	portText := strconv.Itoa(int(listenPort))
	tableText := strconv.FormatUint(uint64(table), 10)
	priorityText := strconv.FormatUint(uint64(priority), 10)
	underlayText := underlay4.String()

	return NetworkPlan{
		ListenPort: listenPort,
		Mark:       mark,
		Table:      table,
		Priority:   priority,
		Underlay4:  underlay4,
		NFTFamily:  "inet",
		NFTTable:   OwnedNFTTable,
		Rules: []CaptureRule{
			{Purpose: "already-marked-bypass", Marker: "wbd-tproxy-mark-bypass", Match: "meta mark " + markText, Action: "return"},
			{Purpose: "underlay-source-bypass", Marker: "wbd-tproxy-underlay-src", Match: "ip saddr " + underlayText, Action: "return"},
			{Purpose: "local-destination-bypass", Marker: "wbd-tproxy-local-dst", Match: "fib daddr type local", Action: "return"},
			{Purpose: "underlay-destination-bypass", Marker: "wbd-tproxy-underlay-dst", Match: "ip daddr " + underlayText, Action: "return"},
			{Purpose: "tcp-tproxy-capture", Marker: "wbd-tproxy-tcp", Match: "meta nfproto ipv4 meta l4proto tcp", Action: "tproxy ip to :" + portText + " meta mark set " + markText + " accept"},
			{Purpose: "udp-tproxy-capture", Marker: "wbd-tproxy-udp", Match: "meta nfproto ipv4 meta l4proto udp", Action: "tproxy ip to :" + portText + " meta mark set " + markText + " accept"},
		},
		PolicyRule:    Command{Name: "ip", Args: []string{"-4", "rule", "add", "priority", priorityText, "fwmark", markText, "lookup", tableText}},
		LocalRoute:    Command{Name: "ip", Args: []string{"-4", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", tableText}},
		IPv6Rule:      Command{Name: "ip", Args: []string{"-6", "rule", "add", "priority", priorityText, "lookup", tableText}},
		IPv6Blackhole: Command{Name: "ip", Args: []string{"-6", "route", "add", "blackhole", "::/0", "table", tableText}},
	}, nil
}

func (p NetworkPlan) NFTScript() (string, error) {
	canonical, err := BuildNetworkPlan(p.ListenPort, p.Mark, p.Table, p.Priority, p.Underlay4)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "table %s %s {\n", canonical.NFTFamily, canonical.NFTTable)
	direct, err := splitroute.Normalize(p.Direct4)
	if err != nil {
		return "", err
	}
	if len(direct) > 0 {
		b.WriteString("  set direct4 { type ipv4_addr; flags interval; elements = { ")
		for i, prefix := range direct {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(prefix.String())
		}
		b.WriteString(" }; }\n")
	}
	b.WriteString("  chain prerouting {\n")
	b.WriteString("    type filter hook prerouting priority mangle; policy accept;\n")
	b.WriteString("    meta nfproto ipv6 counter drop comment \"wbd-ipv6-drop\"\n")
	fmt.Fprintf(&b, "    meta mark 0x%x return comment \"wbd-reply-bypass\"\n", p.Mark|0x80000000)
	if p.DNSHijack {
		fmt.Fprintf(&b, "    ip daddr %s return comment \"wbd-dns-underlay\"\n", canonical.Underlay4)
		fmt.Fprintf(&b, "    meta nfproto ipv4 meta l4proto { tcp, udp } th dport 53 tproxy ip to :%d meta mark set 0x%x counter accept comment \"wbd-dns-capture\"\n", p.ListenPort, p.Mark)
	}
	for _, rule := range canonical.Rules {
		if rule.Purpose == "tcp-tproxy-capture" && len(direct) > 0 {
			b.WriteString("    ip daddr @direct4 counter return comment \"wbd-direct4\"\n")
		}
		fmt.Fprintf(&b, "    %s %s comment %q\n", rule.Match, rule.Action, rule.Marker)
	}
	b.WriteString("  }\n")
	b.WriteString("  chain output {\n    type route hook output priority mangle; policy accept;\n    meta nfproto ipv6 counter drop comment \"wbd-ipv6-output-drop\"\n")
	fmt.Fprintf(&b, "    meta mark 0x%x return comment \"wbd-reply-output-bypass\"\n", p.Mark|0x80000000)
	if p.DNSHijack {
		fmt.Fprintf(&b, "    ip daddr %s return comment \"wbd-dns-output-underlay\"\n", canonical.Underlay4)
		fmt.Fprintf(&b, "    meta nfproto ipv4 meta l4proto { tcp, udp } th dport 53 meta mark set 0x%x comment \"wbd-dns-output-route\"\n", p.Mark)
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String(), nil
}
