# 2026-09-05 Windows Physical Retest Handoff

This document closes the handoff gap between the stale hosted-qualification checkpoint `0e0bf6865b719ebd17d9d2d4dd0e5463373729a3` and the current Windows L3-fence runtime candidate `2e44c407eee677252897f2c75942407687ff8450`.

It is a durable reconstruction record for PR #9. It records what the two recent physical runs actually proved, what they did not prove, the packaging/runtime-closure repair chain between them, and the exact boundary for the next test candidate. It must not be read as release authorization.

## 1. Architecture and qualification invariants

The following remain controlling constraints throughout this history:

- ADR-0012 controls Logical Tunnel multipath/lifecycle; ADR-0011 controls the per-lane same-association bootstrap/no-HOL lineage.
- `single-flow` is **per Transport Lane / transport epoch**, not global to the Logical Tunnel lifetime.
- One Logical Tunnel owns stable identity/lease and one logical SessionID/PacketID race namespace with 1..4 logical Transport Lanes.
- Normal desired lanes = 1. Game/weak-network policy may use 2..4.
- `wbd-game-lane-client` is the internal race/dedup aggregation point even when there is only one public lane. A one-lane profile is not permission to bypass Game.
- First valid PacketID arrival wins; duplicates are suppressed; bounded out-of-order unique packets may deliver independently; there is no cross-lane HOL.
- Planned replacement is generation-fenced make-before-break `A -> A+B -> B`; candidate failure preserves healthy A.
- Four logical lanes use a bounded pool of 1..5 physical slots; the extra slot is replacement overlap capacity only, never a fifth logical lane.
- Linux final product path remains one shared WBD TUN + root routing + one WBD-owned host NAT.
- Raw IPv4 ingress source must equal the Logical Tunnel leased IPv4. This anti-spoof boundary is not to be weakened to accommodate client bugs.
- FakeTCP/Reality-like TLS/pinned wolfSSL DTLS 1.3/LINK/FEC wire is frozen absent deterministic first-broken-layer evidence.
- Functional qualification uses FEC OFF first. Fixed systematic 20:20 is compatibility smoke only.
- Windows IPv6 remains fail-closed until real IPv6 qualification.
- SourceIP/default-route changes are not authoritative direct public NAT mapping reflection.
- Hosted CI/build success and a public `:443` pcap are not substitutes for physical Wintun/shared-TUN/application-path evidence.
- Release evidence from different source SHAs must never be stitched into one release candidate.

## 2. Stale checkpoint and the 13 intervening commits

The old machine handoff stopped at source:

`0e0bf6865b719ebd17d9d2d4dd0e5463373729a3`

It correctly recorded hosted exact-source qualification as green at that point but still treated physical Windows acceptance as entirely unproven. The following ordinary commits occurred before the current runtime candidate:

1. `1138832b...` — `test: qualify linked windows portable runtime`
2. `4b36b52e...` — `build: route windows physical test through portable wbd`
3. `7b441250...` — `test: validate physical portable runtime`
4. `bb0984cc...` — `test: encode physical portable runtime in contract`
5. `be7607093b3065bddce024048ba376f2e5d21cdd` — `ci: publish windows test prerelease`
6. `bde38123401c3da38e76538f7ad0ecc16645cc88` — `fix: harden test release source pin`
7. `4c0004362ffb6bb3b0eb25cee41c3c7800b6c9a9` — `fix: separate release tag from source sha`
8. `adb6bb4927e60a0047045908d5032a7885209e08` — `fix: make test release idempotent`
9. `4afc6757e88e6797637ef4c59d2b25e6e5d7c842` — `fix: harden windows prerelease publish`
10. `e042796373e23fa4665f7647bf767d5bc36f482b` — `fix: package windows game runtime child`
11. `4d391528779bd771e368f0f1bd44dfec6e2a8883` — `test: fix windows game runtime contract`
12. `cf8298f9cb30c7aa9d60ca00611a783c401ba735` — `test: tighten windows game runtime contract`
13. `2e44c407eee677252897f2c75942407687ff8450` — `fix: fence Windows tunnel L3 identity`

The handoff protocol sequence is **not** incremented once per ordinary commit. The next completed handoff phase advances sequence 98 to 99 while preserving this commit chronology here.

## 3. Physical round 1 — `be760709...`

### 3.1 Test source and external evidence identity

Test release tag:

`wbd-test-be7607093b30`

Exact source:

`be7607093b3065bddce024048ba376f2e5d21cdd`

The externally supplied failure log and capture used during diagnosis had these recorded identities:

