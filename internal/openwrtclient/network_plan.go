package openwrtclient

import (
	"errors"
	"fmt"
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

	PolicyRule Command
	LocalRoute Command
}

// BuildNetworkPlan describes only WBD-owned IPv4 TPROXY kernel state. It does
// not create a proxy process or any localhost relay. IPv6 is intentionally not
// captured by this P4 atom because the active Logical Tunnel lease/data plane is
// IPv4-only.
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
			{Purpose: "tcp-tproxy-capture", Marker: "wbd-tproxy-tcp", Match: "meta nfproto ipv4 meta l4proto tcp", Action: "tproxy to :" + portText + " meta mark set " + markText + " accept"},
			{Purpose: "udp-tproxy-capture", Marker: "wbd-tproxy-udp", Match: "meta nfproto ipv4 meta l4proto udp", Action: "tproxy to :" + portText + " meta mark set " + markText + " accept"},
		},
		PolicyRule: Command{Name: "ip", Args: []string{"-4", "rule", "add", "priority", priorityText, "fwmark", markText, "lookup", tableText}},
		LocalRoute: Command{Name: "ip", Args: []string{"-4", "route", "add", "local", "0.0.0.0/0", "dev", "lo", "table", tableText}},
	}, nil
}

func (p NetworkPlan) NFTScript() (string, error) {
	canonical, err := BuildNetworkPlan(p.ListenPort, p.Mark, p.Table, p.Priority, p.Underlay4)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "table %s %s {\n", canonical.NFTFamily, canonical.NFTTable)
	b.WriteString("  chain prerouting {\n")
	b.WriteString("    type filter hook prerouting priority mangle; policy accept;\n")
	for _, rule := range canonical.Rules {
		fmt.Fprintf(&b, "    %s %s comment %q\n", rule.Match, rule.Action, rule.Marker)
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String(), nil
}
