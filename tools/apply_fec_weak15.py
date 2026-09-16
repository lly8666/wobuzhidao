from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text(encoding='utf-8')
    n = text.count(old)
    if n != 1:
        raise SystemExit(f'{path}: expected 1 occurrence, got {n}: {old[:100]!r}')
    p.write_text(text.replace(old, new), encoding='utf-8')

# Wire header can now describe the historical 20:10 product profile while the
# internal RS matrix remains the proven 20x20 superset. Only the negotiated
# first 10 repair rows are transmitted/accepted for weak-1.5x.
replace_once('internal/fec/fec.go', '''\tDataShards   = 20\n\tParityShards = 20\n\tTotalShards  = DataShards + ParityShards''', '''\tDataShards        = 20\n\tWeakParityShards  = 10\n\tParityShards      = 20\n\tTotalShards       = DataShards + ParityShards''')
replace_once('internal/fec/fec.go', '''type BlockHeader struct {\n\tBlockID         uint32\n\tShardIndex      uint8\n\tDataCount       uint8\n\tShardSize       uint16\n\tOriginalLengths [DataShards]uint16\n}\n''', '''type BlockHeader struct {\n\tBlockID         uint32\n\tShardIndex      uint8\n\tDataCount       uint8\n\tShardSize       uint16\n\t// ParityCount is the immutable negotiated repair-shard geometry. Zero is\n\t// retained as a source/API compatibility alias for the historical 20:20\n\t// default; parsed wire headers always carry an explicit value.\n\tParityCount     uint8\n\tOriginalLengths [DataShards]uint16\n}\n\nfunc validParityCount(n int) bool { return n == WeakParityShards || n == ParityShards }\nfunc (h BlockHeader) EffectiveParityCount() int {\n\tif h.ParityCount == 0 { return ParityShards }\n\treturn int(h.ParityCount)\n}\n''')
replace_once('internal/fec/fec.go', '''\tb[9] = DataShards\n\tb[10] = ParityShards\n\tb[11] = h.DataCount''', '''\tb[9] = DataShards\n\tb[10] = byte(h.EffectiveParityCount())\n\tb[11] = h.DataCount''')
replace_once('internal/fec/fec.go', '''\tif b[9] != DataShards || b[10] != ParityShards {\n\t\treturn h, errors.New("fec: incompatible shard geometry")\n\t}\n\th.BlockID = binary.BigEndian.Uint32(b[4:8])''', '''\tif b[9] != DataShards || !validParityCount(int(b[10])) {\n\t\treturn h, errors.New("fec: incompatible shard geometry")\n\t}\n\th.BlockID = binary.BigEndian.Uint32(b[4:8])\n\th.ParityCount = b[10]''')
replace_once('internal/fec/fec.go', '''func (h BlockHeader) Validate() error {\n\tif h.ShardIndex >= TotalShards {\n\t\treturn fmt.Errorf("fec: shard index %d out of range", h.ShardIndex)\n\t}''', '''func (h BlockHeader) Validate() error {\n\tparity := h.EffectiveParityCount()\n\tif !validParityCount(parity) {\n\t\treturn fmt.Errorf("fec: parity count %d unsupported", parity)\n\t}\n\tif int(h.ShardIndex) >= DataShards+parity {\n\t\treturn fmt.Errorf("fec: shard index %d out of range for 20:%d", h.ShardIndex, parity)\n\t}''')

