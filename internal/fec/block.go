package fec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

var (
	ErrPacketTooLarge = errors.New("fec: packet too large")
	ErrDecoderFull    = errors.New("fec: decoder block window full")
	ErrHeaderMismatch = errors.New("fec: inconsistent block header")
)

// Header bytes 14:16 were reserved from the first WBD FEC wire format. Bit 0
// now marks a systematic source shard whose payload is complete and may be
// delivered immediately, before the block's final size/count metadata exists.
// Final parity shards keep flags=0 and carry the authoritative block metadata.
const headerFlagStreamingSystematic uint16 = 1

// BlockEncoder is the simple reference encoder. It retains the original
// all-at-flush behavior; FastBlockEncoder is the performance data path and
// streams systematic shards immediately.
type BlockEncoder struct {
	codec         Codec
	maxPacketSize int
	flushAfter    time.Duration
	nextBlockID   uint32
	packets       [][]byte
	firstAt       time.Time
}

func NewBlockEncoder(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32) (*BlockEncoder, error) {
	if codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || flushAfter <= 0 {
		return nil, errors.New("fec: invalid block encoder config")
	}
	return &BlockEncoder{codec: codec, maxPacketSize: maxPacketSize, flushAfter: flushAfter, nextBlockID: firstBlockID}, nil
}

func (e *BlockEncoder) Add(packet []byte, now time.Time) ([][]byte, error) {
	if len(packet) == 0 || len(packet) > e.maxPacketSize {
		return nil, ErrPacketTooLarge
	}
	if len(e.packets) == 0 {
		e.firstAt = now
	}
	e.packets = append(e.packets, append([]byte(nil), packet...))
	if len(e.packets) == DataShards {
		return e.flush()
	}
	return nil, nil
}

func (e *BlockEncoder) FlushDue(now time.Time) ([][]byte, error) {
	if len(e.packets) == 0 || now.Sub(e.firstAt) < e.flushAfter {
		return nil, nil
	}
	return e.flush()
}

func (e *BlockEncoder) Flush() ([][]byte, error) {
	if len(e.packets) == 0 {
		return nil, nil
	}
	return e.flush()
}

func (e *BlockEncoder) Pending() int { return len(e.packets) }

func (e *BlockEncoder) flush() ([][]byte, error) {
	dataCount := len(e.packets)
	shardSize := 0
	for _, p := range e.packets {
		if len(p) > shardSize {
			shardSize = len(p)
		}
	}
	shards := make([][]byte, TotalShards)
	for i := range shards {
		shards[i] = make([]byte, shardSize)
	}
	var lengths [DataShards]uint16
	for i, p := range e.packets {
		copy(shards[i], p)
		lengths[i] = uint16(len(p))
	}
	if err := e.codec.Encode(shards); err != nil {
		return nil, err
	}

	// A partial block does not transmit the known-zero unused systematic shards.
	wire := make([][]byte, 0, dataCount+ParityShards)
	appendShard := func(index int) error {
		h := BlockHeader{
			BlockID:          e.nextBlockID,
			ShardIndex:       uint8(index),
			DataCount:        uint8(dataCount),
			ShardSize:        uint16(shardSize),
			OriginalLengths: lengths,
		}
		hb, err := h.MarshalBinary()
		if err != nil {
			return err
		}
		b := make([]byte, HeaderSize+shardSize)
		copy(b, hb)
		copy(b[HeaderSize:], shards[index])
		wire = append(wire, b)
		return nil
	}
	for i := 0; i < dataCount; i++ {
		if err := appendShard(i); err != nil {
			return nil, err
		}
	}
	for i := DataShards; i < TotalShards; i++ {
		if err := appendShard(i); err != nil {
			return nil, err
		}
	}

	e.nextBlockID++
	e.packets = e.packets[:0]
	e.firstAt = time.Time{}
	return wire, nil
}

