# P5 weak-network chronological interval validator fix

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base HEAD: `906b8c2a4c7a717760a48648e15e4b38ba855ab5`
- Failed Actions: `35564363961`
- Current product qualification remains: `b7c0b3d334ba1c52f9f53479eb8a2f4e4de692e3` / Actions `35555303253`
- Scope: weak-network validator only plus STATUS/devlog

## Preserved third-candidate result

The third weak-network candidate ran ten jobs. Seven passed. Both dedicated weak-network jobs completed the full 120 second harness and the Go test itself passed; both then failed only in the independent validator.

Run 1 / seed 20260921:

- scheduled 40;
- success 38;
- application loss 2;
- repair retransmits 3463;
- wire amplification 8.577119;
- artifact `10623955497`;
- digest `sha256:fe6a42d870234a7648e2be4a83e750c213503da426cfae96f107582202981f44`.

Run 2 / seed 20260922:

- scheduled 40;
- success 37;
- application loss 3;
- repair retransmits 3615;
- wire amplification 9.082183;
- artifact `10623311429`;
- digest `sha256:cefd5b86be83c76cd1a60291cc3ff671e55ab2faf326563027baef5535b0c883`.

Both validators stopped at:

`outer interval raw evidence`

The previous phase-ordinal and zero/omitempty fixes therefore both progressed validation farther than their original failure points.

## Raw artifact audit

The failed artifacts were downloaded and inspected directly.

Run 1:

- outer packet events: 12663;
- omitted `inter_arrival_ns`: 2;
- negative outer intervals: 14;
- minimum outer interval: -155365 ns;
- negative TLS-like record intervals: 3;
- minimum record interval: -149614 ns.

Run 2:

- outer packet events: 13169;
- omitted `inter_arrival_ns`: 2;
- negative outer intervals: 10;
- minimum outer interval: -41190 ns;
- negative TLS-like record intervals: 0.

These are microsecond-scale serialization artifacts, not 300 ms network-delay violations.

## Root cause

`p5MeasurementRecorder.wrapIO` samples `time.Now()` before entering the recorder's shared mutex. Two concurrent Emit calls in the same direction can therefore behave like this:

1. call A samples timestamp t1;
2. call B samples later timestamp t2;
3. B acquires the recorder mutex first and is written first;
4. A acquires it second and computes t1 - t2 as a negative convenience interval.

The raw `t_ns` timestamp on each event remains intact. JSONL append order is not guaranteed to be chronological under that concurrency.

Changing product transport or adding timing sleeps would be the wrong fix.

## Fix

Only `tools/check_p5_weaknet_measurement.py` changes.

For outer packet and TLS-like record events the validator now:

- requires a non-negative integer raw `t_ns`;
- keeps validating packet/record lengths, direction and burst fields;
- treats omitted `inter_arrival_ns` as schema-zero and requires any serialized value to be an integer;
- groups events by direction;
- sorts by `(t_ns, original_file_index)`;
- independently reconstructs chronological intervals from raw `t_ns`;
- requires reconstructed intervals to be non-negative and the multi-sample timeline not to collapse to one timestamp.

The serialized `inter_arrival_ns` field is no longer mistaken for the timing authority when concurrent recorder calls were appended out of sampled-time order.

No recorder, injector, workload, product runtime, FEC, padding, repair or queue behavior changes.

## Windows P4 regression signal

The same workflow also had one unrelated Windows unit failure:

`TestLifecycleEntryGameThreeAndFourLaneMatrix/lanes-4`

with server stats `GameDelivered=1 GameDuplicates=2`.

This is the same unchanged P4 timing assertion signature that appeared once on fixed-FEC candidate `7aae4993…` and then passed on its retry. It also passed on the first two weak-network candidates. This atom therefore records the evidence but does not modify or reopen P4. A new full exact-SHA run must decide whether the failure repeats stably.

## Actions/VM policy unchanged

Each weak-network run remains one dedicated Actions VM, one seed, one 120 second scenario and one artifact.

CPU, PPS, memory and GC are observational for that runner only. No absolute cross-VM performance threshold is introduced.

## Next

Create a new exact SOURCE_SHA and run the full ten-job `next-foundation` qualification. Both weak-network runs and all eight existing jobs must pass on that same SHA. Do not mix in 5% -> 30% -> 5%, load, soak or FEC tuning.
