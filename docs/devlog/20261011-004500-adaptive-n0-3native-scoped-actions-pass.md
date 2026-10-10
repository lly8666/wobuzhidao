# N0 exact-SOURCE nine-netns three-real-client functional acceptance — scoped PASS

## Authority and source
- Current working branch: `next/adaptive-fec-aes-tun-20261010`; product SOURCE **`80487e6175c16cd6d689b82fca5d1053167cc1fa`**. The containing commit of this devlog is docs-only and is **not** a new product SOURCE.
- **[next-adaptive-n0-multi Action #38067388763](https://github.com/lly8666/wobuzhidao/actions/runs/38067388763): PASS**, one real functional job **114257695523**, evidence artifact **11674289902**, sha256 artifact `61c4c8f7b02ca43dad59347c44540a585cf12ed83a21a6f010ec1cd2be57d220`.
- **[next-foundation Action #38067388735](https://github.com/lly8666/wobuzhidao/actions/runs/38067388735): PASS** for exact `80487e6`. Linux/Windows build/Go regressions and P4 privileged baseline passed; this is not native Windows Wintun/physical evidence.
- Prior focused 3-client in-memory first S2C before C2S plus actual negotiation/TLS suite `-race -count=2` on `a50accc45ff7925f4f7488640dad86e76bee1767` [Action #38066837682](https://github.com/lly8666/wobuzhidao/actions/runs/38066837682): PASS. Earlier [#38066640082](https://github.com/lly8666/wobuzhidao/actions/runs/38066640082) FAIL shallow checkout remains preserved, repaired in a later exact SOURCE.
- Scope: one functional-only sample 8s, 10s drain, 0% configured loss, exactly 300ms one-way netem in each direction on router, 0.15Mbps **per direction per client**, 3 concurrently active formal Linux clients + one formal Linux server (shared port 443, shared TUN) across 9 real namespaces.

## Independently observed production-path state
| Client | Product policy actually negotiated on both ends | Active lane parity | Server-assigned unique /32 lease | Actual TLS1.3 handshake suite |
|---|---|---|---|---|
| #1 Normal off | V3, FECMode=0, record Cipher=ChaCha(1), Normal(1) | [0] | `10.66.54.103/32` | 4865 (0x1301) |
| #2 Normal fixed 20:4 | V3, FECMode=1, FixedParity=4, ChaCha(1) | [4] | `10.66.68.242/32` | 4865 (0x1301) |
| #3 Game2 fixed 20:20 | V3, FECMode=1, FixedParity=20, ChaCha(1), Game(2) | [20,20] | `10.66.19.168/32` | 4865 (0x1301) |

Both sides report same actual authenticated V3 policy, record version, TLS suite and independent unique lease/TunnelID for each client. TLS cipher suite **is not** the independent-record AEAD algorithm and no decoy cipher consistency qualification is claimed. 0x1301 is TLS_AES_128_GCM_SHA256 **outer TLS**; protected record AEAD still ChaCha20-Poly1305.

## Functional delivery and WAN/runner health
- **Each of 3 real clients, both directions**: sent **297 packets / 150,480 business bytes**; independent receiver first-unique delivered **297 / 150,480**. Across **six directions**: 1,782 first-unique packets and 902,880 uniquely delivered business bytes. Corruption=0, duplicates=0, local send failures=0 for all six. All 8 of 8 probes returned for each client; returned-only RTT p99 respectively **600,459,383ns**, **600,523,710ns**, **600,488,156ns** (not a performance qualification).
- First client's pre/post underlay capture pairing missing=0; actual one-way qdisc median measured **300,010,000ns** in each direction. All six captures had **0 kernel tcpdump drops**. The extra 2 client captures showed separate real FakeTCP SYN flows (client2=1, client3=2), each separate rcli netem qdisc active. First-client captured outer TCP payload repair=0 this single lossless sample (not evidence that real repairs are byte-identical). Hash then unlink each bounded pcap; no pcap uploaded. Runner CPU, quota/PSI, official binary sha256, original pinned source template sha256 and derived script hash retained in small artifact.
- Process liveness explicitly observed after the 8s traffic +10s drain for all three official clients and server. The independent checker emitted `WBD_ADAPTIVE_N0_MULTI_PASS_N0_3_NATIVE_CLIENTS_SCOPED` with `problems: []`. The workflow's final enforced result also PASS.

## Gaps and next action
- This is **NOT entire N0 acceptance**: the native Linux fixture *does not* prove first S2C user business arrives before any C2S user business; that property passed in the independent three-client in-memory real TLS/FakeTCP test but native TPROXY reply mapping requires business-origin mapping unless product changes. Never call in-memory carrier physical network.
- Same FakeTCP TCP Seq retransmission carrying **identical full ciphertext wire** needs a directed repair case and full payload capture or tightly scoped source-level invariant verification. Current tcpdump uses snaplen=96 and captured 0 external repair in this lossless sample, so cannot qualify that property.
- No N0 comprehensive load/capacity qualification; no N1 quality feedback, N2 auto FEC, N3 AES, N4 native Windows Wintun direct, N5 Chinese GUI, N6 integrated P6/packages, or physical hardware testing. Do not mark N0 DONE or ACTIONS_READY_FOR_PHYSICAL. All physical tests must wait for final all-features Actions+P6 completion per user governance.

## Changed in this docs-only commit
The unique `docs/STATUS.json` advances from multi-client PENDING to exact-SOURCE **scoped PASS**, records both PASS Actions without overriding earlier FAIL, keeps the legitimate open N0 gates and N1..N6 NOT_STARTED. `docs/evidence/adaptive-n0-3-native-80487e6-actions-20261011.json` stores concise keyed receipts. No product code modified in this docs-only commit, no local compilation, no deployment or user machine access.
