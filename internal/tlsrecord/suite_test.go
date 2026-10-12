package tlsrecord

import (
    "bytes"
    "crypto/aes"
    "crypto/cipher"
    "encoding/binary"
    "errors"
    "testing"
)

func n3Keys(t *testing.T, suite uint8) KeyPair {
    t.Helper()
    var nonce [16]byte
    for i := range nonce { nonce[i]=byte(i*7+3) }
    master:=bytes.Repeat([]byte{0x6d},32)
    k,err:=DeriveKeysV3(master,nonce,suite)
    if err!=nil { t.Fatal(err) }
    if k.C2S==k.S2C {t.Fatal("directional keys collided")}
    return k
}

// Independent standard-library oracle: we construct a ciphertext record
// directly, without calling the production sealer/mask/nonce methods.
func referenceAESWire(t *testing.T, keys Keys, suite uint8, pn uint64, kind byte, payload []byte) []byte {
    t.Helper()
    size:=16
    if suite==RecordCipherAES256 {size=32}
    block,err:=aes.NewCipher(keys.AEADKey[:size]);if err!=nil{t.Fatal(err)}
    aead,err:=cipher.NewGCM(block);if err!=nil{t.Fatal(err)}
    hp,err:=aes.NewCipher(keys.HPKey[:size]);if err!=nil{t.Fatal(err)}
    plain:=append([]byte{kind},payload...)
    plain=append(plain,InnerType)
    header:=[]byte{OuterType,0x03,0x03,0,0}
    binary.BigEndian.PutUint16(header[3:],uint16(ProtectedPNLen+len(plain)+AEADTagLen))
    aad:=append(append([]byte{},header...),make([]byte,8)...)
    binary.BigEndian.PutUint64(aad[5:],pn)
    nonce:=keys.IV
    var pnBig [8]byte
    binary.BigEndian.PutUint64(pnBig[:],pn)
    for i:=range pnBig {nonce[i+4]^=pnBig[i]}
    ciphertext:=aead.Seal(nil,nonce[:],plain,aad)
    var pad [16]byte
    hp.Encrypt(pad[:],ciphertext[:16])
    for i:=range pnBig {pnBig[i]^=pad[i]}
    wire:=append([]byte{},header...)
    wire=append(wire,pnBig[:]...)
    wire=append(wire,ciphertext...)
    return wire
}

func TestN3AES128AES256ExactWireAndAuth(t *testing.T) {
    for _,suite:=range []uint8{RecordCipherAES128,RecordCipherAES256} {
        keys:=n3Keys(t,suite)
        seal,err:=newSealerAtPNForCipher(keys.C2S,MaxWireLen,suite,7)
        if err!=nil{t.Fatal(err)}
        opener,err:=NewOpenerForCipher(keys.C2S,MaxWireLen,suite)
        if err!=nil{t.Fatal(err)}
        var vectors=[][]byte{nil,[]byte("short"),bytes.Repeat([]byte{0x5a},1024)}
        for i,payload:=range vectors {
            wire,pn,err:=seal.Seal(payload)
            if err!=nil||pn!=uint64(7+i) {t.Fatalf("suite %d PN %d: %v",suite,pn,err)}
            expected:=referenceAESWire(t,keys.C2S,suite,pn,KindLINK,payload)
            if !bytes.Equal(wire,expected){t.Fatalf("suite %d reference ciphertext mismatch PN %d",suite,pn)}
            record,err:=opener.OpenRecord(wire)
            if err!=nil||record.PN!=pn||record.Kind!=KindLINK||!bytes.Equal(record.Payload,payload) {
                t.Fatalf("suite %d roundtrip: %+v %v",suite,record,err)
            }
            for _,index:=range []int{OuterHeaderLen,OuterHeaderLen+ProtectedPNLen,len(wire)-1} {
                modified:=bytes.Clone(wire);modified[index]^=0x40
                if _,err:=opener.OpenRecord(modified);!errors.Is(err,ErrAuthentication) {
                    t.Fatalf("suite %d mutation at %d must fail auth, got %v",suite,index,err)
                }
            }
        }
        health,pn,err:=seal.SealHealthWithPadding([]byte("Q"),8)
        if err!=nil||pn!=10{t.Fatalf("suite %d health: %v",suite,err)}
        decoded,err:=opener.OpenRecord(health)
        if err!=nil||decoded.Kind!=KindHealth||!bytes.Equal(decoded.Payload,[]byte("Q")) {
            t.Fatalf("suite %d health padding decode: %+v %v",suite,decoded,err)
        }
        if len(health)!=FixedWireOverhead+1+8 {t.Fatalf("suite %d wire overhead drift: %d",suite,len(health))}
    }
}

