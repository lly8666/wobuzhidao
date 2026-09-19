package faketcp

import (
	"fmt"
	"strings"
)

// PacketPersona controls only observable IPv4/TCP presentation. It does not
// alter sequence ownership, bootstrap reliability, or transition routing.
type PacketPersona uint8

const (
	PacketPersonaLegacy PacketPersona = iota
	PacketPersonaWindows11
)

func ParsePacketPersona(s string) (PacketPersona, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "legacy", "linux":
		return PacketPersonaLegacy, nil
	case "windows", "windows11", "win11":
		return PacketPersonaWindows11, nil
	default:
		return PacketPersonaLegacy, fmt.Errorf("unknown TCP packet persona %q", s)
	}
}

func (p PacketPersona) String() string {
	switch p {
	case PacketPersonaWindows11:
		return "windows11"
	default:
		return "legacy"
	}
}
