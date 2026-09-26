# M9 same-budget protection admission experiment

Status: **XOR repair admitted only as an experimental candidate; no production FEC wire frame is admitted yet.**

M9 asks one narrow question before WBD spends complexity on FEC: under the same M8 intentional-overhead entitlement, can a minimal repair code reduce logical completion delay compared with the existing M6 gap-driven reinjection and proactive duplication baselines?

## Deterministic model

`internal/experiment/protection` uses discrete steps rather than wall-clock time. Eight equal 1024-byte logical chunks form one batch. Unstalled original TCP-carrier copies arrive at step 1. A selected original copy can be held until step 9, representing TCP HOL/stall rather than permanent packet loss. Proactive duplicate/XOR repair copies use a healthy protection path and arrive at step 1. Once a later logical chunk exposes a gap, reactive reinjection arrives at step 2.

Every strategy receives the same `rbc.Budget` entitlement from the same source bytes. Protection bytes are charged as `duplicate`, `reinjection`, or `FEC`; overspending fails the test. The harness does not model throughput, congestion control, real TCP queues, encryption, REALITY/Vision, or public wire format. Those remain later benchmark/integration work.

## Fixed matrix

For 8192 source bytes, 1.5x grants 4096 protection bytes and 2.0x grants 8192.

| Budget | Fault | Reinjection | Duplicate | Pairwise XOR repair |
| --- | --- | ---: | ---: | ---: |
| 1.5x | single stall, duplicated index | step 2 / 1024 B | step 1 / 4096 B | step 1 / 4096 B |
| 1.5x | single stall, non-duplicated index | step 2 / 1024 B | step 9 / 4096 B | step 1 / 4096 B |
| 1.5x | two stalls in same XOR pair | step 2 / 2048 B | step 9 / 4096 B | step 9 / 4096 B |
| 1.5x | two stalls split across XOR pairs | step 2 / 2048 B | step 9 / 4096 B | step 1 / 4096 B |
| 2.0x | single stall | step 2 / 1024 B | step 1 / 8192 B | step 1 / 4096 B |
| 2.0x | two stalls in same XOR pair | step 2 / 2048 B | step 1 / 8192 B | step 2 / 6144 B |
| 2.0x | two stalls split across XOR pairs | step 2 / 2048 B | step 1 / 8192 B | step 1 / 4096 B |

The test also sweeps all eight possible single-stall positions at 1.5x. Pairwise XOR finishes at step 1 for all eight. GAP-driven reinjection finishes at step 2 for the first seven positions, but a stalled final chunk/FIN has no later offset to reveal a gap and therefore waits for the original TCP copy until step 9. Deterministic 50% duplication protects four positions and leaves four positions waiting until step 9. This avoids basing admission on one favorable index and exposes the current tail-gap blind spot.

## Interpretation

The experiment demonstrates a repeatable FEC benefit, but also a sharp failure mode:

- At 1.5x, one XOR repair chunk per two source chunks protects either member of every pair, giving wider proactive coverage than spending the same bytes on four full duplicates.
- XOR can repair before a gap/reinjection round trip, so isolated stalls and bursts distributed across repair groups complete one step earlier than reactive reinjection in this model. It also protects a stalled final chunk that the current pure GAP-driven M6 recovery cannot detect without a future soft-deadline/tail timer.
- Two missing chunks inside the same two-source XOR group are unrecoverable from one parity symbol. At 1.5x this can be dramatically worse than reinjection because the proactive parity has consumed the entire protection budget.
- At 2.0x, full duplication is the latency ceiling in this simplified model. XOR uses fewer bytes for many cases and can use spare entitlement for reinjection, but does not beat full duplication on completion step.

Therefore M9 does **not** justify a production pairwise-XOR protocol. It justifies carrying an experimental repair strategy into the broader M10 UDP/QUIC/network-profile benchmark. Any future production FEC needs a mixed RBC allocation policy and/or a sliding-window code that avoids the same-pair erasure weakness. Independently of FEC, WBD also needs a bounded soft-deadline/tail recovery trigger because GAP_HINT alone cannot expose a missing final extent.

## Admission decision

**Experimental admit, production defer.**

The next gate should compare real baselines and more realistic delay/loss/reorder schedules before a FEC wire frame is frozen. Until that gate passes, REALITY/Vision, the WBD public frame vocabulary, and platform integrations remain unchanged.
