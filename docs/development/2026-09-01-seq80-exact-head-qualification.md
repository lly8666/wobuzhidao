# Sequence 80 exact-head qualification recovery

Date: 2026-09-01

This note is durable recovery evidence for the single-flow product branch. It records the live state recovered after repeated chat/session interruptions and the release-qualification action taken from that state. Chat history is not authority.

## Product authority

Current authority remains ADR-0011 + ADR-0012 as captured by canonical handoff sequence 79 and `docs/development/SINGLE_FLOW_DEVLOG.md`.

Per Transport Lane:

```text
one raw FakeTCP SYN / one public 4-tuple / one FakeTCP sequence space
  -> bounded reliable ordered same-association bootstrap
  -> real TLS 1.3 Reality-like admission / Firefox-120-like public persona where practical
  -> explicit in-band barrier, no FIN/RST/reconnect/new SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

A Logical Tunnel may own 1..4 independent complete Transport Lanes. Game/race/make-before-break operate above the per-lane transport. Mature FakeTCP/TCP-like ACK/SACK/RTO/recovery and release FEC behavior remain frozen unless a deterministic lower-layer gate proves a defect.

The retired topology `ordinary Reality TCP -> close -> second FakeTCP SYN` must not return.

## Recovery snapshot

Canonical branch at recovery:

```text
dev/wbd-raw-fec-v2
7d7a6acc945a69f8b811a1accae943ca8acfffd5
handoff: sequence 79 recover current single-flow product state
```

Active feature branch before this log commit:

```text
feat/single-flow-reality-faketcp
49dcbe4bb8013d281f08160b0b5576296d1561ac
ci: require exact-head physical Npcap qualification path
PR #9
```

Sequence 79 correctly states that packaging success alone is not release authority and that final physical Windows 11 Npcap/NIC/NAT/ISP -> Ubuntu ARM64 acceptance remains mandatory after hosted qualification.

## Exact-head push evidence for 49dcbe4b

The live GitHub Actions refresh for `49dcbe4bb8013d281f08160b0b5576296d1561ac` found all ten branch push runs completed successfully:

- `handoff-verify` run 33492715418 — success
- `ci` run 33492715432 — success
- `faketcp-native` run 33492715433 — success
- `faketcp-pcap-20loss` run 33492715451 — success
- `faketcp-first-arrival` run 33492715458 — success
- `fullstack-first-arrival` run 33492715445 — success
- `openwrt-tcp-tproxy` run 33492715496 — success
- `single-flow-e2e` run 33492715428 — success
- `single-flow-no-hol` run 33492715401 — success
- `single-flow-tcp-persona` run 33492715534 — success

The check-run set also contained path-conditioned skips such as transport benchmark jobs. Skipped jobs are not classified as green release evidence.

This push evidence is useful but is **not release authorization**, because the expensive Windows/Linux/product workflow-dispatch matrix had not been re-established for this exact post-qualification-infrastructure HEAD.

## Physical Npcap qualification path added after the previous kick

Commit `49dcbe4bb8013d281f08160b0b5576296d1561ac` added `.github/workflows/windows-npcap-physical.yml`.

The workflow deliberately targets an elevated self-hosted physical Windows x64 runner labelled `wbd-npcap`; it does not attempt to silently install Npcap Free Edition in hosted CI. It requires Npcap 1.88 already installed normally, downloads the exact successful Windows portable artifact, verifies the artifact run `head_sha` equals the requested source SHA and verifies the Actions ZIP digest, then runs `scripts/windows_npcap_physical_qualify.ps1` and uploads the support JSONL.

This makes physical acceptance exact-head and artifact-bound, but it does not magically create a physical runner. Hosted qualification still has to pass first.

## Why a new qualification kick is required

The exact-head release authority is `.github/workflows/release-qualification-kick.yml` plus `docs/development/QUALIFICATION_KICK.md`.

The aggregator requires one immutable feature HEAD, dispatches 19 exact-head product workflows, resolves 9 exact-head push gates, rejects mixed SHAs or branch movement, and emits `WBD_RELEASE_QUALIFICATION_PASS` only if every child is `success`.

The 19 dispatched gates cover:

- product lifecycle 1..4 lanes / Game / make-before-break;
- combined Windows-runtime -> Linux-server single-flow;
- Windows portable, TUN, admin, raw-IP, FakeTCP persona, IPv6, DTLS;
- Linux server release and firewall;
- raw-IP single-flow, startup stress and LINK fullstack;
- 100 Mbit mux weak-network/load qualification;
- FakeTCP recovery;
- OpenWrt fullstack;
- Game lane fullstack;
- shared-TUN two-client / one-NAT topology.

The 9 exact-head push gates are main CI, FakeTCP native/pcap/first-arrival, fullstack first-arrival, OpenWrt TCP TPROXY, single-flow E2E, no-HOL and TCP persona.

Because the physical-Npcap qualification infrastructure was added after the previous qualification generation, `49dcbe4b` cannot inherit an older candidate pass. A fresh qualification kick must be the last feature-branch commit and the branch must remain frozen while its 28 child gates run.

## Action for sequence 80

1. Commit this durable recovery note.
2. Update `docs/development/QUALIFICATION_KICK.md` as the final feature-branch commit with a new generation ID.
3. Freeze `feat/single-flow-reality-faketcp` on that exact resulting SHA.
4. Let `release-qualification-kick` dispatch and verify all 28 hosted child gates.
5. On failure, inspect the first deterministic non-transport blocker, record it durably, fix only that layer, then create a new candidate/kick. Do not retune mature TCP-like recovery without deterministic transport evidence.
6. Only after an exact-head hosted qualification pass, verify matching Windows x64 portable and Linux ARM64 release artifacts from that same SHA.
7. Then run the exact-head physical Windows Npcap acceptance against physical Ubuntu ARM64 before calling the release complete.

No artifact is delivered to the user from this recovery snapshot.