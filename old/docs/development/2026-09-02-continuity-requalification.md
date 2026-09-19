# 2026-09-02 — Canonical continuity merge and final exact-head requalification

## Purpose

This note is a durable recovery record for PR #9 after repeated chat interruptions. It records the live repository state that was re-read on 2026-09-02, the exact reason PR #9 had become dirty, what was changed to reconcile it, what was deliberately not changed, and the release/physical-acceptance boundary.

The detailed longitudinal history remains `docs/development/SINGLE_FLOW_DEVLOG.md`; the canonical machine-readable checkpoint remains `.wbd/handoff/current.json`.

## Product authority remains unchanged

The user-required transport contract is still:

- single-flow is **per Transport Lane**;
- FakeTCP owns each lane from its first raw SYN and owns that lane's public 4-tuple and sequence lineage through teardown;
- the opening bounded phase on that same FakeTCP association carries real TLS 1.3 / Reality-like admission with a Firefox-120-like public persona as closely as practical;
- the authenticated transition is in-band on the same association: no FIN, no RST, no reconnect and no second WBD payload SYN inside that lane;
- the same association then carries pinned wolfSSL DTLS 1.3 -> LINK -> lane-local FEC/VPN datagrams;
- reliable ordered behavior is setup-only; sustained ordinary-kernel-TCP/TCP-over-TCP head-of-line blocking is forbidden after the barrier;
- a Logical Tunnel may own 1..4 independent complete Transport Lanes; Game/race/dedup and make-before-break are product behavior;
- the mature FakeTCP/TCP-like ACK/SACK/RTO/recovery and release FEC behavior remains frozen unless a deterministic lower-layer test isolates a defect.

No change in this recovery cycle modifies those transport semantics.

## Live refresh at start of this cycle

Canonical branch:

- `dev/wbd-raw-fec-v2`
- live head at refresh: `d872156b4ee24890efecbd7e5d810ee59b9eb237`
- canonical handoff: sequence 81
- `handoff-verify / verify` check run `99011669397`: `success`.

Active implementation:

- branch `feat/single-flow-reality-faketcp`
- PR #9 `[V2.x] Per-lane same-flow transport + Logical Tunnel multipath lifecycle`
- live feature head at refresh: `b2fcf5406f4ed458f5979c7a821029f82254e4b7`
- commit message: `ci: requalify final single-flow candidate`.

For source head `b2fcf540...`, live Actions enumeration returned 30 workflow runs, all completed. No `conclusion=failure` and no in-progress run was found in that exact-head set. The combined `hosted Windows + Linux qualification` check was success, as were Linux raw/netns full-stack off and fixed 20:20 jobs. This is hosted qualification evidence only; it is not physical Windows/Npcap/NIC acceptance.

## Historical physical Windows failure and current code status

The important prior physical Windows 11/Npcap failure was:

```text
WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared
wbd-faketcp handshake: faketcp: not ipv4/tcp
wait for single-flow Reality ticket: ... timeout
```

The first deterministic failure was an unrelated physical-adapter frame entering the strict FakeTCP handshake parser. The later ticket timeout and child-stop noise were downstream symptoms.

The current feature branch has already fixed this boundary. `cmd/wbd-faketcp/main_windows.go` filters Npcap ingress before FakeTCP parsing and only forwards the exact, unfragmented inbound WBD IPv4/TCP reverse four-tuple. ARP, IPv6, UDP, ICMP, unrelated TCP, wrong peer/port, fragments, malformed packets and outbound self-capture are ignored. Exact local-kernel RST can still be observed diagnostically. Npcap `MODE_SENDTORX_CLEAR=0x0200` remains fail-fast.

The feature also contains deterministic Windows-only regressions for ARP/IPv6/LLDP, VLAN/QinQ, IPv4 options, malformed/truncated captures and a 4096-case mutation corpus. `cmd/wbd-faketcp/main_windows_handshake_test.go` reproduces the physical failure shape by injecting UDP, wrong-port TCP and outbound self-capture before a valid WBD SYNACK, then requires the FakeTCP three-way handshake to complete with the correct one-flow tuple and final ACK.

