package tlsrecord

import (
    "crypto/rand"
    "encoding/binary"
    "errors"
    "sync"
    "testing"

    "golang.org/x/crypto/chacha20"
)

func headerMaskReference(key [32]byte, ciphertext []byte) ([8]byte, error) {
    if len(ciphertext) < 16 {
        return [8]byte{}, ErrInvalidLength
    }
    c, err := chacha20.NewUnauthenticatedCipher(key[:], ciphertext[4:16])
    if err != nil {
        return [8]byte{}, err
    }
    c.SetCounter(binary.LittleEndian.Uint32(ciphertext[:4]))
    var result, zeros [8]byte
    c.XORKeyStream(result[:], zeros[:])
    return result, nil
}

func TestHeaderMaskOneBlockReferenceAndBoundaries(t *testing.T) {
    // Independent old x/crypto stream implementation is the byte oracle.
    var key [32]byte
    var sample [16]byte
    for _, counter := range []uint32{0, 1, 0xffff, 0x10000, 0x7fffffff, 0xfffffffe, 0xffffffff} {
        if _, err := rand.Read(key[:]); err != nil { t.Fatal(err) }
        if _, err := rand.Read(sample[:]); err != nil { t.Fatal(err) }
        binary.LittleEndian.PutUint32(sample[:4], counter)
        prepared := prepareHeaderMaskKey(key)
        for i := 0; i < 96; i++ {
            if _, err := rand.Read(sample[4:]); err != nil { t.Fatal(err) }
            reference, err := headerMaskReference(key, sample[:])
            if err != nil { t.Fatal(err) }
            got, err := headerMaskPrepared(prepared, sample[:])
            if err != nil || got != reference { t.Fatalf("counter=%#x sample=%x got=%x want=%x err=%v", counter, sample, got, reference, err) }
            legacy, err := headerMask(key, sample[:])
            if err != nil || got != legacy { t.Fatal("wrapper and prepared path diverged") }
            // Trailing ciphertext must never change the 16-byte sample.
            extra := append(sample[:], byte(i), byte(i^0xff))
            withSuffix, err := headerMaskPrepared(prepared, extra)
            if err != nil || withSuffix != reference { t.Fatal("sample length unexpectedly changes mask") }
        }
    }
}

func TestHeaderMaskShortAndInputOwnership(t *testing.T) {
    var key [32]byte
    h := prepareHeaderMaskKey(key)
    for n := 0; n < 16; n++ {
        b := make([]byte, n)
        got, err := headerMaskPrepared(h, b)
        if !errors.Is(err, ErrInvalidLength) || got != ([8]byte{}) { t.Fatalf("prepared length %d: %x %v", n, got, err) }
        got, err = headerMask(key, b)
        if !errors.Is(err, ErrInvalidLength) || got != ([8]byte{}) { t.Fatalf("wrapper length %d: %x %v", n, got, err) }
    }
    sample := make([]byte, 16)
    before := append([]byte(nil), sample...)
    if _, err := headerMaskPrepared(h, sample); err != nil { t.Fatal(err) }
    for i := range sample { if sample[i] != before[i] { t.Fatal("mutated borrowed ciphertext") } }
}

func TestHeaderMaskPreparedConcurrentReadOnlyKey(t *testing.T) {
    var key [32]byte
    if _, err := rand.Read(key[:]); err != nil { t.Fatal(err) }
    h := prepareHeaderMaskKey(key)
    var wg sync.WaitGroup
    errorsChan := make(chan string, 32)
    for id := 0; id < 16; id++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for i := 0; i < 128; i++ {
                var sample [16]byte
                binary.LittleEndian.PutUint64(sample[:8], uint64(id)<<32|uint64(i))
                binary.LittleEndian.PutUint64(sample[8:], uint64(i)^0xfedcba9876543210)
                want, err := headerMaskReference(key, sample[:])
                got, gotErr := headerMaskPrepared(h, sample[:])
                if err != nil || gotErr != nil || got != want {
                    errorsChan <- "concurrent read-only key mismatch"
                    return
                }
            }
        }(id)
    }
    wg.Wait()
    close(errorsChan)
    for err := range errorsChan { t.Error(err) }
}

var headerMaskBenchSink [8]byte

func BenchmarkHeaderMaskPrepared(b *testing.B) {
    var key [32]byte
    var sample [16]byte
    for i := range key { key[i] = byte(i*7+1) }
    for i := range sample { sample[i] = byte(i*11+3) }
    h := prepareHeaderMaskKey(key)
    b.ReportAllocs()
    for i:=0;i<b.N;i++ {
        sample[0] = byte(i)
        headerMaskBenchSink, _ = headerMaskPrepared(h, sample[:])
    }
}
func BenchmarkHeaderMaskReference(b *testing.B) {
    var key [32]byte
    var sample [16]byte
    for i := range key { key[i] = byte(i*7+1) }
    for i := range sample { sample[i] = byte(i*11+3) }
    b.ReportAllocs()
    for i:=0;i<b.N;i++ {
        sample[0] = byte(i)
        headerMaskBenchSink, _ = headerMaskReference(key, sample[:])
    }
}
