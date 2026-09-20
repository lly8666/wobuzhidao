# P5 HTTPS measurement base fixture-fix candidate

- Date: 2026-09-20
- Branch authority: `next/tlslike-dataplane`
- Parent SOURCE_SHA: `bd96f618b001139dbd971e46d7bacc6ba6dbae2d`
- Failed Actions preserved: `35516224912`
- Previous product qualification remains: P4 `76a20c865bf86edf2098f0ef40c44a31298679dd` / Actions `35510241639` / 7/7 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Failure classification

The first P5 measurement-base candidate reached the dedicated runtime harness and failed on the second controlled HTTPS flow with:

`io: read/write on closed pipe`

The first HTTPS flow had already completed. The harness used `net.Pipe` for the local application-side connection passed to the existing `platformflow.Client.AddTCP` seam. `net.Pipe` has no TCP half-close/`CloseWrite`; closing the first TLS/HTTP connection therefore fully closed the fixture side instead of exercising the platformflow TCP half-close path. This is classified as a **runtime fixture/lifecycle failure in the new P5 harness**, not as a transport-algorithm result.

The same Actions run also had a separate Linux race failure in the unchanged P4 closure test `TestLifecycleEntryGameThreeAndFourLaneMatrix/lanes-3` while normal unit/build, Windows, P2 kernel fallback, Linux shared-TUN iptables/nft and OpenWrt privileged jobs passed. This P5 atom does not reopen or modify P4. The next exact-SHA full workflow will rerun that existing gate.

Failed P5 artifact is retained as Actions artifact `10606209841`, zip digest `sha256:2f46503ae2d3d03e20a4db59eb339a741f90f427a5ef1c315e3db98fb00f9a76`.

## Narrow fix

Only the P5 measurement fixture is changed:

- replace the inner application `net.Pipe` pair with a loopback TCP connection accepted from `127.0.0.1:0`;
- pass the accepted TCP side to the existing `platformflow.Client.AddTCP` path, preserving the same formal TunnelOwner/lane path;
- the loopback TCP endpoint implements real half-close/`CloseWrite`, matching the lifecycle semantics already expected by `platformflow`;
- order test cleanup so runtimeentry server cancellation/wait happens while client/service endpoints are still alive, avoiding a cleanup-induced read error.

This loopback socket is **inner business-test plumbing**, not an outer carrier and not a restoration of the prohibited localhost UDP/Controller/DTLS topology. The outer WBD connection remains the current FakeTCP + TLS/protected-admission runtimeentry association.

No production transport code, FEC policy, padding policy, weak-network parameters, classifier, or P7 physical path is changed. No `old/` code is reused, so `docs/REUSE_LEDGER.json` remains unchanged.

## Qualification rule

No local Go test/build/race/network result is qualification authority. The new candidate must pass the full `next-foundation` workflow at its exact SOURCE_SHA, including `p5-https-measurement-base` and all existing gates. `docs/STATUS.json.last_tested_source_sha` deliberately remains `76a20c865bf86edf2098f0ef40c44a31298679dd` until a new product qualification is established.
