# ADR-0007: Low-frequency fixed-FEC refresh from low-load FakeTCP loss samples

Status: **ACCEPTED FOR DEVELOPMENT** (2026-08-25)

## Context

WBD associations deliberately keep data-plane parameters immutable after establishment. That makes the hot path simple and reproducible, but the access network can change materially over a day. A profile selected in the morning may be unnecessarily expensive or too weak in an evening congestion period.

The product does not need a continuously learning Auto-FEC controller. A much narrower requirement is sufficient:

- roughly every 30 or 60 minutes;
- wait for a low-load period;
- observe about 20 seconds of the existing TCP-shaped carrier;
- estimate the underlying first-transmission loss rate;
- choose one of a small fixed FEC preset table;
- apply a changed preset only by establishing a fresh association.

This ADR does **not** revive runtime config epochs or mid-association FEC switching.

## Decision 1 — measure unique first-loss marks, not retransmission attempts

The FakeTCP sender already knows when a TCP-like segment requires fast or RTO retransmission. Counting total retransmission attempts is not a loss probability because one missing segment may be retransmitted several times.

`SenderStats` therefore records:

- `Enqueued`: original carrier data segments;
- `EnqueuedBytes`: original carrier payload bytes;
- `LossMarked`: original segments that required at least one retransmission, counted exactly once;
- `LossMarkedBytes`: corresponding original bytes;
- existing fast/RTO attempt and retransmit-byte counters.

For a measurement window:

```text
p_hat = delta(LossMarked) / delta(Enqueued)
```

This is a sender-side estimate of the direction the sender transmits. It needs no new acknowledgement format and no continuous measurement traffic.

SACK/reordering/spurious retransmission can still bias the estimate. Therefore profile choice uses a conservative Wilson 95% upper bound rather than treating `p_hat` as exact truth.

## Decision 2 — sample only under low load

A saturated path can create self-inflicted queue loss. That is not the quantity used to choose the baseline FEC profile.

The default candidate measurement gate is:

```text
window = 20 s
outer original bitrate <= 5% of configured physical-path capacity
```

The refresh scheduler may wait for a low-load interval. If no suitable interval appears before a configured deadline, it skips that refresh rather than changing profile from a contaminated sample.

The first implementation assumes physical uplink/downlink capacity is configured by the user/operator. Automatic capacity estimation is a separate advanced feature.

## Decision 3 — no persistent probe overhead

Organic traffic is consumed first. Merely taking sender counter snapshots sends zero bytes.

A statistical limitation remains: an idle path with no packets cannot reveal its loss probability. If the user enables idle probing and the 20-second organic sample contains too few segments, WBD may fill only the sample deficit with small authenticated diagnostic datagrams. Probe generation stops as soon as the sample target is reached.

A representative maximum target is 1024 segments. At roughly 128 bytes of total per-probe wire budget over 20 seconds, filling all 1024 samples from scratch is about 52.4 kbit/s. This is a short diagnostic window once per 30/60 minutes, not steady overhead. A strict `organic-only` option simply postpones a decision when samples are insufficient.

## Decision 4 — coarse fixed preset table

The current selector is deliberately a table, not a learned controller. With `K=20`, the planning work previously found that strong iid block-failure targets need roughly `R=4/8/12/16/20` around `1/5/10/15/20%` raw loss.

The selector uses the Wilson 95% upper bound `p_hi`:

| `p_hi` | desired profile |
| ---: | --- |
| <= 0.5% | off |
| <= 2% | 20:4 |
| <= 5% | 20:8 |
| <= 10% | 20:12 |
| <= 15% | 20:16 |
| > 15% | 20:20 |

If `p_hi > 20%`, `20:20` is only the strongest admitted single-lane preset, not a guarantee. The system records the over-range condition for diagnostics.

At the time of this ADR, the live codec still admits only `off` and `20:20`; `20:4/8/12/16` are the next fixed-code implementation target. Until those codecs are live-qualified, the runtime must not pretend they exist.

## Decision 5 — FEC profile also defines an inner-rate ceiling

FEC and TCP-like shadow retransmission consume physical capacity. To preserve low first-arrival latency, the client must not offer inner traffic at a rate that drives the outer path into persistent queues.

Let:

