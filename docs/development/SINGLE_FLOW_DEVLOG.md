# WBD Single-Flow Development Log

This file is the durable repository-side history for the single-public-flow redesign. It exists because chat recovery can truncate long development sessions. It records decisions, experiments, failures, fixes, qualification evidence, and abandoned approaches. The canonical handoff remains `.wbd/handoff/current.json`; this file supplies the detailed history behind that concise checkpoint.

## Non-negotiable product invariant

As clarified by the user on 2026-08-29, one WBD VPN session must expose exactly one TCP-shaped public flow for its full lifetime:

- one public 4-tuple;
- one SYN/SYNACK/ACK lineage;
- FakeTCP owns public TCP-shaped sequence space from the initial SYN;
- the first seconds carry a Reality-like / TLS-1.3-like authenticated bootstrap on that same association;
- the setup-to-data transition does not FIN, close, reconnect, or create another SYN;
- after the transition, the same association carries pinned wolfSSL DTLS 1.3, immutable LINK, and optional FEC;
- sustained VPN payload must not inherit ordinary kernel-TCP reliable-stream head-of-line blocking.

A separate kernel-TCP Reality admission connection followed by a second FakeTCP connection is therefore retired architecture, even if the two are linked by a ticket.

## Historical baseline before the architecture correction

The project originally treated the Reality-like front as setup-only admission and FakeTCP as a separate sustained data connection. Windows ran the Reality-like bootstrap synchronously, read a one-time ticket, closed that connection, then started FakeTCP/DTLS/LINK/TUN. Linux simultaneously exposed a kernel TCP Reality-like listener and a raw FakeTCP listener on the same public port.

This design preserved the important rule that sustained VPN data never ran inside ordinary TCP, but it created two unrelated public flows. NAT, conntrack and DPI saw two connections rather than one continuous connection. That contradicted the user's original requirement once it was restated explicitly.

## Linux/shared-port and early physical qualification

The Linux server path was made self-contained for amd64/arm64 and validated on a real Ubuntu ARM64 host. Important fixes included resolving wildcard raw-listen addresses to a concrete local IPv4 for FakeTCP and making release checksum generation portable.

A physical server was observed running:

- Reality-like front on public 443;
- FakeTCP raw listener on the concrete server IPv4:443;
- LINK mux on loopback;
- platform proxy on loopback.

A secondary security issue was discovered: process argv shown by systemd could expose Reality authentication material. Secrets must not be repeated in logs; moving credentials off argv remains a later hardening task.

## FakeTCP worker-start A/B/C/D experiments

A major RTT100 failure was initially misdiagnosed as an inherited nonblocking fd problem. Exact code/log inspection corrected the root cause: the old mux started a per-session wolfSSL DTLS worker synchronously while handling SYN, so SYNACK latency was coupled to process creation and a worker that could already be waiting for data while the test harness was still waiting for FakeTCP readiness.

Experiments:

### A — lazy worker after final ACK

- SYN created only a lightweight association.
- SYNACK was sent immediately.
- worker creation moved to valid final ACK.
- a data-bearing final ACK was allowed to fall through into normal segment processing.

Network behavior improved, but a SYN that never completed ACK could occupy the session table indefinitely because no worker timeout existed yet.

### B — SYNACK before worker startup

- SYNACK was emitted immediately;
- worker still started for every half-open SYN afterward.

This was stable but resource-expensive under half-open traffic.

### C — worker on first established payload

This avoided spawning a worker for pure final ACK but created another leak shape: an ACK-only established association with no payload could occupy the table indefinitely. RTT100 also exposed a client `interrupted system call` failure.

### D — final-ACK worker + half-open expiry + EINTR robustness

D became the best version of the old dual-flow mux behavior:

- lightweight association on SYN;
- immediate SYNACK;
- worker after valid final ACK;
- data-bearing final ACK falls through and preserves payload;
- half-open expiry after 25 seconds;
- stale timer/worker cleanup uses expected-session pointer matching so an old goroutine cannot delete a new incarnation that reused a tuple;
- Linux raw receives transparently retry EINTR;
- server mux raw receive also retries EINTR;
- relay/worker teardown became nil-safe.

D passed repeated RTT100 qualification and the 40 Mbit release point. These low-level robustness lessons remain useful in the single-flow architecture even though D itself still assumed a separate Reality setup connection.

## Windows readiness/lifecycle work

A real Windows self-test showed the old runtime launching FakeTCP, DTLS, LINK and TUN almost simultaneously. DTLS could start hundreds of milliseconds before FakeTCP was actually ready. Routes and the IPv6 kill-switch could be applied before LINK was usable, causing a false `connect_pass` followed by DNS/UDP/TCP timeouts.

