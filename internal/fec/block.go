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

// Keep enough compact retired-block delivery state to cover the product's
// longest weak-link recovery horizon without letting a long-running transport
// grow memory with every FEC BlockID. At the default 8ms partial flush this is
// more than 65 seconds of one-partial-block-per-flush history. Each entry keeps
// only compact delivery/metadata state, never FEC shard payloads.
const maxRetiredBlocks = 8192

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

	// Streaming sources are retained only as reconstruction inputs. Their
	// payload has already been delivered to the inner path on first arrival.
	sources       [DataShards][]byte
	sourcePresent [DataShards]bool
	sourceCount   int
	delivered     [DataShards]bool
}

// retiredBlock is the bounded ARQ-only pressure state. It never retains shard
// payloads. Before final metadata arrives, OriginalLengths records only lengths
// of already-delivered systematic sources so the later authoritative header can
// be checked without reopening a heavy reconstruction slot.
type retiredBlock struct {
	header    BlockHeader
	final     bool
	delivered uint32
}

// BlockDecoder keeps a bounded number of full reconstruction states and compact
// exact completion history so arbitrarily late parity/source retransmissions
// cannot recreate blocks that already delivered their originals. Streaming
// systematic shards are returned immediately; parity later supplies final
// metadata and reconstructs only sources that never arrived.
//
// Under decoder pressure, any old block whose retained payload is no longer
// required for first delivery can be compacted into bounded retired state. If
// every heavy slot is unsafe to shed, a new streaming block itself enters that
// bounded ARQ-only state instead of turning maxBlocks into a permanent drop
// wall. Reference/final-only input preserves ErrDecoderFull when no safe victim
// exists because it may still contain undelivered source payloads.
type BlockDecoder struct {
	codec         Codec
	maxPacketSize int
	maxBlocks     int
	blocks        map[uint32]*decodeBlock
	blockOrder    []uint32
	blockHead     int

	retired      map[uint32]retiredBlock
	retiredOrder []uint32
	retiredHead  int

	completed completedBlockSet
}