Therefore this recovery cycle does **not** reopen the mature TCP-like core or add a second generic parse workaround without new evidence.

## Why PR #9 was dirty

PR #9 was `mergeable=false / dirty` even though its product implementation had already advanced well beyond canonical. A compare of the old PR base to current canonical showed the canonical-only divergence consisted of repository continuity files rather than product executable source:

- `.wbd/handoff/current.json`;
- five dated qualification/recovery notes from 2026-08-29/30;
- the canonical-side `SINGLE_FLOW_DEVLOG.md` history.

The five dated canonical documents did not exist on the feature branch. The feature `SINGLE_FLOW_DEVLOG.md`, however, is substantially richer and newer than the canonical copy, so replacing it with the shorter canonical version would lose development history.

## Continuity merge

A real two-parent Git merge commit was created:

- commit: `6d887f82f2b4c1f0151147e8c12fa624a06c3fb1`
- parent 1: feature `b2fcf5406f4ed458f5979c7a821029f82254e4b7`
- parent 2: canonical `d872156b4ee24890efecbd7e5d810ee59b9eb237`
- message: `merge: reconcile canonical single-flow continuity`.

Merge resolution deliberately preserves the entire feature product tree and the richer feature `SINGLE_FLOW_DEVLOG.md`, while bringing in canonical sequence-81 handoff plus the five canonical-only dated history/qualification files. No FakeTCP, TLS/Reality-like bootstrap, DTLS, LINK, FEC, Windows runtime, Linux server or product lifecycle executable source was changed by the merge.

After fast-forwarding `feat/single-flow-reality-faketcp` to this merge commit, GitHub reported PR #9 `mergeable=true` against canonical `dev/wbd-raw-fec-v2`. The previous dirty state was therefore continuity ancestry/document reconciliation, not an unresolved product-code conflict.

## Exact-head requalification rule after the merge

Even though the merge does not change product behavior, its SHA is a new source HEAD. WBD release authority does not inherit old green runs across SHAs. The next and last feature commit in this cycle updates `docs/development/QUALIFICATION_KICK.md` and starts a fresh exact-head release qualification.

That kick must require all 28 hosted child gates already defined by `release-qualification-kick.yml`, including:

- combined hosted Windows-runtime -> Linux-server qualification;
- Windows portable/TUN/admin/raw-IP/persona/IPv6/DTLS;
- Linux server release/firewall/raw-netns/shared-TUN;
- one-SYN single-flow E2E and Reality-like TLS persona;
- post-switch no-HOL;
- FakeTCP native/pcap/first-arrival/recovery/load;
- Logical Tunnel 1..4 lanes, fifth rejection, Game race/dedup/no-cross-lane-HOL and make-before-break;
- FEC off and fixed systematic 20:20;
- OpenWrt regressions.

The candidate branch must remain immutable while the aggregator runs. A queued, skipped, stale-SHA or merely-started workflow is not counted as green.

## Delivery boundary

A successful hosted matrix can authorize a matching Windows/Linux **physical-test candidate**, but it cannot truthfully claim final physical acceptance. Final release acceptance still requires the exact same candidate source to pass the user's real path:

```text
physical Windows 11 + Npcap + physical NIC/NAT/ISP
    -> physical Ubuntu ARM64 WBD server
```

Do not tell the user that Windows and Linux are fully release-passed until that physical exact-candidate test succeeds. If physical E2E fails, classify the first deterministic marker and reproduce it in sandbox/hosted tests before touching the mature TCP-like recovery/FEC core.

## Immediate continuation

1. Create the final qualification-kick commit from the reconciled feature HEAD.
2. Freeze `feat/single-flow-reality-faketcp` on that exact SHA while `release-qualification-kick` dispatches and validates all 28 child gates.
3. On any failure, record the first deterministic red in this development log lineage, fix only that layer, and issue a new candidate/kick; never mix evidence from different SHAs.
4. If the exact-head matrix closes green, update the canonical machine-readable handoff to the next sequence with the candidate SHA and exact qualification evidence.
5. Only then prepare matching Windows x64 and Linux ARM64 artifacts for the final physical acceptance run.
