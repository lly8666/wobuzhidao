from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    s = p.read_text(encoding='utf-8')
    if s.count(old) != 1:
        raise SystemExit(f'{path}: expected one occurrence of {old[:80]!r}, got {s.count(old)}')
    p.write_text(s.replace(old, new, 1), encoding='utf-8')

# Preserve source/API compatibility: zero remains the in-memory alias for the
# historical default 20:20, even though the wire always carries an explicit 20.
replace_once('internal/fec/fec.go', '\th.ParityCount = b[10]\n', '\tif b[10] != ParityShards {\n\t\th.ParityCount = b[10]\n\t}\n')

# The product contract now has exactly two live fixed profiles: 20:10 and 20:20.
replace_once('internal/control/link_test.go', '''\tbad := fixed20x20Link()\n\tbad.ParityShards = 10\n\tif err := p.Validate(bad); !errors.Is(err, ErrUnsupported) {\n\t\tt.Fatalf("20:10 should remain unsupported by live WBD codec, err=%v", err)\n\t}\n\tbad = fixed20x20Link()''', '''\tweak := fixed20x20Link()\n\tweak.ParityShards = 10\n\tif err := p.Validate(weak); err != nil {\n\t\tt.Fatalf("20:10 historical product profile must be admitted, err=%v", err)\n\t}\n\tbad := fixed20x20Link()\n\tbad.ParityShards = 9\n\tif err := p.Validate(bad); !errors.Is(err, ErrUnsupported) {\n\t\tt.Fatalf("20:9 non-product profile must remain unsupported, err=%v", err)\n\t}\n\tbad = fixed20x20Link()''')
replace_once('internal/control/link_test.go', '''\tbad := fixed20x20Link()\n\tbad.ParityShards = 10\n\twire, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: bad})''', '''\tbad := fixed20x20Link()\n\tbad.ParityShards = 9\n\twire, _ := MarshalLink(LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: bad})''')
replace_once('internal/control/link_test.go', '''\t\tLinkAccept{Protocol: 1, AuthRequired: false, Config: fixed20x20Link()},\n''', '''\t\tLinkAccept{Protocol: 1, AuthRequired: false, Config: fixed20x20Link()},\n\t\tLinkInit{MinProtocol: 1, MaxProtocol: 1, Config: LinkConfig{FECMode: FECFixed, Scheduler: FECSchedulerTailRS, DataShards: 20, ParityShards: 10, LaneCount: 1, FlushMillis: 8, MTU: 1400}},\n''')
print('FEC weak-1.5x compatibility contract aligned')
