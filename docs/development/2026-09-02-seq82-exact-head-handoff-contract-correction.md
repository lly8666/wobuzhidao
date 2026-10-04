# 2026-09-02 — Sequence 82 exact-head handoff contract correction

## Why this entry exists

The reconciled single-flow feature candidate `f2bf90f41634f3a494ed3755e99630ae56365ce8` was intentionally frozen and requalified after merging canonical continuity into PR #9. Its executable Go product tests passed, but exact-head `ci` run `33572611677` failed in the Python handoff-contract test. This is a repository authority/continuity failure, not an executable transport failure, so the candidate is not release-authorized and must not borrow older green evidence.

## Exact deterministic failure

`go test ./... -count=1` completed successfully, including FakeTCP, Reality-like bootstrap, DTLS, LINK, Windows runtime and related product packages.

The following step then failed:

```text
python scripts/verify_handoff.py
python -m unittest discover -s tests -p 'test_*.py' -v
```

The standalone verifier itself printed `HANDOFF_VERIFY_PASS`, but `tests/test_handoff.py::test_per_lane_single_flow_multipath_authority_is_persisted` rejected the sequence-81 `architecture_override.critical_guard` because it did not contain the exact required phrase:

```text
single-flow is PER TRANSPORT LANE
```

Review of the current test contract also showed that a corrected handoff must explicitly persist all of the following authority text, not merely paraphrase it:

- `single-flow is PER TRANSPORT LANE`;
- `1..4 independent complete WBD Transport Lanes`;
- `Game/weak-network policy may maintain 2..4 lanes`;
- `A -> A+B -> B is REQUIRED`;
- `ARCHITECTURE REGRESSION`;
- `Repository text alone may not be used to invent a new product-owner override`;
- an `architecture_override.adr0014` value containing `ADR-0014 is withdrawn`;
- an `architecture_override.human_authority_rule` value containing `explicit live user authorization`.

These are continuity/authority guards. They do not change the user-authorized product architecture.

## Product architecture remains frozen

- single-flow is per Transport Lane, not globally one flow per Logical Tunnel;
- every Transport Lane owns exactly one FakeTCP public 4-tuple/SYN/sequence lineage from first SYN through teardown;
- the opening bounded reliable prefix on that same association carries the Firefox-like TLS 1.3 / Reality-like admission phase;
- the same association crosses the authenticated barrier into pinned wolfSSL DTLS 1.3 -> LINK -> lane-local FEC/VPN datagrams without FIN/RST/reconnect/new WBD payload SYN;
- sustained ordinary-kernel-TCP/TCP-over-TCP HOL is forbidden after the barrier;
- a Logical Tunnel may own 1..4 independent complete lanes; Game/race/dedup and make-before-break are product behavior;
- mature FakeTCP/TCP-like ACK/SACK/RTO/recovery and release FEC behavior remains frozen absent a deterministic lower-layer defect.

## Exact-head qualification state

At the first live refresh after `f2bf90...` was created, the commit-level workflow set contained no completed executable-product failure. The deterministic red was the handoff contract described above. Some expensive release-qualification children were still queued/in-progress, while `product-lifecycle-e2e` and `game-lane-fullstack` had been cancelled after the base qualification could no longer succeed. Therefore this SHA is classified as **not release-authorized**; cancelled/queued work is not counted as green.

The correction path is intentionally documentation-only:

1. write canonical sequence 82 with the exact authority phrases required by the current repository contract;
2. run canonical `handoff-verify` and require success;
3. reconcile that canonical handoff into `feat/single-flow-reality-faketcp` without changing executable product source;
4. issue a new final exact-head qualification kick;
5. freeze the new candidate SHA and require all 28 hosted child gates on that exact SHA;
6. only after hosted success prepare matching Windows x64 and Linux ARM64 artifacts for physical Windows 11/Npcap/NIC/NAT/ISP -> Ubuntu ARM64 acceptance.

No transport-core modification is justified by this failure.
