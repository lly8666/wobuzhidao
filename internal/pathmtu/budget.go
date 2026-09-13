package pathmtu

import (
	"errors"
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/gamepath"
)

const (
	// DefaultConnectionMTU is only a default for profiles that omit MTU. It is
	// never an additional runtime cap.
	DefaultConnectionMTU = 1500
	// MinConnectionMTU and MaxConnectionMTU define the operator-visible product
	// range. Every inner limit is derived from this one outer ceiling.
	MinConnectionMTU = 576
	MaxConnectionMTU = 9000

	// DTLS13RecordReserve is the product-side ciphertext expansion reserve for
	// one DTLS 1.3 application record on the pinned wolfSSL path. It covers the
	// protected record header, TLSInnerPlaintext content type and AEAD tag, with
	// margin for the current no-CID record form. It is deliberately a reserve,
	// not an estimate of average overhead: the resulting datagram must fit in a
	// single FakeTCP carrier payload without invoking CarrierFragmenter.
	DTLS13RecordReserve = 32
)

var ErrConnectionMTU = errors.New("pathmtu: connection MTU cannot satisfy enabled feature budget")

type Features struct {
	FEC  bool
	Game bool
}

type Budget struct {
	ConnectionMTU     int
	CarrierPayloadMTU int
	DTLSPlaintextMTU  int
	LinkPlaintextMTU  int
	InnerMTU          int
	FECOverhead       int
	GameOverhead      int
	DTLSRecordReserve int
}

// ValidateConnectionMTU validates the sole operator-visible MTU. Lower layers
// must not impose their own historical 1360/1400/1500 ceilings; they derive a
// budget from this value and may only reject immutable protocol impossibilities.
func ValidateConnectionMTU(connectionMTU int) error {
	if connectionMTU < MinConnectionMTU || connectionMTU > MaxConnectionMTU {
		return fmt.Errorf("%w: connection=%d outside product range %d..%d", ErrConnectionMTU, connectionMTU, MinConnectionMTU, MaxConnectionMTU)
	}
	return nil
}

// Derive turns the operator-visible connection MTU ceiling into the nested
// product limits used by each layer. The configured value is never passed
// through as a TUN/LINK MTU. Instead every enabled wrapper consumes budget
// before the next inner layer is sized.
func Derive(connectionMTU int, features Features) (Budget, error) {
	if err := ValidateConnectionMTU(connectionMTU); err != nil {
		return Budget{}, err
	}
	carrier, err := faketcp.CarrierPayloadBudget(connectionMTU)
	if err != nil {
		return Budget{}, fmt.Errorf("%w: connection=%d: %v", ErrConnectionMTU, connectionMTU, err)
	}
	b := Budget{
		ConnectionMTU:     connectionMTU,
		CarrierPayloadMTU: carrier,
		DTLSRecordReserve: DTLS13RecordReserve,
	}
	b.DTLSPlaintextMTU = carrier - DTLS13RecordReserve
	if b.DTLSPlaintextMTU <= 0 {
		return Budget{}, fmt.Errorf("%w: connection=%d leaves no DTLS plaintext", ErrConnectionMTU, connectionMTU)
	}

	link := b.DTLSPlaintextMTU
	if features.FEC {
		b.FECOverhead = fec.HeaderSize
		link -= b.FECOverhead
	}
	if link <= 0 {
		return Budget{}, fmt.Errorf("%w: connection=%d leaves no LINK plaintext", ErrConnectionMTU, connectionMTU)
	}

	// CurrentLinkPolicy's MTU range is now a protocol representation/sanity
	// boundary only. It must never shrink a valid product connection MTU. Fail
	// explicitly if a future wire format cannot represent the derived value.
	policy := control.CurrentLinkPolicy()
	if link < int(policy.MinMTU) || link > int(policy.MaxMTU) {
		return Budget{}, fmt.Errorf("%w: derived LINK MTU=%d outside protocol range %d..%d", ErrConnectionMTU, link, policy.MinMTU, policy.MaxMTU)
	}
	b.LinkPlaintextMTU = link

	inner := link
	if features.Game {
		b.GameOverhead = gamepath.DatagramOverhead()
		inner -= b.GameOverhead
		if inner < gamepath.MinimumInnerMTU {
			return Budget{}, fmt.Errorf("%w: derived Game inner MTU=%d below product minimum=%d", ErrConnectionMTU, inner, gamepath.MinimumInnerMTU)
		}
	}
	b.InnerMTU = inner
	return b, nil
}

func (b Budget) MaxDTLSDatagram() int {
	return b.DTLSPlaintextMTU + b.DTLSRecordReserve
}

func (b Budget) MaxFECWireDatagram() int {
	return b.LinkPlaintextMTU + b.FECOverhead
}

func (b Budget) MaxGameDatagram() int {
	return b.InnerMTU + b.GameOverhead
}
