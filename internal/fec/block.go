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
			BlockID:         e.nextBlockID,
			ShardIndex:      uint8(index),
			DataCount:       uint8(dataCount),
			ShardSize:       uint16(shardSize),
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

// degradedBlock is the bounded-pressure fallback for the streaming production
// path. It deliberately retains no FEC shard payloads: an old incomplete block
// may lose early reconstruction, but later systematic retransmissions can still
// be delivered exactly once while newer blocks keep access to the heavy decoder
// window. Final metadata is retained only to validate those late sources.
type degradedBlock struct {
	header    BlockHeader
	final     bool
	delivered uint32
}

// BlockDecoder keeps a bounded number of heavy in-flight reconstruction blocks,
// compact exact completion history, and lightweight degraded state for blocks
// shed under streaming-window pressure. The heavy maxBlocks limit therefore
// remains a memory bound rather than becoming a permanent forward-progress
// failure when lower-layer ARQ delivers old holes far behind newer BlockIDs.
// Streaming systematic shards are returned immediately; parity later supplies
// final metadata and reconstructs only sources that never arrived while the
// block remains in the heavy window.
type BlockDecoder struct {
	codec         Codec
	maxPacketSize int
	maxBlocks     int
	blocks        map[uint32]*decodeBlock
	degraded      map[uint32]*degradedBlock
	completed     completedBlockSet
}

func NewBlockDecoder(codec Codec, maxPacketSize, maxBlocks int) (*BlockDecoder, error) {
	if codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || maxBlocks <= 0 {
		return nil, errors.New("fec: invalid block decoder config")
	}
	return &BlockDecoder{
		codec: codec, maxPacketSize: maxPacketSize, maxBlocks: maxBlocks,
		blocks: make(map[uint32]*decodeBlock), degraded: make(map[uint32]*degradedBlock),
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
	if flags&^headerFlagStreamingSystematic != 0 {
		return nil, false, ErrHeaderMismatch
	}
	streaming := flags&headerFlagStreamingSystematic != 0
	if streaming {
		if err := validateStreamingHeader(h); err != nil {
			return nil, false, err
		}
	}
	if d.completed.contains(h.BlockID) {
		return nil, false, nil
	}
	if b := d.degraded[h.BlockID]; b != nil {
		return d.addDegraded(h.BlockID, b, h, datagram[HeaderSize:], streaming)
	}

	b := d.blocks[h.BlockID]
	if b == nil {
		if len(d.blocks) >= d.maxBlocks {
			// Production FastBlockEncoder emits a streaming systematic shard as the
			// first useful payload for each block. Prefer shedding an older safe
			// reconstruction block so the newer block retains FEC. If every older
			// heavy block is unsafe to shed, degrade this new streaming block rather
			// than turning maxBlocks into a permanent drop wall. Reference/final-only
			// input keeps the historical ErrDecoderFull behavior when no safe victim
			// exists because it may still hold undelivered source payloads.
			if streaming {
				if victim, ok := d.oldestDegradableBefore(h.BlockID); ok {
					d.degradeHeavy(victim)
				} else {
					light := &degradedBlock{}
					d.degraded[h.BlockID] = light
					return d.addDegraded(h.BlockID, light, h, datagram[HeaderSize:], true)
				}
			} else {
				return nil, false, ErrDecoderFull
			}
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

func canDegradeBlock(b *decodeBlock) bool {
	if b == nil {
		return false
	}
	if !b.final {
		// Before final metadata only streaming systematic sources can be present,
		// and addStreamingSource delivers each of them before retaining its copy.
		return true
	}
	for i := 0; i < int(b.header.DataCount); i++ {
		if b.present[i] && !b.delivered[i] {
			return false
		}
	}
	return true
}

func (d *BlockDecoder) oldestDegradableBefore(limit uint32) (uint32, bool) {
	var oldest uint32
	found := false
	for id, b := range d.blocks {
		if id >= limit || !canDegradeBlock(b) {
			continue
		}
		if !found || id < oldest {
			oldest, found = id, true
		}
	}
	return oldest, found
}

func (d *BlockDecoder) degradeHeavy(id uint32) {
	b := d.blocks[id]
	if b == nil || !canDegradeBlock(b) {
		return
	}
	light := &degradedBlock{final: b.final}
	if b.final {
		light.header = b.header
	}
	for i := 0; i < DataShards; i++ {
		if b.delivered[i] {
			light.delivered |= uint32(1) << uint(i)
		}
		if !b.final && b.sourcePresent[i] {
			light.header.OriginalLengths[i] = uint16(len(b.sources[i]))
		}
	}
	delete(d.blocks, id)
	d.degraded[id] = light
	if degradedComplete(light) {
		delete(d.degraded, id)
		d.markCompleted(id)
	}
}

func (d *BlockDecoder) addDegraded(blockID uint32, b *degradedBlock, h BlockHeader, payload []byte, streaming bool) ([][]byte, bool, error) {
	if streaming {
		idx := int(h.ShardIndex)
		if b.final {
			if idx >= int(b.header.DataCount) || int(b.header.OriginalLengths[idx]) != len(payload) {
				return nil, false, ErrHeaderMismatch
			}
		}
		bit := uint32(1) << uint(idx)
		if b.delivered&bit != 0 {
			return nil, false, nil
		}
		if !b.final {
			b.header.OriginalLengths[idx] = uint16(len(payload))
		}
		b.delivered |= bit
		out := [][]byte{append([]byte(nil), payload...)}
		if degradedComplete(b) {
			delete(d.degraded, blockID)
			d.markCompleted(blockID)
			return out, true, nil
		}
		return out, false, nil
	}

	if !b.final {
		for i := 0; i < DataShards; i++ {
			bit := uint32(1) << uint(i)
			if b.delivered&bit == 0 {
				continue
			}
			if i >= int(h.DataCount) || b.header.OriginalLengths[i] != h.OriginalLengths[i] {
				return nil, false, ErrHeaderMismatch
			}
		}
		b.header = h
		b.final = true
	} else if !sameBlockHeader(b.header, h) {
		return nil, false, ErrHeaderMismatch
	}

	var out [][]byte
	idx := int(h.ShardIndex)
	if idx < int(h.DataCount) {
		bit := uint32(1) << uint(idx)
		if b.delivered&bit == 0 {
			n := int(h.OriginalLengths[idx])
			if n > len(payload) {
				return nil, false, ErrHeaderMismatch
			}
			out = append(out, append([]byte(nil), payload[:n]...))
			b.delivered |= bit
		}
	}
	if degradedComplete(b) {
		delete(d.degraded, blockID)
		d.markCompleted(blockID)
		return out, true, nil
	}
	return out, false, nil
}

func degradedComplete(b *degradedBlock) bool {
	if b == nil {
		return false
	}
	count := DataShards
	if b.final {
		count = int(b.header.DataCount)
	}
	mask := (uint32(1) << uint(count)) - 1
	return b.delivered&mask == mask
}

func (d *BlockDecoder) markCompleted(id uint32) {
	d.completed.add(id)
}