# Fast encoder emits only negotiated repair rows. The 20x20 codec remains the
# matrix implementation, which is mathematically a superset of the first 10 rows.
replace_once('internal/fec/fastblock_encoder.go', '''\tcodec         Codec\n\tmaxPacketSize int''', '''\tcodec         Codec\n\tparityShards  int\n\tmaxPacketSize int''')
replace_once('internal/fec/fastblock_encoder.go', '''func NewFastBlockEncoder(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32) (*FastBlockEncoder, error) {\n\tif codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || flushAfter <= 0 {\n\t\treturn nil, errors.New("fec: invalid fast block encoder config")\n\t}\n\te := &FastBlockEncoder{\n\t\tcodec: codec, maxPacketSize: maxPacketSize, flushAfter: flushAfter, nextBlockID: firstBlockID,\n\t}''', '''func NewFastBlockEncoder(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32) (*FastBlockEncoder, error) {\n\treturn NewFastBlockEncoderWithParity(codec, maxPacketSize, flushAfter, firstBlockID, ParityShards)\n}\n\nfunc NewFastBlockEncoderWithParity(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32, parityShards int) (*FastBlockEncoder, error) {\n\tif codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || flushAfter <= 0 || !validParityCount(parityShards) {\n\t\treturn nil, errors.New("fec: invalid fast block encoder config")\n\t}\n\te := &FastBlockEncoder{\n\t\tcodec: codec, parityShards: parityShards, maxPacketSize: maxPacketSize, flushAfter: flushAfter, nextBlockID: firstBlockID,\n\t}''')
replace_once('internal/fec/fastblock_encoder.go', 'marshalStreamingSourceHeader(wire[:HeaderSize], e.nextBlockID, idx, len(packet))', 'marshalStreamingSourceHeader(wire[:HeaderSize], e.nextBlockID, idx, len(packet), e.parityShards)')
replace_once('internal/fec/fastblock_encoder.go', '''\tparityCount := dataCount\n\tif parityCount > ParityShards {\n\t\tparityCount = ParityShards\n\t}''', '''\tparityCount := dataCount\n\tif parityCount > e.parityShards {\n\t\tparityCount = e.parityShards\n\t}''')
replace_once('internal/fec/fastblock_encoder.go', 'marshalFastHeader(b[:HeaderSize], e.nextBlockID, index, dataCount, shardSize, e.lengths)', 'marshalFastHeader(b[:HeaderSize], e.nextBlockID, index, dataCount, shardSize, e.parityShards, e.lengths)')
replace_once('internal/fec/fastblock_encoder.go', '''func marshalStreamingSourceHeader(dst []byte, blockID uint32, shardIndex, packetLen int) {''', '''func marshalStreamingSourceHeader(dst []byte, blockID uint32, shardIndex, packetLen, parityShards int) {''')
replace_once('internal/fec/fastblock_encoder.go', '''\tdst[9] = DataShards\n\tdst[10] = ParityShards''', '''\tdst[9] = DataShards\n\tdst[10] = byte(parityShards)''')
replace_once('internal/fec/fastblock_encoder.go', '''func marshalFastHeader(dst []byte, blockID uint32, shardIndex, dataCount, shardSize int, lengths [DataShards]uint16) {''', '''func marshalFastHeader(dst []byte, blockID uint32, shardIndex, dataCount, shardSize, parityShards int, lengths [DataShards]uint16) {''')
# second geometry occurrence belongs to final header.
p = Path('internal/fec/fastblock_encoder.go')
text = p.read_text(encoding='utf-8')
old = '\tdst[9] = DataShards\n\tdst[10] = ParityShards'
if text.count(old) != 1: raise SystemExit(f'final fast header geometry count={text.count(old)}')
p.write_text(text.replace(old, '\tdst[9] = DataShards\n\tdst[10] = byte(parityShards)'), encoding='utf-8')

# Decoder pins the negotiated geometry, but keeps 40 internal slots so the
# proven 20x20 matrix can reconstruct from the transmitted first 10 repair rows.
replace_once('internal/fec/block.go', '''type BlockDecoder struct {\n\tcodec         Codec\n\tmaxPacketSize int''', '''type BlockDecoder struct {\n\tcodec         Codec\n\tparityShards  int\n\tmaxPacketSize int''')
replace_once('internal/fec/block.go', '''func NewBlockDecoder(codec Codec, maxPacketSize, maxBlocks int) (*BlockDecoder, error) {\n\tif codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || maxBlocks <= 0 {\n\t\treturn nil, errors.New("fec: invalid block decoder config")\n\t}\n\treturn &BlockDecoder{\n\t\tcodec: codec, maxPacketSize: maxPacketSize, maxBlocks: maxBlocks,\n\t\tblocks: make(map[uint32]*decodeBlock), retired: make(map[uint32]retiredBlock),\n\t}, nil\n}''', '''func NewBlockDecoder(codec Codec, maxPacketSize, maxBlocks int) (*BlockDecoder, error) {\n\treturn NewBlockDecoderWithParity(codec, maxPacketSize, maxBlocks, ParityShards)\n}\n\nfunc NewBlockDecoderWithParity(codec Codec, maxPacketSize, maxBlocks, parityShards int) (*BlockDecoder, error) {\n\tif codec == nil || maxPacketSize <= 0 || maxPacketSize > 0xffff || maxBlocks <= 0 || !validParityCount(parityShards) {\n\t\treturn nil, errors.New("fec: invalid block decoder config")\n\t}\n\treturn &BlockDecoder{\n\t\tcodec: codec, parityShards: parityShards, maxPacketSize: maxPacketSize, maxBlocks: maxBlocks,\n\t\tblocks: make(map[uint32]*decodeBlock), retired: make(map[uint32]retiredBlock),\n\t}, nil\n}''')
replace_once('internal/fec/block.go', '''\th, err := ParseBlockHeader(datagram[:HeaderSize])\n\tif err != nil {\n\t\treturn nil, false, err\n\t}''', '''\th, err := ParseBlockHeader(datagram[:HeaderSize])\n\tif err != nil {\n\t\treturn nil, false, err\n\t}\n\tif h.EffectiveParityCount() != d.parityShards {\n\t\treturn nil, false, ErrHeaderMismatch\n\t}''')
replace_once('internal/fec/block.go', '''return a.BlockID == b.BlockID && a.DataCount == b.DataCount && a.ShardSize == b.ShardSize && a.OriginalLengths == b.OriginalLengths''', '''return a.BlockID == b.BlockID && a.DataCount == b.DataCount && a.ShardSize == b.ShardSize && a.EffectiveParityCount() == b.EffectiveParityCount() && a.OriginalLengths == b.OriginalLengths''')