- failure-log SHA256: `0167fd5f49f52ec0e7587a96282740652bc2e65ffe66b40374b03440f6caeb7e`
- pcap size: `2,618,636` bytes
- pcap SHA256: `b51844853f1d86977ad5283dad35e0b4a4b4249527ea43c472b4e30fade63767`

The pcap hash/size matched the evidence hash written in the supplied failure log, so the capture analyzed was the original capture referred to by that log. These external attachments are not themselves stored in this repository; the hashes are retained here so later work does not silently substitute another file.

### 3.2 What the public `:443` capture proved

The relevant server-side flow was:

`210.13.100.75:48287 <-> 10.0.0.130:443`

The Windows raw local source port was `57930`; the public capture saw NAT-rewritten source port `48287`. That is ordinary NAT source-port rewriting, not a tuple inconsistency.

The target flow contained 47 packets. Packet bytes independently corroborated, rather than merely trusting process markers:

1. raw FakeTCP SYN/SYNACK on the same public association;
2. a TLS ClientHello-like record containing SNI `www.cloudflare.com`;
3. the corresponding server TLS record;
4. later DTLS record framing on that same TCP-shaped FakeTCP lineage;
5. encrypted traffic continuing briefly after the Windows process reported the later child-start failure, consistent with cleanup rather than an earlier public-path collapse.

There was no evidence in that target flow of a network RST/FIN ending the session before the runtime-child failure. The capture therefore established the outer physical sequence:

`Windows Npcap/FakeTCP -> public :443 -> Reality-like TLS bootstrap -> DTLS 1.3 -> LINK`

### 3.3 What round 1 did **not** prove

It did not prove:

`Game -> Wintun -> Logical Tunnel backend -> shared Linux TUN -> host NAT -> application echo`

Key process boundary:

- server reached `WBD_LINK_MUX_SESSION_READY ... lanes=1 backend=pending`;
- Windows summary still had `WBD_TUN_READY=0`;
- no authoritative `WBD_SHARED_TUN_SESSION_READY`, `WBD_SHARED_RAWIP_RX_FIRST` or `WBD_SHARED_RAWIP_TX_FIRST` was present for this run.

Therefore the correct wording is: **the public transport bootstrap through LINK physically established; the full Logical Tunnel/TUN application path was not reached.**

### 3.4 Deterministic root cause: Windows portable runtime closure drift

Runtime code used `BuildMultiLanePlan()` / `StartMultiLane()` even for profile `Lanes=1`. That plan creates `wbd-game-lane-client.exe` and points TUN transport at the Game listen address. Start order places Game before TUN.

The Windows portable producer, however, omitted `wbd-game-lane-client.exe` from:

- the Windows build step;
- the manifest required set;
- embedded extraction qualification;
- workflow path triggers.

The physical workflow runtime assertion also omitted the Game child.

This was **runtime dependency graph vs portable build/manifest contract drift**. It was not an architecture reason to bypass Game when `Lanes=1`.

The preflight marker `dependency_preflight_pass=1` was also narrower than its wording suggested: it validated profile/routing/Npcap prerequisites and manifest-listed files, not the complete set of executables that the Controller could later start.

### 3.5 Important interpretation corrections from round 1

- A firewall OUTPUT counter on the kernel RST-drop rule proves RST suppression was exercised; it does not prove WBD server application response bytes. The pcap was the response-byte evidence.
- The earlier public `40443` test was caught by an existing host NAT redirect (`20443:40443 -> 10443`) where an old service listened. Final physical WBD testing moved to public `443`.
- Internal LINK moved from default `47000` to `127.0.0.1:47010` because existing frps occupied `47000`.
- The shared Linux TUN had logged `raw-IP packet is not IPv4` even before the Windows connection. IPv6/control traffic was a plausible explanation but the outer `:443` pcap could not prove packet identity; no shared-TUN wire change was justified from that observation alone.

## 4. Packaging/runtime closure repair — `e042...` to `cf8298...`

### 4.1 `e0427963...`

The producer was corrected to:

- build `cmd/wbd-game-lane-client` as `wbd-game-lane-client.exe`;
- include it in the embedded runtime manifest required set;
- include it in embedded extraction qualification;
- trigger the Windows portable producer when `cmd/wbd-game-lane-client/**` changes;
- include the Game child in the physical runtime expectation.

A static release-contract regression test was added, but its first version mistakenly expected doubled backslashes because the Go raw string assertion itself was escaped incorrectly. That CI failure was a test bug, not a product/runtime failure.

### 4.2 `4d391528...`

The escaping mistake was fixed. The new contract still failed because the occurrence-count assertion expected at least four mentions of the Game executable while the correct producer contract contained three relevant occurrences: build output, manifest required set, and embedded extraction required set.