type decodeBlock struct {
	header BlockHeader
	final  bool

	shards  [][]byte
	present []bool
	count   int

	// Streaming sources are retained only as retransformation inputs. Their
	// payload has already been delivered to the inner path on first arrival.
	sources       [DataShards][]byte
	sourcePresent [DataShards]bool
	sourceCount   int
	delivered     [DataShards]bool
}

// BlockDecoder keeps a bounded number of in-flight blocks and a bounded recent
// completion set so late parity/source shards do not recreate completed blocks.
// Streaming systematic shards are returned immediately; parity later supplies
// final metadata and reconstructs only sources that never arrived.
type BlockDecoder struct {
	codec         Codec
	maxPacketSize int
	maxBlocks     int
	blocks        map[uint32]*decodeBlock
	completed     map[uint32]struct{}
	completedQ    []uint32
	maxCompleted  int
}

func NewBlockDecoder(codec Codec, maxPacketSize, maxBlocks int) (*BlockDecoder, error) {
	if codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || maxBlocks <= 0 {
		return nil, errors.New("fec: invalid block decoder config")
	}
	return &BlockDecoder{
		codec: codec, maxPacketSize: maxPacketSize, maxBlocks: maxBlocks,
		blocks: make(map[uint32]*decodeBlock), completed: make(map[uint32]struct{}), maxCompleted: maxBlocks * 4,
	}, nil
}

func (d *BlockDecoder) InFlight() int { return len(d.blocks) }

func (d *BlockDecoder) Add(datagram []byte) ([][]byte, bool, error) {
	if len(datagram) < HeaderSize {
		return nil, false, errors.New("fec: shard datagram too short")
	}
	h, err := ParseBlockHeader(datagram[:HeaderSize])
	if err != nil {
		return nil, false, err
	}
	if int(h.ShardSize) > d.maxPacketSize || len(datagram) != HeaderSize+int(h.ShardSize) {
		return nil, false, ErrPacketTooLarge
	}
	flags := binary.BigEndian.Uint16(datagram[14:16])
	if flags & ^headerFlagStreamingSystematic != 0 {
		return nil, false, ErrHeaderMismatch
	}
	streaming := flags&headerFlagStreamingSystematic != 0
	if streaming {
		if err := validateStreamingHeader(h); err != nil {
			return nil, false, err
		}
	}
	if _, ok := d.completed[h.BlockID]; ok {
		return nil, false, nil
	}

	b := d.blocks[h.BlockID]
	if b == nil {
		if len(d.blocks) >= d.maxBlocks {
			return nil, false, ErrDecoderFull
		}
		b = &decodeBlock{}
		d.blocks[h.BlockID] = b
	}
	if streaming {
		return d.addStreamingSource(b, h, datagram[HeaderSize:])
	}
	return d.addFinalShard(b, h, datagram[HeaderSize:])
}

func validateStreamingHeader(h BlockHeader) error {
	idx := int(h.ShardIndex)
	if idx >= DataShards || int(h.DataCount) != DataShards {
		return ErrHeaderMismatch
	}
	if h.OriginalLengths[idx] != h.ShardSize {
		return ErrHeaderMismatch
	}
	for i, n := range h.OriginalLengths {
		if i != idx && n != 0 {
			return ErrHeaderMismatch
		}
	}
	return nil
}

func (d *BlockDecoder) addStreamingSource(b *decodeBlock, h BlockHeader, payload []byte) ([][]byte, bool, error) {
	idx := int(h.ShardIndex)
	if b.final {
		if idx >= int(b.header.DataCount) || int(b.header.OriginalLengths[idx]) != len(payload) {
			return nil, false, ErrHeaderMismatch
		}
		if b.present[idx] {
			return nil, false, nil
		}
		copy(b.shards[idx], payload)
		b.present[idx] = true
		b.count++
	} else {
		if b.sourcePresent[idx] {
			return nil, false, nil
		}
		b.sources[idx] = append([]byte(nil), payload...)
		b.sourcePresent[idx] = true
		b.sourceCount++
	}

	var out [][]byte
	if !b.delivered[idx] {
		out = append(out, append([]byte(nil), payload...))
		b.delivered[idx] = true
	}

	// Twenty streaming systematic shards prove this is a complete full block.
	// All originals have already been delivered, so parity can become a late
	// duplicate and the block need not consume decoder-window memory.
	if !b.final && b.sourceCount == DataShards {
		delete(d.blocks, h.BlockID)
		d.markCompleted(h.BlockID)
		return out, true, nil
	}
	if !b.final {
		return out, false, nil
	}
	recovered, done, err := d.maybeComplete(h.BlockID, b)
	if err != nil {
		return nil, false, err
	}
	out = append(out, recovered...)
	return out, done, nil
}