# Immutable LINK admission + live path.
replace_once('internal/control/link.go', '''AllowedFixedFEC: []FixedFECProfile{{DataShards: 20, ParityShards: 20, Scheduler: FECSchedulerTailRS}},''', '''AllowedFixedFEC: []FixedFECProfile{\n\t\t\t{DataShards: 20, ParityShards: 10, Scheduler: FECSchedulerTailRS},\n\t\t\t{DataShards: 20, ParityShards: 20, Scheduler: FECSchedulerTailRS},\n\t\t},''')
replace_once('internal/linkdata/path.go', '''\tif config.FECMode != control.FECFixed ||\n\t\tconfig.Scheduler != control.FECSchedulerTailRS ||\n\t\tconfig.DataShards != fec.DataShards || config.ParityShards != fec.ParityShards {\n\t\treturn nil, ErrUnsupportedLinkConfig\n\t}\n\tcodec := fec.NewFastReedSolomon20x20()\n\tenc, err := fec.NewFastBlockEncoder(codec, int(config.MTU), time.Duration(config.FlushMillis)*time.Millisecond, 1)''', '''\tif config.FECMode != control.FECFixed ||\n\t\tconfig.Scheduler != control.FECSchedulerTailRS ||\n\t\tconfig.DataShards != fec.DataShards ||\n\t\t(config.ParityShards != fec.WeakParityShards && config.ParityShards != fec.ParityShards) {\n\t\treturn nil, ErrUnsupportedLinkConfig\n\t}\n\tcodec := fec.NewFastReedSolomon20x20()\n\tenc, err := fec.NewFastBlockEncoderWithParity(codec, int(config.MTU), time.Duration(config.FlushMillis)*time.Millisecond, 1, int(config.ParityShards))''')
replace_once('internal/linkdata/path.go', '''\tdec, err := fec.NewBlockDecoder(installFECObserver(p, codec), int(config.MTU), maxBlocks)''', '''\tdec, err := fec.NewBlockDecoderWithParity(installFECObserver(p, codec), int(config.MTU), maxBlocks, int(config.ParityShards))''')
replace_once('cmd/wbd-link-proxy/main.go', '''\tcase "20:10", "weak-1.5x":\n\t\treturn control.LinkConfig{}, errors.New("20:10 is not implemented by the live WBD codec")''', '''\tcase "20:10", "weak-1.5x":\n\t\tif o.flushMS <= 0 {\n\t\t\treturn control.LinkConfig{}, errors.New("20:10 requires positive -fec-flush-ms")\n\t\t}\n\t\tcfg.FECMode = control.FECFixed\n\t\tcfg.Scheduler = control.FECSchedulerTailRS\n\t\tcfg.DataShards = 20\n\t\tcfg.ParityShards = 10\n\t\tcfg.FlushMillis = uint16(o.flushMS)''')
replace_once('cmd/wbd-link-proxy/main.go', '"immutable 20:20 partial-block flush"', '"immutable fixed-FEC partial-block flush"')

# Windows profile/MTU admission.
replace_once('internal/windowsruntime/plan.go', '''\tif p.FEC != "off" && p.FEC != "20:20" {\n\t\treturn errors.New("FEC must be off or 20:20")\n\t}''', '''\tif p.FEC != "off" && p.FEC != "20:10" && p.FEC != "20:20" {\n\t\treturn errors.New("FEC must be off, 20:10, or 20:20")\n\t}''')
replace_once('internal/windowsruntime/game_mtu.go', '''\tcase "20:20":\n\t\tfecEnabled = true''', '''\tcase "20:10", "20:20":\n\t\tfecEnabled = true''')

