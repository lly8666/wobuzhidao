package linkdata

import (
	"errors"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/fec"
)

var ErrUnsupportedLinkConfig = errors.New("linkdata: unsupported immutable link config")

// Path is selected once after LINK_ACCEPT/AUTH and never changes for the
// lifetime of an association. It deliberately contains no runtime mode switch
// or config-epoch machinery.
type Path struct {
	config control.LinkConfig
	enc    *fec.FastBlockEncoder
	dec    *fec.BlockDecoder
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

// Encode returns datagrams ready for DTLS. In off mode the input datagram is
// returned directly and remains valid only as long as the caller's input; the
// product proxy sends it synchronously. In fixed mode returned FEC buffers have
// the lifetime documented by FastBlockEncoder.
func (p *Path) Encode(packet []byte, now time.Time) ([][]byte, error) {
	if len(packet) == 0 || len(packet) > int(p.config.MTU) {
		return nil, fec.ErrPacketTooLarge
	}
	if !p.FECEnabled() {
		return [][]byte{packet}, nil
	}
	return p.enc.Add(packet, now)
}

func (p *Path) FlushDue(now time.Time) ([][]byte, error) {
	if !p.FECEnabled() {
		return nil, nil
	}
	return p.enc.FlushDue(now)
}

func (p *Path) Flush() ([][]byte, error) {
	if !p.FECEnabled() {
		return nil, nil
	}
	return p.enc.Flush()
}

// Decode returns first-complete original datagrams. Off mode is a zero-wait
// packet-preserving pass-through. Fixed mode preserves the existing WBD rule:
// surviving systematic sources can be returned immediately while missing ones
// are returned at their earliest FEC reconstruction time.
func (p *Path) Decode(wire []byte) ([][]byte, error) {
	if len(wire) == 0 {
		return nil, fec.ErrInvalidShardSet
	}
	if !p.FECEnabled() {
		if len(wire) > int(p.config.MTU) {
			return nil, fec.ErrPacketTooLarge
		}
		return [][]byte{wire}, nil
	}
	packets, _, err := p.dec.Add(wire)
	return packets, err
}