- `C` = configured physical path capacity;
- `u` = target maximum path utilization (for example 0.8);
- `a` = ACK/control reserve fraction;
- `P` = representative inner payload bytes;
- `H` = measured/estimated outer per-datagram overhead;
- `F = (K+R)/K` for fixed FEC, or 1 when off;
- `A` = shadow-ARQ byte multiplier.

The deterministic inner payload ceiling is approximated by:

```text
B_inner_max = C * u * (1-a) / (F * ((P+H)/P) * A)
```

For safety, `A` is the larger of:

```text
measured_ARQ_factor = 1 + retransmit_bytes/original_bytes
confidence_ARQ      = 1/(1-p_hi)
```

Example ignoring headers/ACK reserve: 200 Mbit/s path, 20% loss upper bound, 20:20 FEC and 80% target utilization gives:

```text
200 * 0.8 / (2 * 1.25) = 64 Mbit/s inner payload ceiling.
```

This rate limiter is not an attempt to recreate TCP congestion control. It is a configured-capacity guard that prevents known FEC/retransmission expansion from saturating the access link. New inner datagrams remain datagram-like and are not gated by the shadow TCP receive sequence.

## Decision 6 — associations stay immutable; adaptation is association rotation

There is no runtime FEC-change frame.

Each endpoint maintains a last good low-load sample for the direction it sends:

- client sender statistics estimate uplink loss;
- server sender statistics estimate downlink loss.

A client-side refresh timer (default configurable 30 or 60 minutes) creates a fresh association when a refresh is due. The new establishment is make-before-break when resources permit: the old association remains usable until the new FakeTCP/DTLS/WBD association reaches Established, then packet ownership switches and the old association drains/closes.

To support asymmetric paths without ongoing control traffic, the establishment protocol will evolve from one symmetric FEC config to two immutable directional transmit configs:

```text
LINK_INIT:
  shared MTU/lane/protocol parameters
  client_tx_fec

LINK_ACCEPT:
  exact accepted shared/client_tx parameters
  server_tx_fec selected from server's last local low-load sample
  auth_required
```

The server does not periodically push FEC updates. It simply applies its locally selected downlink preset to the **next** association. Thus ordinary operation has no new periodic control frames.

If both selected profiles are unchanged, implementations may keep the current association and defer rotation; the first simple implementation may also rotate once per configured interval because one handshake per hour is negligible relative to steady data traffic. This is a product tuning choice, not a wire-semantic requirement.

## Decision 7 — distinguish this from advanced Auto FEC

This feature is called **periodic fixed-profile refresh**.

It has no online optimizer, no loss/capacity joint estimator, no continuous hysteresis loop, no scheduler learning and no mid-session codec transition. It is a coarse periodic classifier over a small fixed table.

The previously deferred advanced Auto-FEC milestone remains deferred. It may later replace the fixed table or infer capacity automatically, but it is not required for this feature.

## Implementation sequence

1. Expose unique first-loss and byte counters from FakeTCP. **Started in `internal/faketcp`.**
2. Add pure low-load window, Wilson-bound, fixed-profile and inner-rate math in `internal/linkadapt`. **Started.**
3. Add deterministic tests and a small CLI/receipt tool for snapshots/profile/rate decisions.
4. Generalize the live systematic RS path to admitted `20:4/8/12/16/20` fixed presets without delaying source shards.
5. Split establishment FEC into immutable client-TX and server-TX profiles.
6. Add the 30/60-minute low-load scheduler and make-before-break association rotation.
7. Run real low-load and loaded netem matrices to verify the selector does not confuse self-induced queue loss with path loss.

## TLS/network-treatment diagnostic boundary

A separate diagnostic mode may connect **genuinely** to a public control service (for example a speed-test service) and record its real TLS certificate/SPKI fingerprint, handshake timing and throughput as a network-treatment baseline. WBD may then compare that with an operator-authorized WBD endpoint using a pinned browser-like ClientHello profile.

A third-party certificate fingerprint alone cannot be used as a functioning WBD server identity: TLS `CertificateVerify` requires the corresponding private key. WBD will not disable certificate verification or claim a third-party hostname/certificate that the operator does not control. The diagnostic goal is to measure differential network treatment, not to forge the control site's identity.
