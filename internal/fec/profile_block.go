package fec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const ProfileHeaderVersion byte = 2

// ProfileBlockHeader keeps the original 56-byte WBD FEC layout but makes the
// already-present parity byte authoritative. Version 1 remains exactly the
// qualified 20:20 format; version 2 admits K=20 and R=1..20.
type ProfileBlockHeader struct {
	BlockID          uint32
	ShardIndex       uint8
	ParityShards     uint8
	DataCount        uint8
	ShardSize        uint16
	OriginalLengths [DataShards]uint16
	Streaming        bool
}

func (h ProfileBlockHeader) TotalShards() int { return DataShards + int(h.ParityShards) }

func (h ProfileBlockHeader) Validate() error {
	if h.ParityShards == 0 || h.ParityShards > ParityShards {
		return fmt.Errorf("fec: profile parity %d out of range", h.ParityShards)
	}
	if int(h.ShardIndex) >= h.TotalShards() {
		return fmt.Errorf("fec: profile shard index %d out of range total=%d", h.ShardIndex, h.TotalShards())
	}
	if h.DataCount == 0 || h.DataCount > DataShards {
		return fmt.Errorf("fec: profile data count %d out of range", h.DataCount)
	}
	if h.ShardSize == 0 {
		return errors.New("fec: profile zero shard size")
	}
	if h.Streaming {
		idx := int(h.ShardIndex)
		if idx >= DataShards || h.DataCount != DataShards || h.OriginalLengths[idx] != h.ShardSize {
			return ErrHeaderMismatch
		}
		for i, n := range h.OriginalLengths {
			if i != idx && n != 0 {
				return ErrHeaderMismatch
			}
		}
		return nil
	}
	for i, n := range h.OriginalLengths {
		if i < int(h.DataCount) {
			if n == 0 || n > h.ShardSize {
				return fmt.Errorf("fec: profile packet %d length=%d shard=%d", i, n, h.ShardSize)
			}
		} else if n != 0 {
			return fmt.Errorf("fec: profile unused packet %d length=%d", i, n)
		}
	}
	return nil
}

func (h ProfileBlockHeader) MarshalBinary() ([]byte, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}
	b := make([]byte, HeaderSize)
	b[0], b[1], b[2] = 'W', 'F', ProfileHeaderVersion
	binary.BigEndian.PutUint32(b[4:8], h.BlockID)
	b[8] = h.ShardIndex
	b[9] = DataShards
	b[10] = h.ParityShards
	b[11] = h.DataCount
	binary.BigEndian.PutUint16(b[12:14], h.ShardSize)
	if h.Streaming {
		binary.BigEndian.PutUint16(b[14:16], headerFlagStreamingSystematic)
	}
	for i, n := range h.OriginalLengths {
		binary.BigEndian.PutUint16(b[16+i*2:18+i*2], n)
	}
	return b, nil
}

func ParseProfileBlockHeader(b []byte) (ProfileBlockHeader, error) {
	var h ProfileBlockHeader
	if len(b) < HeaderSize {
		return h, fmt.Errorf("fec: profile header too short: %d", len(b))
	}
	if b[0] != 'W' || b[1] != 'F' || b[2] != ProfileHeaderVersion || b[3] != 0 || b[9] != DataShards {
		return h, errors.New("fec: invalid profile header magic/version/geometry")
	}
	flags := binary.BigEndian.Uint16(b[14:16])
	if flags & ^headerFlagStreamingSystematic != 0 {
		return h, ErrHeaderMismatch
	}
	h.BlockID = binary.BigEndian.Uint32(b[4:8])
	h.ShardIndex = b[8]
	h.ParityShards = b[10]
	h.DataCount = b[11]
	h.ShardSize = binary.BigEndian.Uint16(b[12:14])
	h.Streaming = flags&headerFlagStreamingSystematic != 0
	for i := range h.OriginalLengths {
		h.OriginalLengths[i] = binary.BigEndian.Uint16(b[16+i*2 : 18+i*2])
	}
	return h, h.Validate()
}

// ProfileFastBlockEncoder is the variable-R counterpart of FastBlockEncoder.
// Every source datagram is emitted immediately. The selected R repair shards
// are emitted only when the K=20 block closes or the partial flush timer fires.
// The hot path uses the same GF lookup-table strategy as the qualified 20:20
// codec; lowering R reduces both repair bytes and parity arithmetic.
type ProfileFastBlockEncoder struct {
	codec         *FastReedSolomon20xR
	parity        int
	maxPacketSize int
	flushAfter    time.Duration
	nextBlockID   uint32
	firstAt       time.Time
	dataCount     int
	shardSize     int
	lengths       [DataShards]uint16
	shardBuf      [TotalShards][]byte
	shardView     [TotalShards][]byte
	wireBuf       [TotalShards][]byte
	out           [ParityShards + 1][]byte
}

