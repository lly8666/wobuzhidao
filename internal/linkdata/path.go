package linkdata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/fec"
)

var ErrUnsupportedLinkConfig = errors.New("linkdata: unsupported immutable link config")

func supportedFixedParityShards(n uint8) bool {
	switch n {
	case 4, 8, 10, 12, 16, 20:
		return true
	default:
		return false
	}
}

type PathStats struct {
	InnerTXPackets uint64
	InnerTXBytes   uint64
	WireTXPackets  uint64
	WireTXBytes    uint64

	FECSystematicTXPackets uint64
	FECSystematicTXBytes   uint64
	FECRepairTXPackets     uint64
	FECRepairTXBytes       uint64

	WireRXPackets  uint64
	WireRXBytes    uint64
	InnerRXPackets uint64
	InnerRXBytes   uint64
}

// Path is selected once after LINK_ACCEPT/AUTH and never changes for the
// lifetime of an association. It deliberately contains no runtime mode switch
// or config-epoch machinery.
type Path struct {
	config      control.LinkConfig
	enc         *fec.FastBlockEncoder
	dec         *fec.BlockDecoder
	recovery    *fecRecoveryTracker
	fragmenter  *datagramFragmenter
	reassembler *datagramReassembler
	stats       PathStats
}

func New(config control.LinkConfig, maxBlocks int) (*Path, error) {
	if err := control.CurrentLinkPolicy().Validate(config); err != nil {
		return nil, err
	}
	fragmenter, err := newDatagramFragmenter(int(config.MTU))
	if err != nil {
		return nil, err
	}
	p := &Path{config: config, fragmenter: fragmenter, reassembler: newDatagramReassembler()}
	if config.FECMode == control.FECOff {
		return p, nil
	}
	if config.FECMode != control.FECFixed ||
		config.Scheduler != control.FECSchedulerTailRS ||
		config.DataShards != fec.DataShards ||
		!supportedFixedParityShards(config.ParityShards) {
		return nil, ErrUnsupportedLinkConfig
	}
	codec := fec.NewFastReedSolomon20x20()
	enc, err := fec.NewFastBlockEncoderWithParity(codec, int(config.MTU), time.Duration(config.FlushMillis)*time.Millisecond, 1, int(config.ParityShards))
	if err != nil {
		return nil, err
	}
	dec, err := fec.NewBlockDecoderWithParity(installFECObserver(p, codec), int(config.MTU), maxBlocks, int(config.ParityShards))
	if err != nil {
		return nil, err
	}
	p.enc, p.dec = enc, dec
	p.recovery = newFECRecoveryTracker(candidateFECRecoveryHorizon)
	return p, nil
}

func (p *Path) Config() control.LinkConfig { return p.config }
func (p *Path) FECEnabled() bool           { return p.config.FECMode == control.FECFixed }
func (p *Path) Stats() PathStats           { return p.stats }

// Encode returns datagrams ready for DTLS. A legal application datagram keeps
// the historical zero-copy/zero-wait first-arrival path. An oversized logical
// datagram is first split into LINK-sized fragment frames; FEC, when enabled,
// protects those frames independently. This makes LINK MTU a wire-datagram
// ceiling rather than an application-datagram drop threshold.
func (p *Path) Encode(packet []byte, now time.Time) ([][]byte, error) {
	fragments, err := p.fragmenter.Fragment(packet)
	if err != nil {
		return nil, fmt.Errorf("linkdata encode: input_bytes=%d configured_mtu=%d: %w", len(packet), p.config.MTU, err)
	}
	wire := make([][]byte, 0, len(fragments))
	for _, fragment := range fragments {
		if !p.FECEnabled() {
			wire = append(wire, fragment)
			continue
		}
		encoded, err := p.enc.Add(fragment, now)
		if err != nil {
			return nil, fmt.Errorf("linkdata encode fragment_bytes=%d configured_mtu=%d: %w", len(fragment), p.config.MTU, err)
		}
		wire = append(wire, encoded...)
	}
	p.stats.InnerTXPackets++
	p.stats.InnerTXBytes += uint64(len(packet))
	p.recordWireTX(wire)
	return wire, nil
}

func (p *Path) FlushDue(now time.Time) ([][]byte, error) {
	if !p.FECEnabled() {
		return nil, nil
	}
	p.expireFECRecovery(now)
	observeFECDecoder(p, now)
	wire, err := p.enc.FlushDue(now)
	if err != nil {
		return nil, err
	}
	p.recordWireTX(wire)
	return wire, nil
}

func (p *Path) Flush() ([][]byte, error) {
	if !p.FECEnabled() {
		return nil, nil
	}
	wire, err := p.enc.Flush()
	if err != nil {
		return nil, err
	}
	p.recordWireTX(wire)
	return wire, nil
}

func (p *Path) recordWireTX(wire [][]byte) {
	for _, datagram := range wire {
		p.stats.WireTXPackets++
		p.stats.WireTXBytes += uint64(len(datagram))
		if !p.FECEnabled() || len(datagram) < fec.HeaderSize {
			continue
		}
		h, err := fec.ParseBlockHeader(datagram[:fec.HeaderSize])
		if err != nil {
			continue
		}
		observeFECWire(p, h)
		if int(h.ShardIndex) < fec.DataShards {
			p.stats.FECSystematicTXPackets++
			p.stats.FECSystematicTXBytes += uint64(len(datagram))
		} else {
			p.stats.FECRepairTXPackets++
			p.stats.FECRepairTXBytes += uint64(len(datagram))
		}
	}
}

// Decode returns first-complete original application datagrams. FEC recovery
// happens before logical fragment reassembly, so one lost fragment can still be
// reconstructed from parity. Ordinary non-fragmented traffic remains a direct
// pass-through.
func (p *Path) Decode(wire []byte) ([][]byte, error) {
	return p.decodeAt(wire, time.Now())
}

func (p *Path) decodeAt(wire []byte, now time.Time) ([][]byte, error) {
	if len(wire) == 0 {
		return nil, fec.ErrInvalidShardSet
	}
	p.stats.WireRXPackets++
	p.stats.WireRXBytes += uint64(len(wire))
	var (
		candidates [][]byte
		err        error
	)
	if !p.FECEnabled() {
		if len(wire) > int(p.config.MTU) {
			return nil, fmt.Errorf("linkdata decode off: wire_bytes=%d configured_mtu=%d: %w", len(wire), p.config.MTU, fec.ErrPacketTooLarge)
		}
		candidates = [][]byte{wire}
	} else {
		p.expireFECRecovery(now)
		candidates, _, err = p.dec.Add(wire)
		if err == nil && len(wire) >= 8 && p.recovery != nil {
			blockID := binary.BigEndian.Uint32(wire[4:8])
			if p.dec.IsHeavyBlock(blockID) {
				p.recovery.observeHeavy(blockID, now)
			} else {
				p.recovery.forget(blockID)
			}
		}
		observeFECDecoder(p, now)
		if err != nil {
			return nil, fmt.Errorf("linkdata decode: wire_bytes=%d configured_mtu=%d: %w", len(wire), p.config.MTU, err)
		}
	}

	packets := make([][]byte, 0, len(candidates))
	for _, candidate := range candidates {
		packet, complete, err := p.reassembler.Push(candidate, now)
		if err != nil {
			return nil, fmt.Errorf("linkdata reassemble: wire_bytes=%d configured_mtu=%d: %w", len(wire), p.config.MTU, err)
		}
		if !complete {
			continue
		}
		packets = append(packets, packet)
		p.stats.InnerRXPackets++
		p.stats.InnerRXBytes += uint64(len(packet))
	}
	return packets, nil
}
