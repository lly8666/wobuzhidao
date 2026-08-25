package fec

import (
	"encoding/binary"
	"errors"
	"time"
)

// FastBlockEncoder is the performance-first transport encoder. Every complete
// systematic source datagram is emitted immediately on Add, so an unlost inner
// packet never waits for a 20-packet FEC block or the flush timer. The source is
// retained in preallocated shard storage only so parity can be computed later.
//
// When the block fills, or a partial block reaches flushAfter, the encoder emits
// exactly the 20 parity shards with authoritative final block metadata. Total
// wire geometry is unchanged: a full block is still 20 source + 20 parity, and
// a partial N-packet block is still N source + 20 parity.
//
// Returned wire slices remain valid until the corresponding backing slot is
// reused. The UDP proxy sends returned slices synchronously before the next Add.
type FastBlockEncoder struct {
	codec         Codec
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

func NewFastBlockEncoder(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32) (*FastBlockEncoder, error) {
	if codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || flushAfter <= 0 {
		return nil, errors.New("fec: invalid fast block encoder config")
	}
	e := &FastBlockEncoder{
		codec: codec, maxPacketSize: maxPacketSize, flushAfter: flushAfter, nextBlockID: firstBlockID,
	}
	for i := 0; i < TotalShards; i++ {
		e.shardBuf[i] = make([]byte, maxPacketSize)
		e.wireBuf[i] = make([]byte, HeaderSize+maxPacketSize)
	}
	return e, nil
}

func (e *FastBlockEncoder) Add(packet []byte, now time.Time) ([][]byte, error) {
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

	// Provisional source metadata is intentionally self-contained: the packet is
	// complete now, while final DataCount/max ShardSize/other original lengths
	// are unknown until the block closes. Reserved header flag bit 0 tells the
	// WBD decoder it may deliver this payload immediately.
	wire := e.wireBuf[idx][:HeaderSize+len(packet)]
	marshalStreamingSourceHeader(wire[:HeaderSize], e.nextBlockID, idx, len(packet))
	copy(wire[HeaderSize:], packet)
	e.out[0] = wire

	if e.dataCount == DataShards {
		return e.flushParity(1)
	}
	return e.out[:1], nil
}

func (e *FastBlockEncoder) FlushDue(now time.Time) ([][]byte, error) {
	if e.dataCount == 0 || now.Sub(e.firstAt) < e.flushAfter {
		return nil, nil
	}
	return e.flushParity(0)
}

func (e *FastBlockEncoder) Flush() ([][]byte, error) {
	if e.dataCount == 0 {
		return nil, nil
	}
	return e.flushParity(0)
}

func (e *FastBlockEncoder) Pending() int { return e.dataCount }

func (e *FastBlockEncoder) flushParity(offset int) ([][]byte, error) {
	dataCount := e.dataCount
	shardSize := e.shardSize
	for i := dataCount; i < DataShards; i++ {
		clear(e.shardBuf[i][:shardSize])
	}
	for i := 0; i < TotalShards; i++ {
		e.shardView[i] = e.shardBuf[i][:shardSize]
	}
	if err := e.codec.Encode(e.shardView[:]); err != nil {
		return nil, err
	}

	for p := 0; p < ParityShards; p++ {
		index := DataShards + p
		b := e.wireBuf[index][:HeaderSize+shardSize]
		marshalFastHeader(b[:HeaderSize], e.nextBlockID, index, dataCount, shardSize, e.lengths)
		copy(b[HeaderSize:], e.shardBuf[index][:shardSize])
		e.out[offset+p] = b
	}

	e.nextBlockID++
	e.dataCount = 0
	e.shardSize = 0
	e.firstAt = time.Time{}
	clear(e.lengths[:])
	return e.out[:offset+ParityShards], nil
}

func marshalStreamingSourceHeader(dst []byte, blockID uint32, shardIndex, packetLen int) {
	clear(dst)
	dst[0], dst[1] = 'W', 'F'
	dst[2] = HeaderVersion
	binary.BigEndian.PutUint32(dst[4:8], blockID)
	dst[8] = byte(shardIndex)
	dst[9] = DataShards
	dst[10] = ParityShards
	// The final block DataCount is not known yet. DataShards is a legal placeholder
	// for ParseBlockHeader; headerFlagStreamingSystematic defines its provisional
	// meaning. Only this source's original length is authoritative.
	dst[11] = DataShards
	binary.BigEndian.PutUint16(dst[12:14], uint16(packetLen))
	binary.BigEndian.PutUint16(dst[14:16], headerFlagStreamingSystematic)
	binary.BigEndian.PutUint16(dst[16+shardIndex*2:18+shardIndex*2], uint16(packetLen))
}

func marshalFastHeader(dst []byte, blockID uint32, shardIndex, dataCount, shardSize int, lengths [DataShards]uint16) {
	clear(dst)
	dst[0], dst[1] = 'W', 'F'
	dst[2] = HeaderVersion
	binary.BigEndian.PutUint32(dst[4:8], blockID)
	dst[8] = byte(shardIndex)
	dst[9] = DataShards
	dst[10] = ParityShards
	dst[11] = byte(dataCount)
	binary.BigEndian.PutUint16(dst[12:14], uint16(shardSize))
	for i, n := range lengths {
		binary.BigEndian.PutUint16(dst[16+i*2:18+i*2], n)
	}
}
