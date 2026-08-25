package fec

import (
	"encoding/binary"
	"errors"
	"time"
)

// FastBlockEncoder is the steady-state transport encoder. It owns fixed shard
// and wire buffers sized at construction time, so full 20-packet blocks do not
// allocate or copy through temporary per-block shard matrices. Returned wire
// slices remain valid until the next Add/Flush operation; the UDP proxy sends
// them synchronously before accepting the next plaintext datagram.
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
	out           [TotalShards][]byte
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
	buf := e.shardBuf[e.dataCount]
	clear(buf)
	copy(buf, packet)
	e.lengths[e.dataCount] = uint16(len(packet))
	if len(packet) > e.shardSize {
		e.shardSize = len(packet)
	}
	e.dataCount++
	if e.dataCount == DataShards {
		return e.flush()
	}
	return nil, nil
}

func (e *FastBlockEncoder) FlushDue(now time.Time) ([][]byte, error) {
	if e.dataCount == 0 || now.Sub(e.firstAt) < e.flushAfter {
		return nil, nil
	}
	return e.flush()
}

func (e *FastBlockEncoder) Flush() ([][]byte, error) {
	if e.dataCount == 0 {
		return nil, nil
	}
	return e.flush()
}

func (e *FastBlockEncoder) Pending() int { return e.dataCount }

func (e *FastBlockEncoder) flush() ([][]byte, error) {
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

	nout := 0
	emit := func(index int) {
		b := e.wireBuf[index][:HeaderSize+shardSize]
		marshalFastHeader(b[:HeaderSize], e.nextBlockID, index, dataCount, shardSize, e.lengths)
		copy(b[HeaderSize:], e.shardBuf[index][:shardSize])
		e.out[nout] = b
		nout++
	}
	for i := 0; i < dataCount; i++ {
		emit(i)
	}
	for i := DataShards; i < TotalShards; i++ {
		emit(i)
	}

	e.nextBlockID++
	e.dataCount = 0
	e.shardSize = 0
	e.firstAt = time.Time{}
	clear(e.lengths[:])
	return e.out[:nout], nil
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
