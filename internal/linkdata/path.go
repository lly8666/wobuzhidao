package linkdata

import (
	"errors"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/fec"
)

var ErrUnsupportedLinkConfig = errors.New("linkdata: unsupported immutable link config")

type PathStats struct {
	InnerTXPackets uint64
	InnerTXBytes   uint64
	WireTXPackets  uint64
	WireTXBytes    uint64

	FECSystematicTXPackets uint64
	FECSystematicTXBytes   uint64
	FECRepairTXPackets     uint64
	FECRepairTXBytes       uint64

	WireRXPackets uint64
	WireRXBytes   uint64
	InnerRXPackets uint64
	InnerRXBytes   uint64
}

// Path is selected once after LINK_ACCEPT/AUTH and never changes for the
// lifetime of an association. It deliberately contains no runtime mode switch
// or config-epoch machinery.
type Path struct {
	config control.LinkConfig
	enc    *fec.FastBlockEncoder
	dec    *fec.BlockDecoder
	stats  PathStats
}

func New(config control.LinkConfig, maxBlocks int) (*Path, error) {
	if err := control.CurrentLinkPolicy().Validate(config); err != nil {
		return nil, err
	}
	p := &Path{config: config}
	if config.FECMode == control.FECOff {
		return p, nil
	}
	if config.FECMode != control.FECFixed ||
		config.Scheduler != control.FECSchedulerTailRS ||
		config.DataShards != fec.DataShards || config.ParityShards != fec.ParityShards {
		return nil, ErrUnsupportedLinkConfig
	}
	codec := fec.NewFastReedSolomon20x20()
	enc, err := fec.NewFastBlockEncoder(codec, int(config.MTU), time.Duration(config.FlushMillis)*time.Millisecond, 1)
	if err != nil {
		return nil, err
	}
	dec, err := fec.NewBlockDecoder(codec, int(config.MTU), maxBlocks)
	if err != nil {
		return nil, err
	}
	p.enc, p.dec = enc, dec
	return p, nil
}

func (p *Path) Config() control.LinkConfig { return p.config }
func (p *Path) FECEnabled() bool { return p.config.FECMode == control.FECFixed }
func (p *Path) Stats() PathStats { return p.stats }

// Encode returns datagrams ready for DTLS. In off mode the input datagram is
// returned directly and remains valid only as long as the caller's input; the
// product proxy sends it synchronously. In fixed mode returned FEC buffers have
// the lifetime documented by FastBlockEncoder.
func (p *Path) Encode(packet []byte, now time.Time) ([][]byte, error) {
	if len(packet) == 0 || len(packet) > int(p.config.MTU) {
		return nil, fec.ErrPacketTooLarge
	}
	var (
		wire [][]byte
		err error
	)
	if !p.FECEnabled() {
		wire = [][]byte{packet}
	} else {
		wire, err = p.enc.Add(packet, now)
		if err != nil {
			return nil, err
		}
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
		if int(h.ShardIndex) < fec.DataShards {
			p.stats.FECSystematicTXPackets++
			p.stats.FECSystematicTXBytes += uint64(len(datagram))
		} else {
			p.stats.FECRepairTXPackets++
			p.stats.FECRepairTXBytes += uint64(len(datagram))
		}
	}
}

// Decode returns first-complete original datagrams. Off mode is a zero-wait
// packet-preserving pass-through. Fixed mode preserves the existing WBD rule:
// surviving systematic sources can be returned immediately while missing ones
// are returned at their earliest FEC reconstruction time.
func (p *Path) Decode(wire []byte) ([][]byte, error) {
	if len(wire) == 0 {
		return nil, fec.ErrInvalidShardSet
	}
	p.stats.WireRXPackets++
	p.stats.WireRXBytes += uint64(len(wire))
	var (
		packets [][]byte
		err error
	)
	if !p.FECEnabled() {
		if len(wire) > int(p.config.MTU) {
			return nil, fec.ErrPacketTooLarge
		}
		packets = [][]byte{wire}
	} else {
		packets, _, err = p.dec.Add(wire)
		if err != nil {
			return nil, err
		}
	}
	for _, packet := range packets {
		p.stats.InnerRXPackets++
		p.stats.InnerRXBytes += uint64(len(packet))
	}
	return packets, nil
}
