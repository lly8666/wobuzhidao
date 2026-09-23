# 2026-09-23 server follows client dormant commitment

Base SOURCE_SHA before this correction: `ffca1140e4c2a11cd6d8041bcd9d95558624e490`.

## Exact-SHA evidence

At `ffca1140e4c2a11cd6d8041bcd9d95558624e490`:
- `next-lifecycle` run 35798425122 PASS.
- `next-foundation` run 35798425219 PASS.
- `next-p4-steady-targeted` run 35798425186 PASS.
- `next-lifecycle-fullstack` run 35798425167 executed all 36 sample jobs. 32 sample jobs PASS. The only failures are L6 race1/race4, both seeds; aggregate therefore FAIL.
- L5 SYN/TLS/admission/detach, L6 partial and both L7 default 15s/90s jobs all PASS after the previous harness correction.

The remaining L6 failures are not safely dismissible as harness failures. Raw artifact 10724668351 (race1/seed101) shows the sender wrote 100/100 datagrams with zero send errors, while the target received 96/100 and missed seq 7,46,72,79. Diagnostic alignment shows that for seq46/72/79 the server had entered DORMANT roughly 25ms **before** the client-originated business send, while the client remained ACTIVE. Seq7 is the same boundary with server dormancy about 225ms after the send. The server therefore can close from an older periodic idle hint before the client has committed to sleeping; because the server has no out-of-band reverse wake path, that unilateral close can swallow near-cutoff business.

## Product correction

The idle health duration remains useful evidence, but it is not a promise that no new business will occur after the hint. The client is the endpoint that can actively re-open lanes, so tunnel dormancy ownership is made asymmetric without changing the wire format:

- add `Runtime.PeerWriteClosed()`, true only when every **current authoritative** lane has consumed an in-order peer FIN;
- server auto-idle still requires its own business-idle threshold, but now follows `PeerWriteClosed()` rather than periodic `PeerIdle()`;
- retiring generations do not satisfy the commitment;
- client auto-idle keeps its existing local activity snapshot + authenticated peer-idle evidence, sends the final idle hint, then steady FINs. If a FIN is lost, the server may conservatively remain active; it must not race ahead and drop business.
- add a unit test proving one of two authoritative FINs is insufficient and all current authoritative FINs are required.

No admission/wire record change, parameter/default change, FEC change, 4096/buffer change, repair/ACK/HOL change, or AF_PACKET capacity change is made.

## L6 acceptance correction

The previous wake driver also did not actually implement the written “wake while network is still blackholed” clause and incorrectly assumed roughly one wake per packet. The new real-process L6 sequence is:

1. verify both endpoints are DORMANT;
2. blackhole all underlay tuples and inject 16 client-originated business events;
3. require at least one failed wake, bounded attempts (no tick storm), then clear the blackhole;
4. require a post-clear recovery probe to deliver;
5. wait for a fresh both-DORMANT precondition;
6. send 100 mixed-size client-originated events at 1.25s intervals on a clear path and require 100/100 unique delivery;
7. require repeated successful Dormant→Wake cycles, no clear-path wake failure, bounded resources and no deadlock.

This separates unavoidable payload loss during the intentional blackhole from the correctness requirement that clear-path near-cutoff business must not be swallowed by unilateral server dormancy.

## Qualification boundary

No PASS is inherited. The new exact SHA must rerun core/race/foundation and the full 36-sample aggregate. The strict 10 Mbps Normal / 3 Mbps Game target-rate evidence remains separately `CAPACITY_LIMITED` at `0882adb...`; the AF_PACKET/uplink-capacity mainline remains HOLD.