func NewProfileFastBlockEncoder(parity, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32) (*ProfileFastBlockEncoder, error) {
	codec, err := NewFastReedSolomon20xR(parity)
	if err != nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || flushAfter <= 0 {
		return nil, errors.New("fec: invalid profile fast block encoder config")
	}
	e := &ProfileFastBlockEncoder{codec: codec, parity: parity, maxPacketSize: maxPacketSize, flushAfter: flushAfter, nextBlockID: firstBlockID}
	for i := 0; i < DataShards+parity; i++ {
		e.shardBuf[i] = make([]byte, maxPacketSize)
		e.wireBuf[i] = make([]byte, HeaderSize+maxPacketSize)
	}
	return e, nil
}

func (e *ProfileFastBlockEncoder) ParityShards() int { return e.parity }
func (e *ProfileFastBlockEncoder) Pending() int      { return e.dataCount }

func (e *ProfileFastBlockEncoder) Add(packet []byte, now time.Time) ([][]byte, error) {
	if len(packet) == 0 || len(packet) > e.maxPacketSize {
		return nil, ErrPacketTooLarge
	}
	if e.dataCount == 0 {
		e.firstAt = now
	}
	idx := e.dataCount
	buf := e.shardBuf[idx]
	clear(buf)
	copy(buf, packet)
	e.lengths[idx] = uint16(len(packet))
	if len(packet) > e.shardSize {
		e.shardSize = len(packet)
	}
	e.dataCount++

	var only [DataShards]uint16
	only[idx] = uint16(len(packet))
	h := ProfileBlockHeader{
		BlockID: e.nextBlockID, ShardIndex: uint8(idx), ParityShards: uint8(e.parity),
		DataCount: DataShards, ShardSize: uint16(len(packet)), OriginalLengths: only, Streaming: true,
	}
	wire := e.wireBuf[idx][:HeaderSize+len(packet)]
	if err := marshalProfileHeaderInto(wire[:HeaderSize], h); err != nil {
		return nil, err
	}
	copy(wire[HeaderSize:], packet)
	e.out[0] = wire
	if e.dataCount == DataShards {
		return e.flushParity(1)
	}
	return e.out[:1], nil
}

func (e *ProfileFastBlockEncoder) FlushDue(now time.Time) ([][]byte, error) {
	if e.dataCount == 0 || now.Sub(e.firstAt) < e.flushAfter {
		return nil, nil
	}
	return e.flushParity(0)
}

func (e *ProfileFastBlockEncoder) Flush() ([][]byte, error) {
	if e.dataCount == 0 {
		return nil, nil
	}
	return e.flushParity(0)
}

func (e *ProfileFastBlockEncoder) flushParity(offset int) ([][]byte, error) {
	dataCount, shardSize := e.dataCount, e.shardSize
	for i := dataCount; i < DataShards; i++ {
		clear(e.shardBuf[i][:shardSize])
	}
	total := DataShards + e.parity
	for i := 0; i < total; i++ {
		e.shardView[i] = e.shardBuf[i][:shardSize]
	}
	if err := e.codec.Encode(e.shardView[:total]); err != nil {
		return nil, err
	}
	for p := 0; p < e.parity; p++ {
		idx := DataShards + p
		h := ProfileBlockHeader{
			BlockID: e.nextBlockID, ShardIndex: uint8(idx), ParityShards: uint8(e.parity),
			DataCount: uint8(dataCount), ShardSize: uint16(shardSize), OriginalLengths: e.lengths,
		}
		b := e.wireBuf[idx][:HeaderSize+shardSize]
		if err := marshalProfileHeaderInto(b[:HeaderSize], h); err != nil {
			return nil, err
		}
		copy(b[HeaderSize:], e.shardBuf[idx][:shardSize])
		e.out[offset+p] = b
	}
	e.nextBlockID++
	e.dataCount = 0
	e.shardSize = 0
	e.firstAt = time.Time{}
	clear(e.lengths[:])
	return e.out[:offset+e.parity], nil
}

func marshalProfileHeaderInto(dst []byte, h ProfileBlockHeader) error {
	if len(dst) < HeaderSize {
		return ErrHeaderMismatch
	}
	b, err := h.MarshalBinary()
	if err != nil {
		return err
	}
	copy(dst, b)
	return nil
}

type profileDecodeBlock struct {
	parity int
	header ProfileBlockHeader
	final  bool
	codec  *FastReedSolomon20xR
	shards [][]byte
	present []bool
	count int
	sources       [DataShards][]byte
	sourcePresent [DataShards]bool
	sourceCount   int
	delivered     [DataShards]bool
}

// ProfileBlockDecoder accepts only FEC wire version 2. Keeping it separate from
// BlockDecoder prevents the variable-R work from changing qualified v1 20:20
// behavior while the preset family is still under qualification.
type ProfileBlockDecoder struct {
	maxPacketSize int
	maxBlocks int
	blocks map[uint32]*profileDecodeBlock
	completed map[uint32]struct{}
	completedQ []uint32
	maxCompleted int
}