The validated lifecycle order became:

1. FakeTCP READY;
2. DTLS READY;
3. LINK `WBD_LINK_READY`;
4. TUN `WBD_TUN_READY`;
5. IPv6 kill-switch;
6. route apply;
7. only then connected/pass.

This readiness design must remain in the single-flow product. Current PR #9 has `Executor.StartAfterFakeTCP()` and per-process readiness markers, so the latest branch has already restored this lesson.

Secondary Windows lifecycle issues remain:

- PowerShell 5.1 UTF-8 BOM can make route-state JSON fail Go `json.Unmarshal` with `invalid character 'ï'`;
- stopping an already exited LINK child can report `TerminateProcess: Access is denied`; Stop should be idempotent.

## DTLS certificate/inherited-worker sandbox

To stop using the physical machine as the primary debugger, a Linux namespace sandbox was built with:

- a temporary CA;
- SAN `wbd.test` service certificate;
- the same pinned wolfSSL build;
- real `wbd_dtls_shim`;
- FakeTCP mux;
- inherited UDP worker fd;
- two simultaneous clients;
- CA + hostname verification;
- bidirectional UDP echo.

This ran successfully and observed full DTLS 1.3 server stages. Therefore the Linux inherited-worker, pinned wolfSSL, certificate verification, and bidirectional DTLS payload path are viable.

## NAT/RST investigation

A NAT namespace A/B test confirmed that a host kernel can generate RST packets for raw TCP-shaped traffic. RST allow and RST suppression variants both completed FakeTCP + DTLS in that harness, so a blanket Windows firewall RST guard was not justified as the root-cause fix.

A real Npcap bug was found independently: code used `pcap_setmode(handle, 0)` while claiming it cleared SendToRx behavior. In Npcap 1.88, `MODE_SENDTORX_CLEAR` is `0x0200`. The corrected behavior is to require the export, call mode `0x0200`, fail fast on error, and emit `WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared`. Current PR #9 contains this fix.

## Real physical evidence before the architecture pivot

At least one Windows -> Ubuntu ARM64 run progressed through:

- Reality authentication;
- FakeTCP READY;
- DTLS server PEEK/HRR/ACCEPT PASS;
- DTLS server READY;
- `WBD_LINK_MUX_SESSION_READY`.

Later rapid attempts often showed Windows FakeTCP READY and DTLS CONNECT_START while the server never spawned/reached the per-session DTLS worker. This proved the lower layers could interoperate physically, but it also motivated increasingly faithful NAT/shared-port sandboxes.

## Shared kernel TCP 443 + raw FakeTCP 443 reproduction

A stricter namespace/NAT harness added a real kernel TCP:443 listener at the same server port as raw FakeTCP. It reproduced the physical failure shape: FakeTCP client reported READY, DTLS client started, but server mux did not create the DTLS worker.

Experiments around tuple reuse, session reincarnation and SYNACK isolation were useful diagnostics, but the more important conclusion was architectural: the old design forced a kernel TCP state machine and a raw TCP-shaped state machine to coexist around the same WBD public port while the client itself created two independent public connections.

When the user restated the requirement that the entire WBD session must be one TCP-looking connection, continuing to optimize this dual-flow topology was rejected.

## Single-flow architecture pivot

The chosen architecture is not kernel-TCP takeover. Taking a fully established Windows/Linux kernel TCP socket and then attempting to steal its exact sequence/ack state for raw FakeTCP would be platform-specific and would risk leaving normal TCP retransmission/HOL semantics in control.

Instead, FakeTCP owns the public flow from SYN.

### Phase 1: bounded reliable bootstrap

FakeTCP exposes a temporary reliable ordered byte-stream adapter only for setup. A real TLS 1.3 / Reality-like marker and admission exchange run over this adapter. Small setup traffic may retransmit/order bytes because setup latency is bounded and short.

### In-band transition

After authentication, both sides remain on the same public tuple and FakeTCP incarnation. There is no FIN, close, RST or new SYN. Setup state is retired and the transport changes role in-band.

### Phase 2: no-HOL datagram data plane

Pinned wolfSSL DTLS 1.3 and immutable LINK/FEC operate on datagrams over the same FakeTCP association. Steady-state delivery must not wait for earlier missing outer segments merely to preserve a global byte stream. Unit tests include the requirement that later post-bootstrap datagrams remain deliverable across an earlier sequence hole.

## PR #9 implementation line

The active implementation branch is `feat/single-flow-reality-faketcp`, PR #9, titled `Single-flow Reality-like bootstrap over FakeTCP`.

By head `904ad0aa5db615c61ef12ed8b3c0b6f9c0fa3a6e`, it already contained:

- one FakeTCP-owned public flow from SYN;
- bounded reliable bootstrap stream;
- Reality-like TLS 1.3 bootstrap on the same association;
- same-flow ticket output;
- server mux bootstrap before inherited DTLS worker;
- Linux manager using one raw WBD public ingress for WBD sessions;
- Windows integration without a preliminary public Reality socket;
- architecture/constitution rewrite and ADR-0011;
- bootstrap/no-HOL unit tests;
- a dedicated `single-flow-e2e` workflow.

This branch, not PR #8, is the correct forward implementation line. PR #8 is historical/cherry-pick material only.

## Single-flow E2E qualification evolution

Early `single-flow-e2e` failures were harness failures rather than product transport failures.

### Ticket permission failure

The FakeTCP client ran as root inside a namespace and correctly wrote the ticket as root-owned mode 0600. The workflow attempted to read/upload it as the normal runner user. The fix was to validate ticket existence/length with sudo and not upload the secret ticket.

### Missing observability marker

At head `904ad0aa...`, evidence artifact 9709741957 showed:

- client `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1`;
- client FakeTCP READY with `single_flow_bootstrap=true`;
- server matching single-flow bootstrap READY;
- server `BOUND role=server`;
- server `READY role=server version=DTLSv1.3`;
- client `READY role=client version=DTLSv1.3`;
- a non-empty public capture.

The workflow still failed because it waited for the mature marker `WBD_DTLS_SERVER_ACCEPT_PASS`, which the branch's older shim did not emit. This was an observability regression, not evidence that the same-flow TLS->DTLS transition failed.

### Current qualification fixes, 2026-08-29

Two isolated commits were made after sequence-61 handoff:

- `c8926b44b1632a7bfd9740a6dc20ae513f48c3b0` — reuse the already validated PR #8 DTLS shim blob, restoring child-side blocking and CONNECT/PEEK/PEER_SET/HRR/ACCEPT stage markers without changing wire behavior.
- `0e35c237356437e6d0db11c15ab9e2c24e7fed61` — make `single-flow-e2e` wait until tcpdump reports `listening on vc` before starting the server mux/public flow, preventing the only SYN and early TLS packets from racing capture attachment.

The branch is intentionally frozen on this head while the new qualification run executes.

## Current Windows state at PR #9 latest audit

The latest branch already contains the mature items that older notes once described as missing:

- `Executor.StartAfterFakeTCP()` waits for the prestarted single-flow FakeTCP process to emit readiness;
- DTLS, LINK and TUN are then started and individually waited on;
- IPv6 and routes are not mutated until all child layers are ready;
- Npcap uses `MODE_SENDTORX_CLEAR=0x0200` and fails fast if the mode cannot be applied.

Do not re-port these a second time.

## RTT100 mux-load status at head 904ad0aa

This is a separate high-load issue and must not be conflated with the single-flow E2E gate.

Observed bench(100):

- 40 Mbit/s off: roughly 80.4% delivery / ~32.17 Mbit/s goodput;
- 40 Mbit/s FEC20:20: 100% delivery / ~39.994 Mbit/s goodput;
- 60 Mbit/s off: roughly 80.18% delivery / ~48.11 Mbit/s;
- 60 Mbit/s FEC20:20: roughly 75.304% delivery / ~45.18 Mbit/s in that run;
- 80 Mbit/s off: roughly 59.0% delivery / ~47.20 Mbit/s;
- 80 Mbit/s FEC20:20: failed before benchmark because a second FakeTCP client did not reach READY within the workflow deadline.

The release target remains 40 Mbit/s aggregate-inner on <=100 Mbit/s weak links. The 40M+FEC release point still passed. 60/80M behavior is headroom regression and will be debugged after the single-flow qualification gate becomes truthful and green. Do not weaken the 40M release invariant and do not change recovery algorithms merely to hide the 60/80 result.

## Durable-work rules

For future development sessions:

1. Refresh canonical handoff, active PR head, and current Actions before editing.
2. Read this file and the Drive document `WBD 开发历程与实时结论` if chat history is incomplete.
3. Append deterministic failures, conclusions, commits and qualification evidence here/Drive as work proceeds.
4. Do not call a queued or in-progress workflow successful.
5. Do not hand a new Windows/Linux package to the user until sandbox/integration qualification is green.
6. Before a development turn ends, update Google Drive and write the next canonical GitHub handoff sequence.

## Next action at time of this entry

Wait for the `0e35c237...` single-flow E2E run. If it passes, preserve that evidence and move to the RTT100 headroom investigation. If it fails, inspect the first deterministic failure and artifact before changing code. Do not change the single-flow wire unless the evidence specifically requires it.
