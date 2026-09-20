# P5 full/resumed handshake readiness fix

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Parent SOURCE_SHA: `e0650911ed2268c5b2f0744006016de924ea462d`
- Failed Actions: `35522232171`
- Current product qualification remains: `a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f`
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Second candidate result

Actions `35522232171` completed 7/8 PASS. The only failure was the P5 gate, before either inner HTTPS flow started:

`p5_https_measurement_test.go:492: server platformflow service missing before HTTPS flows`

The failure occurred at effectively 0.00s in the test after `DialClient` returned.

All other gates passed, including the P2 privileged kernel fallback that had failed once on the preceding SHA. On this run P2 completed in 1.24s with 29 packets captured, 58 received by filter, and 0 dropped by kernel. This confirms that the previous P2 EOF did not justify changing P2 inside this atom.

P5 failure artifact:

- ID `10609261179`
- digest `sha256:01a3bc4b10456fab88cfed495a315a4c5841cb71ba83133575b551f7df255a23`.

P2 PASS artifact:

- ID `10609166459`
- digest `sha256:da9fe54bf57475e73da6b56836673be4b5c41bbf0226134eb8de0635a3252208`.

## Root cause

The harness assumed that server-side admission/service installation was visible immediately when the client-side `DialClient` returned. Those operations are on different goroutines.

The server only inserts the fully constructed `serverTunnel` into `byTunnel` after:

- protected admission;
- lease lookup;
- runtime creation;
- service creation;
- router service-handler installation;
- server lane attach.

Therefore direct one-shot lookup is a test readiness race, not a product handshake or session-resumption result.

## Narrow fix

The third candidate replaces the one-shot lookup with a bounded condition wait:

- poll the authoritative `server.byTunnel[tunnelID]` under its mutex;
- return immediately once a non-nil tunnel/service exists;
- use a 1ms ticker only to recheck the condition;
- enforce a 2s absolute deadline;
- fail if readiness never arrives.

This wait occurs before `recorder.markSteady()`, so it is outside the measured business interval and does not alter packet timing, TLS handshake timing, burst accounting, or business latency.

It is not a fixed sleep and it does not wait for unrelated business traffic.

No production code changes. No FEC, padding, RTO, lane, topology, or certificate-chain changes. The TLS1.3 full/resumed logic and validator are unchanged from the previous candidate.

## Qualification

The candidate must pass all eight `next-foundation` jobs at the exact SOURCE_SHA. The P5 gate must still independently validate:

- first inner handshake full;
- real ticket cache Put;
- second inner handshake resumed;
- real cache Hit;
- first flow fully closed before second;
- same outer lane and one outer SYN;
- FEC off, padding off, no network injection.

No `old/` reuse; REUSE_LEDGER unchanged. Windows/Npcap physical remains P7 `NOT_RUN`.