func TestN3AESNoHOLReplayAndOtherSuiteRejected(t *testing.T) {
    for _,suite:=range []uint8{RecordCipherAES128,RecordCipherAES256} {
        keys:=n3Keys(t,suite)
        sealer,err:=NewSealerForCipher(keys.C2S,MaxWireLen,suite);if err!=nil{t.Fatal(err)}
        decoder,err:=NewDecoderForCipher(keys.C2S,MaxWireLen,suite);if err!=nil{t.Fatal(err)}
        first,_,err:=sealer.Seal([]byte("lost early"));if err!=nil{t.Fatal(err)}
        second,_,err:=sealer.Seal([]byte("later independent"));if err!=nil{t.Fatal(err)}
        after:=decoder.OpenPayload(second)
        if len(after)!=1||after[0].Err!=nil||string(after[0].Payload)!="later independent" {
            t.Fatalf("suite %d later delivery is HOL blocked: %+v",suite,after)
        }
        late:=decoder.OpenPayload(first)
        if len(late)!=1||late[0].Err!=nil||string(late[0].Payload)!="lost early" {t.Fatalf("suite %d late record: %+v",suite,late)}
        replay:=decoder.OpenPayload(first)
        if len(replay)!=1||!errors.Is(replay[0].Err,ErrDuplicate){t.Fatalf("suite %d replay: %+v",suite,replay)}
        corrupted:=bytes.Clone(second);corrupted[len(corrupted)-1]^=1
        third,_,err:=sealer.Seal([]byte("independent after corruption"));if err!=nil{t.Fatal(err)}
        concatenated:=append(corrupted,third...)
        delivered:=decoder.OpenPayload(concatenated)
        if len(delivered)!=2||!errors.Is(delivered[0].Err,ErrAuthentication)||delivered[1].Err!=nil {
            t.Fatalf("suite %d failed record blocked later independent record: %+v",suite,delivered)
        }
        wrongSuite:=RecordCipherAES128
        if suite==RecordCipherAES128 {wrongSuite=RecordCipherAES256}
        otherKeys:=n3Keys(t,wrongSuite)
        wrong,err:=NewOpenerForCipher(otherKeys.C2S,MaxWireLen,wrongSuite)
        if err!=nil {t.Fatal(err)}
        if _,err:=wrong.OpenRecord(first);!errors.Is(err,ErrAuthentication) {
            t.Fatalf("suite %d other authenticated suite unexpectedly opened, %v",suite,err)
        }
    }
}

func TestN3SuiteGuardAndLegacyChaChaUnchanged(t *testing.T) {
    keys:=n3Keys(t,RecordCipherChaCha).C2S
    legacy,err:=NewSealer(keys,MaxWireLen);if err!=nil{t.Fatal(err)}
    modern,err:=NewSealerForCipher(keys,MaxWireLen,RecordCipherChaCha);if err!=nil{t.Fatal(err)}
    for _,body:=range [][]byte{[]byte("a"),bytes.Repeat([]byte{0x42},1000)} {
        old,_,err:=legacy.Seal(body);if err!=nil{t.Fatal(err)}
        newWire,_,err:=modern.Seal(body);if err!=nil{t.Fatal(err)}
        if !bytes.Equal(old,newWire){t.Fatal("legacy ChaCha wire changed")}
    }
    for _,suite:=range []uint8{0,4,255}{
        if _,err:=NewSealerForCipher(keys,1500,suite);!errors.Is(err,ErrRecordCipherUnsupported){t.Fatalf("accepted unsupported cipher %d: %v",suite,err)}
        if _,err:=NewOpenerForCipher(keys,1500,suite);!errors.Is(err,ErrRecordCipherUnsupported){t.Fatalf("accepted unsupported opener cipher %d: %v",suite,err)}
    }
    if _,err:=NewSealerForCipher(keys,FixedWireOverhead-1,RecordCipherAES128);!errors.Is(err,ErrInvalidWireLimit){t.Fatal(err)}
}
