//go:build windows

package faketcp

// Windows is the only supported physical client backend. Keep the entire raw
// client flow on a coherent Windows-family IPv4/TCP presentation.
const DefaultPacketPersona = PacketPersonaWindows11
