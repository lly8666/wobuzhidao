package tlsrecord

import (
    "encoding/binary"
    "math/bits"
)

// headerMaskKey holds only immutable ChaCha key words. A Sealer/Opener owns
// this value for its lifetime; records provide fresh independent sample state.
// RFC 8439 section 2.3 (IETF ChaCha20) and x/crypto/chacha20 v0.38.0 are
// the independent algorithm/reference provenance. No stream state is shared.
// This portable scalar path intentionally makes NO ISA-acceleration claim.
type headerMaskKey struct {
    words [8]uint32
}

func prepareHeaderMaskKey(key [32]byte) headerMaskKey {
    var h headerMaskKey
    for i := 0; i < len(h.words); i++ {
        h.words[i] = binary.LittleEndian.Uint32(key[i*4 : i*4+4])
    }
    return h
}

// chachaHeaderQuarter preserves all operations/rotations of RFC 8439.
// Four column and four diagonal quarter-rounds form one double round.
func chachaHeaderQuarter(x *[16]uint32, a, b, c, d int) {
    x[a] += x[b]
    x[d] = bits.RotateLeft32(x[d]^x[a], 16)
    x[c] += x[d]
    x[b] = bits.RotateLeft32(x[b]^x[c], 12)
    x[a] += x[b]
    x[d] = bits.RotateLeft32(x[d]^x[a], 8)
    x[c] += x[d]
    x[b] = bits.RotateLeft32(x[b]^x[c], 7)
}

func headerMaskPrepared(h headerMaskKey, ciphertext []byte) ([8]byte, error) {
    if len(ciphertext) < 16 {
        return [8]byte{}, ErrInvalidLength
    }
    // The sample is EXACTLY ciphertext[0:16], with little-endian counter
    // and 96-bit nonce. Nonce/counter must not be retained across records.
    var state = [16]uint32{
        0x61707865, 0x3320646e, 0x79622d32, 0x6b206574,
        h.words[0], h.words[1], h.words[2], h.words[3],
        h.words[4], h.words[5], h.words[6], h.words[7],
        binary.LittleEndian.Uint32(ciphertext[0:4]),
        binary.LittleEndian.Uint32(ciphertext[4:8]),
        binary.LittleEndian.Uint32(ciphertext[8:12]),
        binary.LittleEndian.Uint32(ciphertext[12:16]),
    }
    x := state
    // ChaCha20 means exactly 20 rounds, regardless of an 8-byte result.
    for i := 0; i < 10; i++ {
        chachaHeaderQuarter(&x, 0, 4, 8, 12)
        chachaHeaderQuarter(&x, 1, 5, 9, 13)
        chachaHeaderQuarter(&x, 2, 6, 10, 14)
        chachaHeaderQuarter(&x, 3, 7, 11, 15)
        chachaHeaderQuarter(&x, 0, 5, 10, 15)
        chachaHeaderQuarter(&x, 1, 6, 11, 12)
        chachaHeaderQuarter(&x, 2, 7, 8, 13)
        chachaHeaderQuarter(&x, 3, 4, 9, 14)
    }
    // Only first two words of this exact one-block output are consumed.
    var mask [8]byte
    binary.LittleEndian.PutUint32(mask[0:4], x[0]+state[0])
    binary.LittleEndian.PutUint32(mask[4:8], x[1]+state[1])
    return mask, nil
}

// headerMask retains the original internal API for direct callers and
// golden vectors; production Sealer/Opener cache parsed immutable key words.
func headerMask(key [32]byte, ciphertext []byte) ([8]byte, error) {
    if len(ciphertext) < 16 {
        return [8]byte{}, ErrInvalidLength
    }
    return headerMaskPrepared(prepareHeaderMaskKey(key), ciphertext)
}
