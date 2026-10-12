package tlsrecord

import (
    "crypto/aes"
    "crypto/cipher"
    "errors"

    "golang.org/x/crypto/chacha20poly1305"
)

// N3 record suite identifiers match the authenticated V3 admission policy.
// Legacy constructors remain ChaCha-only; callers must opt into this API
// explicitly after binding the V3 policy to an authenticated lane.
const (
    RecordCipherChaCha uint8 = 1
    RecordCipherAES128 uint8 = 2
    RecordCipherAES256 uint8 = 3
)

var ErrRecordCipherUnsupported = errors.New("tlsrecord: unsupported record cipher")

// cipherForSuite initializes AEAD and independent HP block ONCE per lane.
// AES-128 consumes the first 16 bytes of the suite-domain-separated V3
// AEADKey and HPKey; AES-256 uses 32 bytes of each. The outer record remains
// 5-byte header + 8-byte protected PN + 16-byte tag + kind/inner marker.
func cipherForSuite(keys Keys, suite uint8) (cipher.AEAD, cipher.Block, error) {
    if suite == RecordCipherChaCha {
        aead, err := chacha20poly1305.New(keys.AEADKey[:])
        return aead, nil, err
    }
    keyLen := 0
    switch suite {
    case RecordCipherAES128:
        keyLen = 16
    case RecordCipherAES256:
        keyLen = 32
    default:
        return nil, nil, ErrRecordCipherUnsupported
    }
    payloadBlock, err := aes.NewCipher(keys.AEADKey[:keyLen])
    if err != nil { return nil, nil, err }
    aead, err := cipher.NewGCM(payloadBlock)
    if err != nil { return nil, nil, err }
    hpBlock, err := aes.NewCipher(keys.HPKey[:keyLen])
    if err != nil { return nil, nil, err }
    return aead, hpBlock, nil
}

// headerMaskForSuite samples exactly the first 16 bytes of AES-GCM
// ciphertext (including the tag for tiny records) and AES-encrypts it once.
// This is WBD's protected 8-byte PN, NOT QUIC header protection.
func headerMaskForSuite(hpKey [32]byte, hpBlock cipher.Block, ciphertext []byte) ([8]byte, error) {
    if len(ciphertext) < 16 { return [8]byte{}, ErrInvalidLength }
    if hpBlock == nil { return headerMask(hpKey, ciphertext) }
    var encrypted [16]byte
    hpBlock.Encrypt(encrypted[:], ciphertext[:16])
    var mask [8]byte
    copy(mask[:], encrypted[:8])
    return mask, nil
}

func newSealerAtPNForCipher(keys Keys, maxWire int, suite uint8, nextPN uint64) (*Sealer, error) {
    maxBody, err := effectiveBodyLimit(maxWire)
    if err != nil { return nil, err }
    aead, hpBlock, err := cipherForSuite(keys, suite)
    if err != nil { return nil, err }
    return &Sealer{keys:keys, aead:aead, hpBlock:hpBlock, maxBody:maxBody, nextPN:nextPN},nil
}

func NewSealerForCipher(keys Keys, maxWire int, suite uint8) (*Sealer, error) {
    return newSealerAtPNForCipher(keys, maxWire, suite, 0)
}

func NewOpenerForCipher(keys Keys, maxWire int, suite uint8) (*Opener, error) {
    maxBody, err := effectiveBodyLimit(maxWire)
    if err != nil { return nil, err }
    aead, hpBlock, err := cipherForSuite(keys, suite)
    if err != nil { return nil, err }
    return &Opener{keys:keys, aead:aead, hpBlock:hpBlock, maxBody:maxBody},nil
}

func NewDecoderForCipher(keys Keys, maxWire int, suite uint8) (*Decoder, error) {
    opener, err := NewOpenerForCipher(keys, maxWire, suite)
    if err != nil { return nil, err }
    return &Decoder{opener:opener, recent:newRecentPNSet()},nil
}
