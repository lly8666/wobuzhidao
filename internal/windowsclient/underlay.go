package windowsclient

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

var (
	ErrPhysicalUnderlay            = errors.New("windowsclient: invalid physical underlay")
	ErrPhysicalUnderlayUnsupported = errors.New("windowsclient: physical underlay discovery unsupported")
)

type PhysicalUnderlay struct {
	Peer4          netip.Addr
	Source4        netip.Addr
	InterfaceIndex uint32
	PacketDevice   string
	SourceMAC      [6]byte
	NextHop4       netip.Addr
	NextHopMAC     [6]byte
}

func (u PhysicalUnderlay) Validate() error {
	if !u.Peer4.IsValid() || !u.Peer4.Is4() ||
		!u.Source4.IsValid() || !u.Source4.Is4() ||
		!u.NextHop4.IsValid() || !u.NextHop4.Is4() ||
		u.InterfaceIndex == 0 ||
		u.SourceMAC == ([6]byte{}) ||
		u.NextHopMAC == ([6]byte{}) {
		return ErrPhysicalUnderlay
	}
	device := strings.TrimSpace(u.PacketDevice)
	if !strings.HasPrefix(strings.ToUpper(device), "\\DEVICE\\NPF_{") ||
		!strings.HasSuffix(device, "}") {
		return ErrPhysicalUnderlay
	}
	return nil
}

func (u PhysicalUnderlay) PhysicalPath() (PhysicalPath, error) {
	if err := u.Validate(); err != nil {
		return PhysicalPath{}, err
	}
	return PhysicalPath{
		InterfaceIndex: u.InterfaceIndex,
		NextHop4:       u.NextHop4.Unmap(),
	}, nil
}

func (u PhysicalUnderlay) NpcapConfig(sourcePort, peerPort uint16, generation uint64, persona faketcp.PacketPersona) (faketcp.NpcapConfig, error) {
	if err := u.Validate(); err != nil {
		return faketcp.NpcapConfig{}, err
	}
	cfg := faketcp.NpcapConfig{
		Device:     u.PacketDevice,
		SourceMAC:  u.SourceMAC,
		NextHopMAC: u.NextHopMAC,
		Flow: faketcp.NpcapFlow{
			LocalIP:   u.Source4.Unmap().As4(),
			PeerIP:    u.Peer4.Unmap().As4(),
			LocalPort: sourcePort,
			PeerPort:  peerPort,
		},
		Persona:    persona,
		Generation: generation,
	}
	if err := cfg.Validate(); err != nil {
		return faketcp.NpcapConfig{}, err
	}
	return cfg, nil
}

type physicalRouteCandidate struct {
	prefix         netip.Prefix
	nextHop        netip.Addr
	interfaceIndex uint32
	metric         uint64
	source4        netip.Addr
	adapterName    string
	sourceMAC      [6]byte
}

func selectPhysicalRoute(in []physicalRouteCandidate) (physicalRouteCandidate, error) {
	if len(in) == 0 {
		return physicalRouteCandidate{}, ErrPhysicalUnderlay
	}
	candidates := append([]physicalRouteCandidate(nil), in...)
	sort.Slice(candidates, func(i, j int) bool {
		ib, jb := candidates[i].prefix.Bits(), candidates[j].prefix.Bits()
		if ib != jb {
			return ib > jb
		}
		if candidates[i].metric != candidates[j].metric {
			return candidates[i].metric < candidates[j].metric
		}
		return candidates[i].interfaceIndex < candidates[j].interfaceIndex
	})
	return candidates[0], nil
}

func npcapDeviceForAdapter(name string) (string, error) {
	guid := strings.TrimSpace(name)
	upper := strings.ToUpper(guid)
	const tcpipPrefix = "\\DEVICE\\TCPIP_"
	if strings.HasPrefix(upper, tcpipPrefix) {
		guid = guid[len(tcpipPrefix):]
	}
	if len(guid) == 36 && !strings.HasPrefix(guid, "{") {
		guid = "{" + guid + "}"
	}
	if len(guid) != 38 || !strings.HasPrefix(guid, "{") || !strings.HasSuffix(guid, "}") {
		return "", fmt.Errorf(
			"%w: Windows adapter name is not a GUID: %q",
			ErrPhysicalUnderlay,
			name,
		)
	}
	return "\\Device\\NPF_" + strings.ToUpper(guid), nil
}
