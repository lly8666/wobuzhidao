package faketcp

// IsKeepaliveProbe recognizes the classic TCP keepalive shape without adding
// a new wire marker or consuming sequence space. A live peer sends an ACK-only
// segment at RCV.NXT-1; the receiver answers with its current cumulative ACK.
//
// FIN/RST/SYN/PSH are excluded deliberately: lifecycle/control packets must
// keep their own semantics and must never be mistaken for liveness probes.
func IsKeepaliveProbe(seg Segment, receiverNext uint32) bool {
	if len(seg.Payload) != 0 || seg.Flags&FlagACK == 0 {
		return false
	}
	if seg.Flags&(FlagFIN|FlagSYN|FlagRST|FlagPSH) != 0 {
		return false
	}
	return seg.Seq+1 == receiverNext
}