func NewProfileBlockDecoder(maxPacketSize, maxBlocks int) (*ProfileBlockDecoder, error) {
	if maxPacketSize <= 0 || maxPacketSize > 0xffff || maxBlocks <= 0 {
		return nil, errors.New("fec: invalid profile block decoder config")
	}
	return &ProfileBlockDecoder{maxPacketSize: maxPacketSize, maxBlocks: maxBlocks, blocks: make(map[uint32]*profileDecodeBlock), completed: make(map[uint32]struct{}), maxCompleted: maxBlocks*4}, nil
}

func (d *ProfileBlockDecoder) InFlight() int { return len(d.blocks) }

func (d *ProfileBlockDecoder) Add(datagram []byte) ([][]byte, bool, error) {
	if len(datagram) < HeaderSize {
		return nil, false, errors.New("fec: profile shard datagram too short")
	}
	h, err := ParseProfileBlockHeader(datagram[:HeaderSize])
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
		codec, err := NewFastReedSolomon20xR(int(h.ParityShards))
		if err != nil {
			return nil, false, err
		}
		b = &profileDecodeBlock{parity: int(h.ParityShards), codec: codec}
		d.blocks[h.BlockID] = b
	} else if b.parity != int(h.ParityShards) {
		return nil, false, ErrHeaderMismatch
	}
	if h.Streaming {
		return d.addStreaming(b, h, datagram[HeaderSize:])
	}
	return d.addFinal(b, h, datagram[HeaderSize:])
}

func (d *ProfileBlockDecoder) addStreaming(b *profileDecodeBlock, h ProfileBlockHeader, payload []byte) ([][]byte, bool, error) {
	idx := int(h.ShardIndex)
	if b.final {
		if idx >= int(b.header.DataCount) || int(b.header.OriginalLengths[idx]) != len(payload) {
			return nil, false, ErrHeaderMismatch
		}
		if !b.present[idx] {
			copy(b.shards[idx], payload)
			b.present[idx] = true
			b.count++
		}
	} else if !b.sourcePresent[idx] {
		b.sources[idx] = append([]byte(nil), payload...)
		b.sourcePresent[idx] = true
		b.sourceCount++
	}
	var out [][]byte
	if !b.delivered[idx] {
		out = append(out, append([]byte(nil), payload...))
		b.delivered[idx] = true
	}
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
	return append(out, recovered...), done, nil
}

func (d *ProfileBlockDecoder) addFinal(b *profileDecodeBlock, h ProfileBlockHeader, payload []byte) ([][]byte, bool, error) {
	if !b.final {
		if err := d.finalize(b, h); err != nil {
			return nil, false, err
		}
	} else if !sameProfileHeader(b.header, h) {
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

func (d *ProfileBlockDecoder) finalize(b *profileDecodeBlock, h ProfileBlockHeader) error {
	b.header, b.final = h, true
	total := DataShards + b.parity
	b.shards = make([][]byte, total)
	b.present = make([]bool, total)
	for i := range b.shards {
		b.shards[i] = make([]byte, h.ShardSize)
	}
	for i := int(h.DataCount); i < DataShards; i++ {
		b.present[i] = true
		b.count++
	}
	for i := 0; i < int(h.DataCount); i++ {
		if !b.sourcePresent[i] {
			continue
		}
		if len(b.sources[i]) != int(h.OriginalLengths[i]) {
			return ErrHeaderMismatch
		}
		copy(b.shards[i], b.sources[i])
		b.present[i] = true
		b.count++
		b.sources[i] = nil
	}
	return nil
}

func (d *ProfileBlockDecoder) maybeComplete(blockID uint32, b *profileDecodeBlock) ([][]byte, bool, error) {
	if profileAllDataDelivered(b) {
		delete(d.blocks, blockID)
		d.markCompleted(blockID)
		return nil, true, nil
	}
	if b.count < DataShards {
		return nil, false, nil
	}
	if err := b.codec.Reconstruct(b.shards, b.present); err != nil {
		return nil, false, err
	}
	out := make([][]byte, 0, int(b.header.DataCount))
	for i := 0; i < int(b.header.DataCount); i++ {
		if b.delivered[i] {
			continue
		}
		n := int(b.header.OriginalLengths[i])
		if n > len(b.shards[i]) {
			return nil, false, ErrHeaderMismatch
		}
		out = append(out, append([]byte(nil), b.shards[i][:n]...))
		b.delivered[i] = true
	}
	delete(d.blocks, blockID)
	d.markCompleted(blockID)
	return out, true, nil
}

func profileAllDataDelivered(b *profileDecodeBlock) bool {
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

func sameProfileHeader(a, b ProfileBlockHeader) bool {
	return a.BlockID == b.BlockID && a.ParityShards == b.ParityShards && a.DataCount == b.DataCount && a.ShardSize == b.ShardSize && a.OriginalLengths == b.OriginalLengths
}

func (d *ProfileBlockDecoder) markCompleted(id uint32) {
	d.completed[id] = struct{}{}
	d.completedQ = append(d.completedQ, id)
	if len(d.completedQ) > d.maxCompleted {
		old := d.completedQ[0]
		d.completedQ = d.completedQ[1:]
		delete(d.completed, old)
	}
}