func NewBlockDecoder(codec Codec, maxPacketSize, maxBlocks int) (*BlockDecoder, error) {
	if codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || maxBlocks <= 0 {
		return nil, errors.New("fec: invalid block decoder config")
	}
	return &BlockDecoder{
		codec: codec, maxPacketSize: maxPacketSize, maxBlocks: maxBlocks,
		blocks: make(map[uint32]*decodeBlock), retired: make(map[uint32]retiredBlock),
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
	if r, ok := d.retired[h.BlockID]; ok {
		return d.addRetired(h, datagram[HeaderSize:], streaming, r)
	}

	b := d.blocks[h.BlockID]
	if b == nil {
		if len(d.blocks) >= d.maxBlocks {
			if id, victim, ok := d.oldestRetirableBefore(h.BlockID); ok {
				d.retireBlock(id, victim)
			} else if streaming {
				r := retiredBlock{}
				d.addRetiredState(h.BlockID, r)
				if d.completed.contains(h.BlockID) {
					return nil, false, nil
				}
				return d.addRetired(h, datagram[HeaderSize:], true, d.retired[h.BlockID])
			} else {
				return nil, false, ErrDecoderFull
			}
		}
		b = &decodeBlock{}
		d.blocks[h.BlockID] = b
		d.blockOrder = append(d.blockOrder, h.BlockID)
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

func (d *BlockDecoder) addRetired(h BlockHeader, payload []byte, streaming bool, r retiredBlock) ([][]byte, bool, error) {
	if streaming {
		idx := int(h.ShardIndex)
		if r.final {
			if idx >= int(r.header.DataCount) || int(r.header.OriginalLengths[idx]) != len(payload) {
				return nil, false, ErrHeaderMismatch
			}
		}
		bit := uint32(1) << uint(idx)
		if r.delivered&bit != 0 {
			return nil, false, nil
		}
		if !r.final {
			r.header.OriginalLengths[idx] = uint16(len(payload))
		}
		r.delivered |= bit
		d.retired[h.BlockID] = r
		out := [][]byte{append([]byte(nil), payload...)}
		if retiredAllDelivered(r) {
			d.markCompleted(h.BlockID)
			return out, true, nil
		}
		return out, false, nil
	}

	if !r.final {
		for i := 0; i < DataShards; i++ {
			bit := uint32(1) << uint(i)
			if r.delivered&bit == 0 {
				continue
			}
			if i >= int(h.DataCount) || r.header.OriginalLengths[i] != h.OriginalLengths[i] {
				return nil, false, ErrHeaderMismatch
			}
		}
		r.header = h
		r.final = true
	} else if !sameBlockHeader(r.header, h) {
		return nil, false, ErrHeaderMismatch
	}

	var out [][]byte
	idx := int(h.ShardIndex)
	if idx < int(h.DataCount) {
		bit := uint32(1) << uint(idx)
		if r.delivered&bit == 0 {
			n := int(h.OriginalLengths[idx])
			if n > len(payload) {
				return nil, false, ErrHeaderMismatch
			}
			out = append(out, append([]byte(nil), payload[:n]...))
			r.delivered |= bit
		}
	}
	d.retired[h.BlockID] = r
	if retiredAllDelivered(r) {
		d.markCompleted(h.BlockID)
		return out, true, nil
	}
	return out, false, nil
}

func retiredAllDelivered(r retiredBlock) bool {
	count := DataShards
	if r.final {
		count = int(r.header.DataCount)
	}
	mask := uint32(1)<<uint(count) - 1
	return r.delivered&mask == mask
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

func canRetireBlock(b *decodeBlock) bool {
	if b == nil {
		return false
	}
	if !b.final {
		// Before final metadata only streaming systematic sources can be present,
		// and addStreamingSource delivers each one before retaining its copy.
		return true
	}
	for i := 0; i < int(b.header.DataCount); i++ {
		if b.present[i] && !b.delivered[i] {
			return false
		}
	}
	return true
}

func (d *BlockDecoder) oldestRetirableBefore(limit uint32) (uint32, *decodeBlock, bool) {
	for i := d.blockHead; i < len(d.blockOrder); i++ {
		id := d.blockOrder[i]
		b := d.blocks[id]
		if b == nil {
			if i == d.blockHead {
				d.blockHead++
			}
			continue
		}
		if id >= limit || !canRetireBlock(b) {
			continue
		}
		d.compactBlockOrder()
		return id, b, true
	}
	d.compactBlockOrder()
	return 0, nil, false
}

func (d *BlockDecoder) retireBlock(id uint32, b *decodeBlock) {
	if b == nil || !canRetireBlock(b) {
		return
	}
	delete(d.blocks, id)
	r := retiredBlock{header: b.header, final: b.final}
	for i, delivered := range b.delivered {
		if delivered {
			r.delivered |= uint32(1) << uint(i)
		}
		if !b.final && b.sourcePresent[i] {
			r.header.OriginalLengths[i] = uint16(len(b.sources[i]))
		}
	}
	d.addRetiredState(id, r)
	d.compactBlockOrder()
	if retiredAllDelivered(r) {
		d.markCompleted(id)
	}
}

func (d *BlockDecoder) addRetiredState(id uint32, r retiredBlock) {
	d.retired[id] = r
	d.retiredOrder = append(d.retiredOrder, id)
	d.trimRetired()
}

func (d *BlockDecoder) trimRetired() {
	for len(d.retired) > maxRetiredBlocks {
		for d.retiredHead < len(d.retiredOrder) {
			id := d.retiredOrder[d.retiredHead]
			d.retiredHead++
			if _, ok := d.retired[id]; !ok {
				continue
			}
			delete(d.retired, id)
			// Once compact late-delivery history itself ages out, ignore all later
			// shards for that BlockID. This preserves a hard memory ceiling and
			// avoids duplicate reconstruction from very old parity.
			d.completed.add(id)
			break
		}
	}
	d.compactRetiredOrder()
}

func (d *BlockDecoder) compactBlockOrder() {
	for d.blockHead < len(d.blockOrder) {
		if d.blocks[d.blockOrder[d.blockHead]] != nil {
			break
		}
		d.blockHead++
	}
	if d.blockHead >= 1024 && d.blockHead*2 >= len(d.blockOrder) {
		copy(d.blockOrder, d.blockOrder[d.blockHead:])
		d.blockOrder = d.blockOrder[:len(d.blockOrder)-d.blockHead]
		d.blockHead = 0
	}
}

func (d *BlockDecoder) compactRetiredOrder() {
	for d.retiredHead < len(d.retiredOrder) {
		if _, ok := d.retired[d.retiredOrder[d.retiredHead]]; ok {
			break
		}
		d.retiredHead++
	}
	if d.retiredHead >= 1024 && d.retiredHead*2 >= len(d.retiredOrder) {
		copy(d.retiredOrder, d.retiredOrder[d.retiredHead:])
		d.retiredOrder = d.retiredOrder[:len(d.retiredOrder)-d.retiredHead]
		d.retiredHead = 0
	}
}

func (d *BlockDecoder) markCompleted(id uint32) {
	delete(d.retired, id)
	d.completed.add(id)
	d.compactBlockOrder()
	d.compactRetiredOrder()
}
