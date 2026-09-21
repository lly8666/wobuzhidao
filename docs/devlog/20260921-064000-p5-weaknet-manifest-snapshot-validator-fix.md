# P5 weak-network manifest snapshot validator fix

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base HEAD: `97f60ec0d1144261f3c6d3813685c6aebd629398`
- Failed Actions: `35568056556`
- Current product qualification remains: `b7c0b3d334ba1c52f9f53479eb8a2f4e4de692e3` / Actions `35555303253`
- Scope: weak-network validator only plus STATUS/devlog

## Fourth-candidate result

All eight pre-existing required jobs passed on the exact SHA, including the Windows lanes-4 test that had failed intermittently on the previous candidate.

Both dedicated weak-network jobs completed the full 120 second harness and the Go test itself passed.

Run 1 / seed 20260921:

- scheduled 40;
- success 39;
- application loss 1;
- repair retransmits 3815;
- wire amplification 8.657677;
- artifact `10624786896`;
- digest `sha256:0d927725803fc752f0962d4acc42e6c74cb9f5faf54b1cbfea44c94a8ebbbd10`.

Run 2 / seed 20260922:

- scheduled 40;
- success 38;
- application loss 2;
- repair retransmits 3365;
- wire amplification 8.389240;
- artifact `10625285703`;
- digest `sha256:39c47850626a9ca5d4a2e5c2d0047d8abbdca75064d405cc04e1a6918c198286`.

Both validators progressed through phase/drop provenance, chronological interval reconstruction, request/loss/duplicate checks, post5 recovery, FEC-off/padding-off inventory and wire-cost checks, then stopped at:

`manifest capture inventory`

## Raw capture audit

Run 1:

- manifest live snapshot outer packets: 13815;
- final outer events: 13860;
- final PCAP packets: 13860;
- manifest live snapshot record events: 7415;
- final record events: 7426;
- snapshot tail: 45 outer + 11 record;
- first tail timestamp: about 121.451 s.

Run 2:

- manifest live snapshot outer packets: 12625;
- final outer events: 12637;
- final PCAP packets: 12637;
- manifest live snapshot record events: 6769;
- final record events: 6770;
- snapshot tail: 12 outer + 1 record;
- first outer tail timestamp: about 120.608 s;
- record tail timestamp: about 120.762 s.

In both runs the final PCAP packet count exactly equals the final outer-event count.

## Root cause

The harness calls `recorder.counters()` while the client/server runtime and tick goroutines are still alive. Manifest creation happens before deferred shutdown.

Therefore the manifest counters are an atomic live snapshot. During the following bounded flow convergence/deferred shutdown, repair and ACK traffic can still be emitted. Every such packet is synchronously serialized by the same recorder into both PCAP and JSONL, so final raw files legitimately contain more events than the earlier manifest snapshot.

The previous validator incorrectly treated the manifest snapshot as the final process-exit capture count.

## Fix

Only `tools/check_p5_weaknet_measurement.py` changes.

Final capture completeness remains a hard raw-data gate:

- final PCAP packet count must exactly equal final outer-event count;
- both directions must be present;
- packet/record fields and chronological timing remain independently checked.

Manifest counters are now validated as shutdown-before-close snapshots:

- `capture_loss_packets` must remain 0;
- snapshot outer/record counts must be positive integers;
- snapshot counts must not exceed final raw counts;
- `client_initial_syns` must remain exactly 1;
- every raw outer/record event beyond the snapshot prefix must have `t_ns >= 120s`.

Thus no evidence may disappear inside the prescribed measurement window, while legitimate post-window shutdown tail traffic is not mislabeled as capture loss.

No recorder, network injector, runtime transport, workload, FEC, padding, repair behavior or performance threshold changes.

## VM/performance policy

The two weak-network runs remain separate Actions jobs, one VM per seed and scenario. CPU, PPS, memory and GC remain observational only; there is no absolute cross-runner threshold.

## Next

Create a new exact SOURCE_SHA and run all ten `next-foundation` jobs. Both weak-network jobs and all eight existing jobs must pass on the same SHA before the atom can close.