Again, this was a regression-test assertion bug; product packages were not reinterpreted as passing on that basis.

### 4.3 `cf8298f9...`

The contract test was tightened to the correct expectation. For this exact SHA:

- the main CI test job completed green;
- the Windows runner actually compiled `wbd-game-lane-client.exe`;
- the Windows portable manifest/PE/extraction qualification completed;
- extraction from the single-file `wbd.exe` verified the Game child was present in the embedded runtime;
- Linux amd64 and ARM64 server artifacts were built from the same SHA.

Only this exact source was delivered as the next physical candidate. Earlier intermediate artifacts from `e042...` and `4d391...` were intentionally treated as superseded.

## 5. Physical round 2 — `cf8298f9...`

Round 2 demonstrated that the packaging repair crossed the previous break.

### 5.1 Newly proven boundaries

Windows evidence included:

- `WBD_GAME_LANE_CLIENT_READY`;
- `WBD_TUN_READY`;
- Logical Tunnel IPv4 lease `10.66.0.1`;
- client `connect_pass`.

Server evidence included:

- `WBD_LINK_MUX_BACKEND_READY`;
- `WBD_GAME_LANE_SESSION_OPEN`;
- `WBD_SHARED_TUN_SESSION_READY`.

Therefore the path had progressed through:

`public transport -> LINK -> GAME -> Windows TUN -> server Logical Tunnel backend/shared-TUN session`

This is strictly stronger than round 1 and confirms that `wbd-game-lane-client` was not only packaged but operational in the intended one-lane topology.

### 5.2 New first-broken layer

After shared-TUN session establishment, the server observed:

- many payloads rejected as non-IPv4;
- IPv4 packets whose source was `169.254.99.241`;
- expected Logical Tunnel lease source `10.66.0.1`.

The server anti-spoof rule therefore correctly rejected the APIPA-sourced packets. Route-fenced DNS, generic UDP and TCP probes timed out, so full application E2E was still not proven.

The server anti-spoof boundary must remain strict. Accepting `169.254.*` would hide the client identity bug and violate the Logical Tunnel lease invariant.

The externally supplied round-2 support log/server journal/pcap were analyzed during the development session. Their exact attachment hashes are not preserved in the current repository context, so this document deliberately does not invent hashes for them. The marker-level findings above are the durable conclusion; a future release receipt must carry its own exact evidence identities.

## 6. Root cause and current fix — `2e44c407...`

The second physical failure mapped onto two Windows-side implementation gaps:

1. the route/address script ensured the lease address existed but did not guarantee that an already-present APIPA/other IPv4 address was removed from the WBD Wintun;
2. the Windows TUN bridge forwarded L3 data into Game/raw-IP without a strict IPv4 gate, allowing non-IPv4 traffic to reach an IPv4-only backend.

Commit:

`2e44c407eee677252897f2c75942407687ff8450` — `fix: fence Windows tunnel L3 identity`

implements a client-side fail-closed repair:

- target only the WBD-owned Wintun InterfaceIndex;
- disable IPv4 DHCP on that WBD virtual interface;
- remove all Wintun IPv4 addresses except the server-issued lease;
- ensure the server-issued lease exists;
- re-read/verify that the lease is the exclusive IPv4 before route setup proceeds;
- permit only valid IPv4 packets from Windows TUN into Game/raw-IP;
- locally drop non-IPv4 packets instead of sending them into the shared IPv4 backend.

Expected new marker:

`WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE ... address4=10.66.0.1 ... dhcp=disabled`

Possible fail-closed informational marker:

`WBD_TUN_WINDOWS_NON_IPV4_DROP fail_closed=1`

### Automatic IPv4 semantics are preserved

This repair does **not** globally disable DHCP and does not affect physical Wi-Fi/Ethernet DHCP configuration. The intended model is:

```text
physical Wi-Fi/Ethernet
  -> keeps normal LAN DHCP / automatic IPv4

WBD-owned Wintun
  -> receives Logical Tunnel IPv4 automatically from authenticated WBD server lease
  -> Windows DHCP/APIPA disabled only on this virtual interface
  -> only the WBD-issued lease may remain on the adapter
```

If no authenticated WBD lease is available, the tunnel must fail closed rather than silently continue using an APIPA address.

No server anti-spoof, FakeTCP, Reality-like TLS, DTLS, LINK, Game wire or FEC wire semantic was changed by this repair.

## 7. Current qualification matrix