# Product regression tests: wire geometry, 10-loss recovery, immutable LINK and
# real proxy integration all exercise weak-1.5x instead of only parsing text.
weak_test = r'''package fec

import (
    "bytes"
    "testing"
    "time"
)

func TestWeak15FastCodecRecoversTenLostSystematicShards(t *testing.T) {
    codec := NewFastReedSolomon20x20()
    enc, err := NewFastBlockEncoderWithParity(codec, 1400, 8*time.Millisecond, 7, WeakParityShards)
    if err != nil { t.Fatal(err) }
    var wire [][]byte
    want := make([][]byte, DataShards)
    now := time.Unix(1, 0)
    for i := 0; i < DataShards; i++ {
        want[i] = []byte{byte(i+1), byte(200-i), byte(i^0x5a)}
        out, err := enc.Add(want[i], now); if err != nil { t.Fatal(err) }
        for _, b := range out { wire = append(wire, append([]byte(nil), b...)) }
    }
    if len(wire) != DataShards+WeakParityShards { t.Fatalf("wire shards=%d want=%d", len(wire), DataShards+WeakParityShards) }
    for _, b := range wire {
        h, err := ParseBlockHeader(b[:HeaderSize]); if err != nil { t.Fatal(err) }
        if h.EffectiveParityCount() != WeakParityShards { t.Fatalf("parity=%d", h.EffectiveParityCount()) }
    }
    dec, err := NewBlockDecoderWithParity(NewReedSolomon20x20(), 1400, 8, WeakParityShards)
    if err != nil { t.Fatal(err) }
    got := make(map[byte][]byte)
    // Drop systematic 0..9. Surviving 10 sources plus 10 repair shards are
    // exactly the 20 equations needed to recover all missing originals.
    for _, b := range wire[10:] {
        packets, _, err := dec.Add(b); if err != nil { t.Fatal(err) }
        for _, p := range packets { got[p[0]] = append([]byte(nil), p...) }
    }
    if len(got) != DataShards { t.Fatalf("recovered=%d want=%d", len(got), DataShards) }
    for _, p := range want { if !bytes.Equal(got[p[0]], p) { t.Fatalf("packet %d mismatch", p[0]) } }
}

func TestDecoderRejectsNegotiatedParityMismatch(t *testing.T) {
    enc, _ := NewFastBlockEncoderWithParity(NewReedSolomon20x20(), 1400, time.Millisecond, 1, WeakParityShards)
    out, err := enc.Add([]byte("x"), time.Now()); if err != nil { t.Fatal(err) }
    dec, _ := NewBlockDecoder(NewReedSolomon20x20(), 1400, 2)
    if _, _, err := dec.Add(out[0]); err != ErrHeaderMismatch { t.Fatalf("err=%v want ErrHeaderMismatch", err) }
}
'''
Path('internal/fec/weak15_test.go').write_text(weak_test, encoding='utf-8')

linkdata_test = r'''package linkdata

import (
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/control"
)

func TestWeak15ImmutablePathIsAdmittedAndEmitsTenRepairShards(t *testing.T) {
    cfg := control.LinkConfig{FECMode:control.FECFixed, Scheduler:control.FECSchedulerTailRS, DataShards:20, ParityShards:10, LaneCount:1, FlushMillis:8, MTU:1400}
    p, err := New(cfg, 8); if err != nil { t.Fatal(err) }
    var repair uint64
    now := time.Unix(2,0)
    for i:=0; i<20; i++ { if _, err := p.Encode([]byte{byte(i+1)}, now); err != nil { t.Fatal(err) } }
    repair = p.Stats().FECRepairTXPackets
    if repair != 10 { t.Fatalf("repair=%d want=10", repair) }
}
'''
Path('internal/linkdata/weak15_test.go').write_text(linkdata_test, encoding='utf-8')

p = Path('cmd/wbd-link-proxy/main_test.go')
text = p.read_text(encoding='utf-8')
needle = '''func TestImmutableLinkProxyFixed20x20WithAuth(t *testing.T) {\n\trunProxyIntegration(t, "20:20", true, false)\n}\n'''
insert = needle + '''\nfunc TestImmutableLinkProxyFixed20x10WithAuth(t *testing.T) {\n\trunProxyIntegration(t, "20:10", true, false)\n}\n'''
if text.count(needle) != 1: raise SystemExit('link proxy test insertion point changed')
p.write_text(text.replace(needle, insert), encoding='utf-8')

print('live FEC weak-1.5x patch applied')
