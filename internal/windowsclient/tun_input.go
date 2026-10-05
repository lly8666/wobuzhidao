package windowsclient

import (
	"errors"
	"sync/atomic"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// TUNInput keeps rejected local input separate from fatal driver/runtime errors.
// It does not own packets or send anything, so a rejection cannot partially emit
// a logical packet or bypass the authenticated lease fence.
type TUNInput struct {
	router   *Router
	oversize atomic.Uint64
	invalid  atomic.Uint64
	spoof    atomic.Uint64
}

type TUNInputDiagnostic struct {
	MaxIPv4PacketBytes  int    `json:"max_ipv4_packet_bytes"`
	RejectedOversize    uint64 `json:"rejected_oversize"`
	RejectedInvalidIPv4 uint64 `json:"rejected_invalid_ipv4"`
	RejectedSource      uint64 `json:"rejected_source"`
}

func NewTUNInput(router *Router) *TUNInput { return &TUNInput{router: router} }

// Accept returns false/nil only for rejected input. Binding errors remain fatal;
// callers must also preserve every error from actual runtime/wire emission.
func (in *TUNInput) Accept(packet []byte) (bool, error) {
	if in == nil || in.router == nil {
		return false, ErrInvalidBinding
	}
	err := in.router.ValidateFromTUN(packet)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, logicaltunnel.ErrInvalidIPv4Packet) {
		if len(packet) > logicaltunnel.MaxLeasedIPv4PacketLen {
			in.oversize.Add(1)
		} else {
			in.invalid.Add(1)
		}
		return false, nil
	}
	if errors.Is(err, logicaltunnel.ErrSourceSpoof) {
		in.spoof.Add(1)
		return false, nil
	}
	return false, err
}

func (in *TUNInput) DiagnosticSnapshot() TUNInputDiagnostic {
	if in == nil {
		return TUNInputDiagnostic{}
	}
	return TUNInputDiagnostic{MaxIPv4PacketBytes: logicaltunnel.MaxLeasedIPv4PacketLen,
		RejectedOversize: in.oversize.Load(), RejectedInvalidIPv4: in.invalid.Load(), RejectedSource: in.spoof.Load()}
}
