package fec

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrPacketTooLarge = errors.New("fec: packet too large")
	ErrDecoderFull    = errors.New("fec: decoder block window full")
	ErrHeaderMismatch = errors.New("fec: inconsistent block header")
)

// BlockEncoder groups packet-preserving datagrams into fixed 20-data-shard
// blocks. Full blocks flush immediately; partial blocks flush on FlushAfter.
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
	// The decoder recreates those zeros from DataCount. All 20 parity shards are
	// still emitted so the real source packets retain the fixed 20-parity budget.
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
	header  BlockHeader
	shards  [][]byte
	present []bool
	count   int
}

// BlockDecoder keeps a bounded number of in-flight blocks and a bounded recent
// completion set so late parity/source shards do not recreate completed blocks.
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
	if _, ok := d.completed[h.BlockID]; ok {
		return nil, false, nil
	}

	b := d.blocks[h.BlockID]
	if b == nil {
		if len(d.blocks) >= d.maxBlocks {
			return nil, false, ErrDecoderFull
		}
		b = &decodeBlock{header: h, shards: make([][]byte, TotalShards), present: make([]bool, TotalShards)}
		for i := range b.shards {
			b.shards[i] = make([]byte, h.ShardSize)
		}
		// Unused systematic shards in a partial block are known zeros and count
		// as available equations without consuming wire bytes.
		for i := int(h.DataCount); i < DataShards; i++ {
			b.present[i] = true
			b.count++
		}
		d.blocks[h.BlockID] = b
	} else if !sameBlockHeader(b.header, h) {
		return nil, false, ErrHeaderMismatch
	}

	idx := int(h.ShardIndex)
	if b.present[idx] {
		return nil, false, nil
	}
	copy(b.shards[idx], datagram[HeaderSize:])
	b.present[idx] = true
	b.count++
	if b.count < DataShards {
		return nil, false, nil
	}
	if err := d.codec.Reconstruct(b.shards, b.present); err != nil {
		return nil, false, err
	}

	packets := make([][]byte, int(b.header.DataCount))
	for i := range packets {
		n := int(b.header.OriginalLengths[i])
		if n > len(b.shards[i]) {
			return nil, false, fmt.Errorf("fec: reconstructed packet %d length overflow", i)
		}
		packets[i] = append([]byte(nil), b.shards[i][:n]...)
	}
	delete(d.blocks, h.BlockID)
	d.markCompleted(h.BlockID)
	return packets, true, nil
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
