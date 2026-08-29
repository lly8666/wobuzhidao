package faketcp

// PeerSYNClass describes a SYN received for an already-indexed server flow.
// The caller is responsible for validating the WBD handshake signature before
// using this classification.
type PeerSYNClass uint8

const (
	PeerSYNInvalid PeerSYNClass = iota
	PeerSYNRetransmit
	PeerSYNNewIncarnation
)

// ClassifyPeerSYN distinguishes a delayed/retransmitted SYN from a new client
// incarnation that happens to reuse the same public four-tuple. NATs can retain
// or rapidly reuse TCP mappings after a prior WBD process exits without a kernel
// TCP FIN, so tuple identity alone is not a session-incarnation identity.
func (a *ServerAssociation) ClassifyPeerSYN(seg Segment) PeerSYNClass {
	if a == nil {
		return PeerSYNInvalid
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == ServerAssociationClosed || !a.flow.Matches(seg) || seg.Flags&FlagSYN == 0 || seg.Flags&FlagACK != 0 || len(seg.Payload) != 0 {
		return PeerSYNInvalid
	}
	if seg.Seq+1 == a.peerNext {
		return PeerSYNRetransmit
	}
	return PeerSYNNewIncarnation
}
