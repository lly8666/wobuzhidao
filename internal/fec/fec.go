package fec

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	DataShards   = 20
	ParityShards = 20
	TotalShards  = DataShards + ParityShards

	HeaderVersion = 1
	HeaderSize    = 56
)

var (
	ErrTooManyMissing  = errors.New("fec: too many missing shards")
	ErrInvalidShardSet = errors.New("fec: invalid shard set")
	ErrPoolExhausted   = errors.New("fec: buffer pool exhausted")
)

// Codec is the replaceable erasure-code boundary used by WBD. Implementations
// must be systematic: shards [0, DataShards) are the original data shards.
type Codec interface {
	Encode(shards [][]byte) error
	Reconstruct(shards [][]byte, present []bool) error
}

// ReedSolomon20x20 is a fixed systematic (20 data + 20 parity) GF(256)
// Reed-Solomon codec. It deliberately exposes no stream/retransmission state.
type ReedSolomon20x20 struct {
	generator [TotalShards][DataShards]byte
}

func NewReedSolomon20x20() *ReedSolomon20x20 {
	return &ReedSolomon20x20{generator: buildGenerator()}
}

func (r *ReedSolomon20x20) Encode(shards [][]byte) error {
	shardSize, err := validateShards(shards)
	if err != nil {
		return err
	}
	for p := 0; p < ParityShards; p++ {
		out := shards[DataShards+p]
		clear(out)
		row := r.generator[DataShards+p]
		for d := 0; d < DataShards; d++ {
			coef := row[d]
			if coef == 0 {
				continue
			}
			in := shards[d]
			for i := 0; i < shardSize; i++ {
				out[i] ^= gfMul(coef, in[i])
			}
		}
	}
	return nil
}

func (r *ReedSolomon20x20) Reconstruct(shards [][]byte, present []bool) error {
	shardSize, err := validateShards(shards)
	if err != nil {
		return err
	}
	if len(present) != TotalShards {
		return fmt.Errorf("%w: present=%d want=%d", ErrInvalidShardSet, len(present), TotalShards)
	}

	available := 0
	for _, ok := range present {
		if ok {
			available++
		}
	}
	if available < DataShards {
		return ErrTooManyMissing
	}
	if available == TotalShards {
		return nil
	}

	var selected [DataShards]int
	var decode [DataShards][DataShards]byte
	n := 0
	for i, ok := range present {
		if !ok {
			continue
		}
		selected[n] = i
		decode[n] = r.generator[i]
		n++
		if n == DataShards {
			break
		}
	}
	inverse, err := invertMatrix(decode)
	if err != nil {
		return fmt.Errorf("fec: invert decode matrix: %w", err)
	}

	// Recover missing systematic data from the selected available shards.
	for d := 0; d < DataShards; d++ {
		if present[d] {
			continue
		}
		out := shards[d]
		clear(out)
		row := inverse[d]
		for s, idx := range selected {
			coef := row[s]
			if coef == 0 {
				continue
			}
			in := shards[idx]
			for i := 0; i < shardSize; i++ {
				out[i] ^= gfMul(coef, in[i])
			}
		}
		present[d] = true
	}

	// Missing parity can now be regenerated from the recovered systematic data.
	for p := DataShards; p < TotalShards; p++ {
		if present[p] {
			continue
		}
		out := shards[p]
		clear(out)
		row := r.generator[p]
		for d := 0; d < DataShards; d++ {
			coef := row[d]
			if coef == 0 {
				continue
			}
			in := shards[d]
			for i := 0; i < shardSize; i++ {
				out[i] ^= gfMul(coef, in[i])
			}
		}
		present[p] = true
	}
	return nil
}

func validateShards(shards [][]byte) (int, error) {
	if len(shards) != TotalShards {
		return 0, fmt.Errorf("%w: shards=%d want=%d", ErrInvalidShardSet, len(shards), TotalShards)
	}
	shardSize := len(shards[0])
	if shardSize == 0 {
		return 0, fmt.Errorf("%w: zero shard size", ErrInvalidShardSet)
	}
	for i, shard := range shards {
		if len(shard) != shardSize {
			return 0, fmt.Errorf("%w: shard %d size=%d want=%d", ErrInvalidShardSet, i, len(shard), shardSize)
		}
	}
	return shardSize, nil
}

// BufferPool bounds FEC workspace memory. It owns a fixed number of equal-size
// buffers and never allocates after construction.
type BufferPool struct {
	shardSize int
	buffers   chan []byte
}

func NewBufferPool(shardSize, buffers int) (*BufferPool, error) {
	if shardSize <= 0 || buffers <= 0 {
		return nil, fmt.Errorf("fec: invalid pool shardSize=%d buffers=%d", shardSize, buffers)
	}
	p := &BufferPool{shardSize: shardSize, buffers: make(chan []byte, buffers)}
	for i := 0; i < buffers; i++ {
		p.buffers <- make([]byte, shardSize)
	}
	return p, nil
}

func (p *BufferPool) Get() ([]byte, error) {
	select {
	case b := <-p.buffers:
		clear(b)
		return b, nil
	default:
		return nil, ErrPoolExhausted
	}
}

func (p *BufferPool) Put(b []byte) error {
	if len(b) != p.shardSize {
		return fmt.Errorf("fec: returned buffer size=%d want=%d", len(b), p.shardSize)
	}
	select {
	case p.buffers <- b:
		return nil
	default:
		return errors.New("fec: buffer pool overfill")
	}
}

