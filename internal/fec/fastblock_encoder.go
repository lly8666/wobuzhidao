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
// enough parity to preserve the fixed 100% redundancy ratio: a full block is
// 20 source + 20 parity, while a partial N-packet block is N source + N parity.
// Unused source slots are authoritative known-zero shards at the decoder, so
// transmitting 20 parity datagrams for a one- or two-packet partial block only
// amplified wire traffic without increasing recoverability.
//
// Returned wire slices remain valid until the corresponding backing slot is
// reused. The UDP proxy sends returned slices synchronously before the next Add.
type FastBlockEncoderStats struct {
	SourceShards   uint64 `json:"source_shards"`
	SourceBytes    uint64 `json:"source_bytes"`
	ParityShards   uint64 `json:"parity_shards"`
	ParityBytes    uint64 `json:"parity_bytes"`
	FullBlocks     uint64 `json:"full_blocks"`
	PartialBlocks  uint64 `json:"partial_blocks"`
	PendingSources int    `json:"pending_sources"`
	SizeClasses    int    `json:"size_classes,omitempty"`
	PendingBlocks  int    `json:"pending_blocks,omitempty"`
}

type FastBlockEncoder struct {
	codec         Codec
	activeCodec   activeParityEncoder
	parityShards  int
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
	stats         FastBlockEncoderStats
}

type activeParityEncoder interface {
	EncodeActive(shards [][]byte, dataCount, parityCount int) error
}

func NewFastBlockEncoder(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32) (*FastBlockEncoder, error) {
	return NewFastBlockEncoderWithParity(codec, maxPacketSize, flushAfter, firstBlockID, ParityShards)
}

func NewFastBlockEncoderWithParity(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32, parityShards int) (*FastBlockEncoder, error) {
	if codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || flushAfter <= 0 || !validParityCount(parityShards) {
		return nil, errors.New("fec: invalid fast block encoder config")
	}
	e := &FastBlockEncoder{
		codec: codec, parityShards: parityShards, maxPacketSize: maxPacketSize, flushAfter: flushAfter, nextBlockID: firstBlockID,
	}
	e.activeCodec, _ = codec.(activeParityEncoder)
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
	marshalStreamingSourceHeader(wire[:HeaderSize], e.nextBlockID, idx, len(packet), e.parityShards)
	copy(wire[HeaderSize:], packet)
	e.out[0] = wire

	if e.dataCount == DataShards {
		out, err := e.flushParity(1)
		if err != nil {
			return nil, err
		}
		e.stats.SourceShards++
		e.stats.SourceBytes += uint64(len(wire))
		return out, nil
	}
	e.stats.SourceShards++
	e.stats.SourceBytes += uint64(len(wire))
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

// NextFlushDeadline reports the first source's absolute partial-parity due
// time. A full group already flushed on Add; an empty group has no deadline.
// The owner calls this under its existing transmit lock.
func (e *FastBlockEncoder) NextFlushDeadline() time.Time {
	if e == nil || e.dataCount == 0 {
		return time.Time{}
	}
	return e.firstAt.Add(e.flushAfter)
}

func (e *FastBlockEncoder) Pending() int { return e.dataCount }

func (e *FastBlockEncoder) Stats() FastBlockEncoderStats {
	if e == nil {
		return FastBlockEncoderStats{}
	}
	out := e.stats
	out.PendingSources = e.dataCount
	return out
}

func (e *FastBlockEncoder) flushParity(offset int) ([][]byte, error) {
	dataCount := e.dataCount
	shardSize := e.shardSize
	// FastReedSolomon20x20.EncodeActive uses only source slots [0,dataCount).
	// Clearing the remaining (20-dataCount) full shard buffers for every
	// sparse 8ms partial block costs memory bandwidth but cannot affect parity.
	// Keep the authoritative known-zero padded inputs for every other codec:
	// generic Encode (and any future active implementation) may read them.
	if _, ignoresPadding := e.codec.(*FastReedSolomon20x20); !ignoresPadding {
		for i := dataCount; i < DataShards; i++ {
			clear(e.shardBuf[i][:shardSize])
		}
	}
	for i := 0; i < TotalShards; i++ {
		e.shardView[i] = e.shardBuf[i][:shardSize]
	}
	parityCount := dataCount
	if parityCount > e.parityShards {
		parityCount = e.parityShards
	}
	var err error
	if e.activeCodec != nil {
		err = e.activeCodec.EncodeActive(e.shardView[:], dataCount, parityCount)
	} else {
		err = e.codec.Encode(e.shardView[:])
	}
	if err != nil {
		return nil, err
	}

	// For a partial block, DataShards-dataCount systematic slots are known zero
	// and the decoder marks them present. dataCount parity shards are therefore
	// sufficient even if every real source datagram is lost: known zeros plus
	// parity still provide the 20 equations required by the fixed 20x20 codec.
	for p := 0; p < parityCount; p++ {
		index := DataShards + p
		b := e.wireBuf[index][:HeaderSize+shardSize]
		marshalFastHeader(b[:HeaderSize], e.nextBlockID, index, dataCount, shardSize, e.parityShards, e.lengths)
		copy(b[HeaderSize:], e.shardBuf[index][:shardSize])
		e.out[offset+p] = b
		e.stats.ParityBytes += uint64(len(b))
	}

	e.stats.ParityShards += uint64(parityCount)
	if dataCount == DataShards {
		e.stats.FullBlocks++
	} else {
		e.stats.PartialBlocks++
	}
	e.nextBlockID++
	e.dataCount = 0
	e.shardSize = 0
	e.firstAt = time.Time{}
	clear(e.lengths[:])
	return e.out[:offset+parityCount], nil
}

func marshalStreamingSourceHeader(dst []byte, blockID uint32, shardIndex, packetLen, parityShards int) {
	clear(dst)
	dst[0], dst[1] = 'W', 'F'
	dst[2] = HeaderVersion
	binary.BigEndian.PutUint32(dst[4:8], blockID)
	dst[8] = byte(shardIndex)
	dst[9] = DataShards
	dst[10] = byte(parityShards)
	// The final block DataCount is not known yet. DataShards is a legal placeholder
	// for ParseBlockHeader; headerFlagStreamingSystematic defines its provisional
	// meaning. Only this source's original length is authoritative.
	dst[11] = DataShards
	binary.BigEndian.PutUint16(dst[12:14], uint16(packetLen))
	binary.BigEndian.PutUint16(dst[14:16], headerFlagStreamingSystematic)
	binary.BigEndian.PutUint16(dst[16+shardIndex*2:18+shardIndex*2], uint16(packetLen))
}

func marshalFastHeader(dst []byte, blockID uint32, shardIndex, dataCount, shardSize, parityShards int, lengths [DataShards]uint16) {
	clear(dst)
	dst[0], dst[1] = 'W', 'F'
	dst[2] = HeaderVersion
	binary.BigEndian.PutUint32(dst[4:8], blockID)
	dst[8] = byte(shardIndex)
	dst[9] = DataShards
	dst[10] = byte(parityShards)
	dst[11] = byte(dataCount)
	binary.BigEndian.PutUint16(dst[12:14], uint16(shardSize))
	for i, n := range lengths {
		binary.BigEndian.PutUint16(dst[16+i*2:18+i*2], n)
	}
}
