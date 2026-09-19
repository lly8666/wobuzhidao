//go:build !windows

package faketcp

// Linux/OpenWrt keep the mature legacy packet presentation unless a test calls
// the explicit persona marshal helper. This preserves existing server and
// benchmark wire bytes.
const DefaultPacketPersona = PacketPersonaLegacy