func (p *BufferPool) Capacity() int  { return cap(p.buffers) }
func (p *BufferPool) Available() int { return len(p.buffers) }
func (p *BufferPool) ShardSize() int { return p.shardSize }

// BlockHeader is repeated with each shard so packet lengths survive arbitrary
// shard loss/reorder. Source packets are padded only inside the FEC block.
type BlockHeader struct {
	BlockID          uint32
	ShardIndex       uint8
	DataCount        uint8
	ShardSize        uint16
	OriginalLengths [DataShards]uint16
}

func (h BlockHeader) MarshalBinary() ([]byte, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}
	b := make([]byte, HeaderSize)
	b[0], b[1] = 'W', 'F'
	b[2] = HeaderVersion
	b[3] = 0
	binary.BigEndian.PutUint32(b[4:8], h.BlockID)
	b[8] = h.ShardIndex
	b[9] = DataShards
	b[10] = ParityShards
	b[11] = h.DataCount
	binary.BigEndian.PutUint16(b[12:14], h.ShardSize)
	// bytes 14:16 are reserved for future flags/format expansion.
	for i, n := range h.OriginalLengths {
		binary.BigEndian.PutUint16(b[16+i*2:18+i*2], n)
	}
	return b, nil
}

func ParseBlockHeader(b []byte) (BlockHeader, error) {
	var h BlockHeader
	if len(b) < HeaderSize {
		return h, fmt.Errorf("fec: header too short: %d", len(b))
	}
	if b[0] != 'W' || b[1] != 'F' || b[2] != HeaderVersion {
		return h, errors.New("fec: invalid header magic/version")
	}
	if b[9] != DataShards || b[10] != ParityShards {
		return h, errors.New("fec: incompatible shard geometry")
	}
	h.BlockID = binary.BigEndian.Uint32(b[4:8])
	h.ShardIndex = b[8]
	h.DataCount = b[11]
	h.ShardSize = binary.BigEndian.Uint16(b[12:14])
	for i := range h.OriginalLengths {
		h.OriginalLengths[i] = binary.BigEndian.Uint16(b[16+i*2 : 18+i*2])
	}
	return h, h.Validate()
}

func (h BlockHeader) Validate() error {
	if h.ShardIndex >= TotalShards {
		return fmt.Errorf("fec: shard index %d out of range", h.ShardIndex)
	}
	if h.DataCount == 0 || h.DataCount > DataShards {
		return fmt.Errorf("fec: data count %d out of range", h.DataCount)
	}
	if h.ShardSize == 0 {
		return errors.New("fec: zero shard size")
	}
	for i, n := range h.OriginalLengths {
		if i < int(h.DataCount) {
			if n > h.ShardSize {
				return fmt.Errorf("fec: packet %d length=%d exceeds shard size=%d", i, n, h.ShardSize)
			}
		} else if n != 0 {
			return fmt.Errorf("fec: unused packet %d has non-zero length=%d", i, n)
		}
	}
	return nil
}

var gfExp [512]byte
var gfLog [256]byte

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[byte(x)] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	for i := 255; i < len(gfExp); i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func gfInv(a byte) byte {
	if a == 0 {
		panic("fec: inverse of zero")
	}
	return gfExp[255-int(gfLog[a])]
}

func gfPow(a byte, n int) byte {
	if n == 0 {
		return 1
	}
	if a == 0 {
		return 0
	}
	return gfExp[(int(gfLog[a])*n)%255]
}

func buildGenerator() [TotalShards][DataShards]byte {
	var vandermonde [TotalShards][DataShards]byte
	for r := 0; r < TotalShards; r++ {
		x := byte(r + 1)
		for c := 0; c < DataShards; c++ {
			vandermonde[r][c] = gfPow(x, c)
		}
	}
	var top [DataShards][DataShards]byte
	copy(top[:], vandermonde[:DataShards])
	inv, err := invertMatrix(top)
	if err != nil {
		panic(err)
	}
	var generator [TotalShards][DataShards]byte
	for r := 0; r < TotalShards; r++ {
		for c := 0; c < DataShards; c++ {
			var v byte
			for k := 0; k < DataShards; k++ {
				v ^= gfMul(vandermonde[r][k], inv[k][c])
			}
			generator[r][c] = v
		}
	}
	return generator
}

func invertMatrix(in [DataShards][DataShards]byte) ([DataShards][DataShards]byte, error) {
	var aug [DataShards][DataShards * 2]byte
	for r := 0; r < DataShards; r++ {
		copy(aug[r][:DataShards], in[r][:])
		aug[r][DataShards+r] = 1
	}
	for c := 0; c < DataShards; c++ {
		pivot := c
		for pivot < DataShards && aug[pivot][c] == 0 {
			pivot++
		}
		if pivot == DataShards {
			return [DataShards][DataShards]byte{}, errors.New("singular matrix")
		}
		if pivot != c {
			aug[pivot], aug[c] = aug[c], aug[pivot]
		}
		invPivot := gfInv(aug[c][c])
		for j := 0; j < DataShards*2; j++ {
			aug[c][j] = gfMul(aug[c][j], invPivot)
		}
		for r := 0; r < DataShards; r++ {
			if r == c || aug[r][c] == 0 {
				continue
			}
			factor := aug[r][c]
			for j := 0; j < DataShards*2; j++ {
				aug[r][j] ^= gfMul(factor, aug[c][j])
			}
		}
	}
	var out [DataShards][DataShards]byte
	for r := 0; r < DataShards; r++ {
		copy(out[r][:], aug[r][DataShards:])
	}
	return out, nil
}