func (d *BlockDecoder) addFinalShard(b *decodeBlock, h BlockHeader, payload []byte) ([][]byte, bool, error) {
	if !b.final {
		if err := d.finalizeMetadata(b, h); err != nil {
			return nil, false, err
		}
	} else if !sameBlockHeader(b.header, h) {
		return nil, false, ErrHeaderMismatch
	}

	idx := int(h.ShardIndex)
	if !b.present[idx] {
		copy(b.shards[idx], payload)
		b.present[idx] = true
		b.count++
	}
	return d.maybeComplete(h.BlockID, b)
}

func (d *BlockDecoder) finalizeMetadata(b *decodeBlock, h BlockHeader) error {
	b.header = h
	b.final = true
	b.shards = make([][]byte, TotalShards)
	b.present = make([]bool, TotalShards)
	for i := range b.shards {
		b.shards[i] = make([]byte, h.ShardSize)
	}
	for i := int(h.DataCount); i < DataShards; i++ {
		b.present[i] = true
		b.count++
	}
	for i := 0; i < DataShards; i++ {
		if !b.sourcePresent[i] {
			continue
		}
		if i >= int(h.DataCount) || len(b.sources[i]) != int(h.OriginalLengths[i]) {
			return ErrHeaderMismatch
		}
		copy(b.shards[i], b.sources[i])
		b.present[i] = true
		b.count++
		b.sources[i] = nil
	}
	return nil
}

func (d *BlockDecoder) maybeComplete(blockID uint32, b *decodeBlock) ([][]byte, bool, error) {
	if allDataDelivered(b) {
		delete(d.blocks, blockID)
		d.markCompleted(blockID)
		return nil, true, nil
	}
	if b.count < DataShards {
		return nil, false, nil
	}
	if err := d.codec.Reconstruct(b.shards, b.present); err != nil {
		return nil, false, err
	}

	packets := make([][]byte, 0, int(b.header.DataCount))
	for i := 0; i < int(b.header.DataCount); i++ {
		if b.delivered[i] {
			continue
		}
		n := int(b.header.OriginalLengths[i])
		if n > len(b.shards[i]) {
			return nil, false, fmt.Errorf("fec: reconstructed packet %d length overflow", i)
		}
		packets = append(packets, append([]byte(nil), b.shards[i][:n]...))
		b.delivered[i] = true
	}
	delete(d.blocks, blockID)
	d.markCompleted(blockID)
	return packets, true, nil
}

func allDataDelivered(b *decodeBlock) bool {
	if !b.final {
		return false
	}
	for i := 0; i < int(b.header.DataCount); i++ {
		if !b.delivered[i] {
			return false
		}
	}
	return true
}

func sameBlockHeader(a, b BlockHeader) bool {
	return a.BlockID == b.BlockID && a.DataCount == b.DataCount && a.ShardSize == b.ShardSize && a.OriginalLengths == b.OriginalLengths
}

func (d *BlockDecoder) markCompleted(id uint32) {
	d.completed[id] = struct{}{}
	d.completedQ = append(d.completedQ, id)
	if len(d.completedQ) > d.maxCompleted {
		old := d.completedQ[0]
		d.completedQ = d.completedQ[1:]
		delete(d.completed, old)
	}
}
