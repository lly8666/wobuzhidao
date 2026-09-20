# P4 runtime test port type fixture fix

Date: 2026-09-20  
Candidate source: `12dfe1c15fca20210f47fd3cd557eb8eff6779d3`  
Actions: `35497343943`

## Result

The first unified-runtime candidate did not qualify. Five non-unit jobs passed:

- repository-contract PASS
- P2 kernel fallback / continuous pcap PASS
- Linux shared-TUN privileged iptables PASS
- Linux shared-TUN privileged nft PASS
- OpenWrt privileged TPROXY + socket regression PASS

Both `active-go-tests` matrix entries (Ubuntu 24.04 and Windows 2022) stopped while compiling `internal/runtimeowner/runtime_test.go`:

```
runtime_test.go:112:23: 40000 (untyped int constant) overflows uint8
runtime_test.go:113:23: 440 (untyped int constant) overflows uint8
```

The helper accepted `lane uint8`; therefore Go converted the untyped constants to the non-constant operand type before the outer `uint16(...)` conversion.

## Fix

Fixture only:

```go
clientPort := uint16(40000) + uint16(lane)
serverPort := uint16(440) + uint16(lane)
```

No product runtime code, transport behavior, wire format, lifecycle, platform adapter, or recovery policy changes in this follow-up.

`last_tested_source_sha` intentionally remains the prior qualified product SHA `a8438872f948849ab378238a69dc1a1e28c86ac6` until the corrected unified-runtime candidate passes the full exact-SHA workflow.
