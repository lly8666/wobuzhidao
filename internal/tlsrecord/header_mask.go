// Copyright 2016 The Go Authors. All rights reserved.
// Derived from golang.org/x/crypto/chacha20 v0.38.0 (BSD-3-Clause).
// License: https://github.com/golang/crypto/blob/v0.38.0/LICENSE
// The implementation retains RFC 8439's entire 20-round block function.
package tlsrecord

import (
    "encoding/binary"
    "math/bits"
)

// headerMaskKey contains parsed immutable HP key words. Every invocation
// below uses a fresh sample counter and nonce from its own record.
type headerMaskKey struct { words [8]uint32 }

func prepareHeaderMaskKey(key [32]byte) headerMaskKey {
    var h headerMaskKey
    for i:=0;i<8;i++ { h.words[i]=binary.LittleEndian.Uint32(key[4*i:4*i+4]) }
    return h
}

// Reuses the widely reviewed x/crypto scalar quarter-round scheduling,
// with native Go locals instead of indexed pointer-to-array mutations.
// The previous candidate's 8 indexed pointer quarter-round calls were slow
// on both AMD64 and ARM64; this candidate is unqualified until Actions micro.
func headerMaskQuarterRound(a,b,c,d uint32) (uint32,uint32,uint32,uint32) {
    a+=b; d=bits.RotateLeft32(d^a,16)
    c+=d; b=bits.RotateLeft32(b^c,12)
    a+=b; d=bits.RotateLeft32(d^a,8)
    c+=d; b=bits.RotateLeft32(b^c,7)
    return a,b,c,d
}

func headerMaskPrepared(h headerMaskKey, ciphertext []byte) ([8]byte,error) {
    if len(ciphertext)<16 {return [8]byte{},ErrInvalidLength}
    const c0,c1,c2,c3 uint32 = 0x61707865,0x3320646e,0x79622d32,0x6b206574
    x0,x1,x2,x3:=c0,c1,c2,c3
    x4,x5,x6,x7:=h.words[0],h.words[1],h.words[2],h.words[3]
    x8,x9,x10,x11:=h.words[4],h.words[5],h.words[6],h.words[7]
    x12:=binary.LittleEndian.Uint32(ciphertext[0:4])
    x13:=binary.LittleEndian.Uint32(ciphertext[4:8])
    x14:=binary.LittleEndian.Uint32(ciphertext[8:12])
    x15:=binary.LittleEndian.Uint32(ciphertext[12:16])
    for i:=0;i<10;i++ {
        x0,x4,x8,x12=headerMaskQuarterRound(x0,x4,x8,x12)
        x1,x5,x9,x13=headerMaskQuarterRound(x1,x5,x9,x13)
        x2,x6,x10,x14=headerMaskQuarterRound(x2,x6,x10,x14)
        x3,x7,x11,x15=headerMaskQuarterRound(x3,x7,x11,x15)
        x0,x5,x10,x15=headerMaskQuarterRound(x0,x5,x10,x15)
        x1,x6,x11,x12=headerMaskQuarterRound(x1,x6,x11,x12)
        x2,x7,x8,x13=headerMaskQuarterRound(x2,x7,x8,x13)
        x3,x4,x9,x14=headerMaskQuarterRound(x3,x4,x9,x14)
    }
    // Only two output words are required; every round above is required.
    var mask [8]byte
    binary.LittleEndian.PutUint32(mask[:4],x0+c0)
    binary.LittleEndian.PutUint32(mask[4:],x1+c1)
    return mask,nil
}
func headerMask(key [32]byte,ciphertext []byte) ([8]byte,error) {
    if len(ciphertext)<16 {return [8]byte{},ErrInvalidLength}
    return headerMaskPrepared(prepareHeaderMaskKey(key),ciphertext)
}