| Boundary | Strongest proven source | State |
| --- | --- | --- |
| Windows raw FakeTCP SYN/SYNACK on real Npcap/public path | `be760709...` and later lineage | physically proven |
| same-association Reality-like TLS bootstrap | `be760709...` | physically/capture proven |
| DTLS 1.3 then LINK on the same public lineage | `be760709...` | physically/capture proven |
| required Game child packaged/embedded/extracted | `cf8298f9...` | hosted Windows build proven |
| Game child starts in one-lane topology | `cf8298f9...` | physically proven |
| Windows TUN ready / lease installed | `cf8298f9...` | physically proven, but old run allowed wrong extra/APIPA source |
| server Game/backend/shared-TUN session | `cf8298f9...` | physically proven |
| strict lease-source anti-spoof | `cf8298f9...` | physically proven to reject wrong APIPA source |
| WBD Wintun exclusive lease + non-IPv4 local fail-closed | `2e44c407...` | implemented; fresh physical proof pending |
| shared raw-IP RX/TX with correct leased source | none after `2e44...` | pending |
| route-fenced DNS application traffic | none after `2e44...` | pending |
| route-fenced generic UDP application traffic | none after `2e44...` | pending |
| route-fenced TCP application traffic | none after `2e44...` | pending |
| release-qualified Windows 11 + Npcap -> Ubuntu ARM64 product path | none | **NOT RELEASE-QUALIFIED** |

Do not combine the physically proven `cf8298...` backend reachability with the unphysically-tested `2e44...` Wintun repair and call the latter physically green. The next run must establish fresh evidence on one exact candidate source.

## 8. Current delivery/test procedure

The user's requested sequencing is:

1. run/collect the normal hosted tests and Windows/Linux producer builds for the repaired candidate;
2. provide compiled Windows x64 portable and Ubuntu ARM64 server packages once that exact-source hosted build is green;
3. user performs the physical Windows 11 + Npcap -> Ubuntu ARM64 test;
4. analyze raw support/journal/pcap evidence and only then decide release qualification.

Do not require physical qualification merely to hand over a **test candidate**. Do require physical qualification before any `RELEASE-QUALIFIED` statement.

The next physical run should keep the already proven deployment fences unless the live environment intentionally changes:

- public WBD endpoint: `:443`;
- internal LINK: `127.0.0.1:47010`;
- Normal lane count: 1;
- FEC: OFF for the primary functional run;
- front/raw public endpoint remains the same endpoint for the per-lane same-flow bootstrap.

Acceptance evidence should include, at minimum:

1. `WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE` with the actual authenticated lease;
2. no WBD-owned Wintun `169.254.*` address/source leakage;
3. `WBD_GAME_LANE_CLIENT_READY` and `WBD_TUN_READY`;
4. server `WBD_LINK_MUX_BACKEND_READY`, `WBD_GAME_LANE_SESSION_OPEN`, `WBD_SHARED_TUN_SESSION_READY`;
5. first correct leased-source shared raw-IP RX and TX evidence;
6. route-fenced DNS success;
7. route-fenced generic UDP success;
8. route-fenced TCP success;
9. deterministic disconnect/cleanup evidence;
10. exact source/package/evidence hashes so no mixed-SHA conclusion is possible.

## 9. Explicit non-actions

Until new deterministic evidence says otherwise, do not:

- bypass Game for `Lanes=1`;
- weaken source anti-spoof to accept `169.254.*` or arbitrary source IPv4;
- treat non-IPv4 as valid raw IPv4 backend traffic;
- redesign FakeTCP recovery;
- introduce a preliminary ordinary kernel-TCP WBD connection;
- alter Reality-like bootstrap semantics;
- replace pinned wolfSSL DTLS 1.3;
- change LINK wire framing;
- retune/redefine FEC during this functional repair;
- resurrect global one-lane ADR-0014 semantics;
- relabel local SourceIP/default-route observation as direct public NAT mapping detection;
- claim a public `:443` pcap alone proves Wintun/shared-TUN/application E2E;
- call hosted-green artifacts `RELEASE-QUALIFIED` before the fresh physical application run passes.

## 10. Recovery cursor after this document

The runtime repair source to qualify is `2e44c407eee677252897f2c75942407687ff8450`. A later docs/handoff-only branch HEAD is metadata, not retroactive runtime qualification.

Before any next code or packaging write:

1. refresh PR #9 live HEAD;
2. compare it to the substantive checkpoint recorded in `.wbd/handoff/current.json`;
3. inspect current Actions instead of assuming any run launched earlier has completed;
4. preserve exact-SHA separation between hosted build candidate and physical evidence;
5. continue from the live handoff's `next_atomic_action`.
