# M10-002 QUIC oracle qualification and weak-network stress baseline

Status: locally qualified benchmark/oracle checkpoint. This document does not promote an FEC wire format or RBC policy.

## QUIC oracle local qualification

The oracle remains isolated from the main WBD Go 1.23 module:

- implementation: `github.com/quic-go/quic-go`
- tag: `v0.61.0`
- release commit short identity: `579ee19`
- required toolchain: Go 1.25.0
- relay workflow run: `32717572887`
- relay artifact id: `9516504571`
- artifact ZIP SHA-256: `d3353e3f55fc5abad3e4e5e1b1cdf71e7a0ccb181c3368a69cba497208737e49`
- oracle binary SHA-256: `9482defb23b556faf0af0f7f48b8108c1e589973c861969d1d9ee750f3e224a1`
- generated `go.sum` SHA-256: `ae98d0b5699ee49aac22fae53a0eb50a7184dbc64c84b1ff46e3e015390cc6cb`

GitHub Actions was used only because the sandbox could not directly reach the Go toolchain/module download endpoints. The exact returned artifact was SHA-verified and the binary was executed inside the local sandbox. Local execution produced 64 STREAM RTT samples and 64 DATAGRAM RTT samples and negotiated bidirectional QUIC DATAGRAM support.

One local run observed:

| Oracle mode | p50 | p95 | p99 | max |
| --- | ---: | ---: | ---: | ---: |
| QUIC STREAM RTT | 42 us | 127 us | 476 us | 476 us |
| QUIC DATAGRAM RTT | 48 us | 106 us | 144 us | 144 us |

These are loopback qualification numbers, not Internet performance claims.

## Stress model separation

M10-001's small deterministic `Profile` model is intentionally unchanged. M10-002 adds a separate `StressProfile` / `wbd-stress-report/v1` layer so large jitter/loss assumptions cannot silently rewrite the original baseline semantics.

Each stress profile uses 64 x 1024-byte logical chunks and two independent real-TCP-carrier delay/recovery sequences drawn from the same quality family. A protection copy, XOR parity or reinjection must pay the alternate lane's sampled propagation/recovery delay. This fixes the earlier unrealistic micro-model assumption where proactive protection could appear one 10 ms step after time zero even on a 150-300 ms network.

UDP uses the same primary first-transmission loss mask but does not retransmit, so the report records delivery ratio as well as latency of delivered datagrams.

Current versioned families:

- normal: 40-60 ms, 0% loss
- mobile: 80-150 ms, 2% loss
- weak: 150-300 ms, 10% loss, two fixed seeds
- very weak: 150-300 ms, 20% loss, two fixed seeds
- extreme: 250-600 ms, 30% loss, two fixed seeds

This is still a deterministic two-lane model, not packet-level `tc/netem` qualification. The current lanes are independently sampled within the same quality family; correlated radio/bottleneck loss remains a required follow-up.

## Representative p95 results

All WBD rows below preserve 100% logical delivery in the model. UDP p95 is measured only over delivered datagrams and must be read together with delivery ratio.

| Profile | Native TCP p95 | UDP delivery | 1.5x XOR p95 | 2.0x duplicate p95 | 2.0x tail-reinject p95 |
| --- | ---: | ---: | ---: | ---: | ---: |
| normal 40-60 ms | 60 ms | 100% | 60 ms | 60 ms | 60 ms |
| 150-300 ms / 10% seed A | 760 ms | 89.1% | 590 ms | 670 ms | 670 ms |
| 150-300 ms / 10% seed B | 800 ms | 89.1% | 680 ms | 680 ms | 800 ms |
| 150-300 ms / 20% seed A | 1130 ms | 79.7% | 920 ms | 1070 ms | 1130 ms |
| 150-300 ms / 20% seed B | 1140 ms | 79.7% | 1090 ms | 300 ms | 450 ms |
| 250-600 ms / 30% seed A | 2120 ms | 68.8% | 1880 ms | 1880 ms | 2070 ms |
| 250-600 ms / 30% seed B | 2130 ms | 68.8% | 1810 ms | 1810 ms | 1810 ms |

Interpretation:

1. On clean 40-60 ms networks, proactive 1.5x/2.0x protection provides no latency win in this model, so Auto should eventually stay near 1.0x when logical delivery is healthy.
2. At 10% loss, protection can reduce tail latency, but fixed-seed variance is already visible; no single 1.5x strategy dominates every run.
3. At 20% loss, outcome variance becomes large. In one seed, the alternate lane happens to rescue most critical gaps and 2.0x duplication cuts p95 sharply; in another, both carrier sequences suffer critical delays and the gain is modest. This supports logical-tail-driven adaptation rather than a policy based only on average loss percentage.
4. At 30% loss / 250-600 ms, even 2.0x protection only partly reduces tail in these independent-lane samples. The hard 2.0x ceiling remains appropriate; the system should expose a degraded state rather than escalating without bound.
5. Pairwise XOR remains experimental. It can help at some loss shapes, but M9's same-pair double-loss counterexample still applies.

## Qualification commands and receipts

The final local source qualification used the remote M10-001 `model.go` / `model_test.go` copied back from the relay checkout, plus the new stress files. It passed:

```text
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./internal/protocol -run='^$' -fuzz=FuzzUnmarshalFrame -fuzztime=2s
Linux/386 benchmark cross-build
Linux/mipsle benchmark cross-build
Windows/amd64 benchmark cross-build
Linux/mipsle wbd-stress cross-build
Windows/amd64 wbd-stress cross-build
python scripts/verify_handoff.py
python -m unittest discover -s tests -p 'test_*.py' -v
```

Protocol fuzz executed about 143,899 cases in the recorded run. The deterministic `wbd-stress` JSON report SHA-256 was `165ee332177cafdf4f5b506c0fdc91446d3a5c15789bbe34b6119fb83b0eb46e`.

## Next gate

Do not tune RBC or freeze FEC from this deterministic result alone. The next atomic benchmark work should run real localhost two-lane fault proxies with seeded 150-300 ms delay variation and 10% / 20% impairment, then add a correlated-loss/burst profile and a worse-network case. QUIC should also receive fault injection through a controlled packet path so clean loopback qualification is not confused with weak-network QUIC performance.
