# 20260924-131500 V9 bounded receive overflow shedding

Exact product SOURCE_SHA: `af6d1f21cc589c4ebfffc07efc38df3ce4ff97bf`

## Trigger

V8 product remains `27e4bb34ae35e4c48fc4ebaa04ba3e3d161b4089`. Its dedicated final18 is permanently recorded as 16 PASS / 2 CAPACITY_LIMITED; the two failed strict runs are not replaced or rerun.

The failures share one amplification mechanism:

- Normal/5305/seed202 `35955301841`: client AF_PACKET drops=3121; SegmentMux route depth 4096 was exhausted, full events=231, handoff block max about 784ms, queue age max about 1.319s.
- Game4/5305/seed303 `35955313644`: server AF_PACKET drops=10460; LifecycleServer ready depth 4096 was exhausted, handoff block max about 1.895s, handler max about 2.333s, queue age max about 2.338s.

Both retained link/qdisc drops=0 and runtimeowner `FreshBlocked=0`, `FreshWindowBypass=0`, `FreshEmitFailures=0`, `RepairEvictionMaxScan=1`. Passing same-generation 5305 samples stayed far below 4096.

## V9 product delta

V9 does not enlarge any buffer.

- SegmentMux route capacity remains 4096.
- LifecycleServer ready capacity remains 4096.
- A valid packet offered to a full receive handoff no longer waits for a free slot.
- The single producer removes at most one oldest queued record in O(1) and offers the newest record into the released slot.
- Overflow records, bytes and queued age are explicit diagnostics.
- Terminal read errors are preserved rather than shed.
- No worker pool, per-packet goroutine, unbounded queue or scan is introduced.

The intent is partial reliability plus freshness: under a rare seconds-long consumer stall, shed stale queued work instead of blocking the raw reader until the kernel starts dropping arbitrary packets.

Unchanged: active repair=4096, one-shot reserve=1024, FEC20:20, wire, 1s InitialRTO, 3s repair horizon, fresh/5 repair credit, 128KiB repair burst, kernel packet socket buffer, loss thresholds and analyzer thresholds.

## Validation order

All compile/test/race/performance remains GitHub Actions only.

1. First run correctness/race only: runtimeowner/runtimeentry coverage, targeted, foundation and lifecycle.
2. Any failure stops performance.
3. If green, run one new Normal/5305/seed202 diagnostic canary and one new Game4/5305/seed303 diagnostic canary at the exact V9 product SHA.
4. Require five classifications PASS, socket/link drops=0, bounded explicit overflow, fresh three counters zero and `RepairEvictionMaxScan<=1`.
5. These canaries do not replace the historical V8 CAPACITY_LIMITED samples and do not count toward a future final18.
