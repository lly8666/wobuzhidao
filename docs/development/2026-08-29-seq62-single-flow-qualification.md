# Sequence 62 — Single-flow qualification and capture-harness repair

Date: 2026-08-29

This document is a durable continuation of `docs/development/SINGLE_FLOW_DEVLOG.md`. It records the exact development work performed after handoff sequence 61 so chat recovery is not required to reconstruct the reasoning.

## User boundary for this sequence

The existing TCP-like/FakeTCP data plane is considered mature and should not be changed without deterministic evidence. The active architectural requirement remains:

- one public TCP-shaped 4-tuple for the full WBD session;
- one SYN lineage;
- FakeTCP owns the public sequence space from SYN onward;
- the first seconds carry real TLS 1.3 / Reality-like admission over a bounded reliable bootstrap stream inside that same FakeTCP association;
- no FIN/RST/new SYN at the setup-to-data transition;
- steady-state pinned wolfSSL DTLS 1.3 + LINK/FEC returns to datagram semantics and must not inherit ordinary kernel-TCP HOL.

This sequence deliberately changed qualification harnesses and project status documentation only. It did **not** change FakeTCP sender, receiver, shadow recovery, sequence semantics, FEC, DTLS wire format, or LINK wire format.

## Starting state

Canonical formal branch at the start of this work:

- `dev/wbd-raw-fec-v2`
- head `0563996ae982fd68e339cde6ec5ffedc3b1373d7`
- handoff sequence 61 was present and valid.

Active implementation:

- branch `feat/single-flow-reality-faketcp`
- draft PR #9 `Single-flow Reality-like bootstrap over FakeTCP`
- active head at refresh: `99870504ae39ae455299ec581707acf0dc0867f7`.

The branch already contained the important mature components:

- FakeTCP-owned public flow from the first SYN;
- temporary reliable bootstrap stream;
- same-flow TLS 1.3 / Reality-like admission and ticket output;
- same association handed to DTLS after the mode barrier;
- post-bootstrap no-HOL unit coverage;
- Windows `StartAfterFakeTCP()` readiness chain;
- Npcap 1.88 `MODE_SENDTORX_CLEAR=0x0200` fail-fast behavior;
- DTLS child-side blocking and handshake stage markers.

## Qualification failure 1 — single-flow E2E pcap was header-only

Workflow/run before the fix:

- `single-flow-e2e` run `33236623561`
- job `99058615140`
- evidence artifact `9710118315`.

The transport itself completed successfully before the workflow failed. Artifact inspection showed:

Client FakeTCP:

- `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1`
- `READY role=client ... single_flow_bootstrap=true`.

Server mux/DTLS:

- matching `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1`;
- inherited DTLS worker `BOUND`;
- `WBD_DTLS_SERVER_PEEK bytes=190`;
- `WBD_DTLS_SERVER_PEER_SET`;
- `WBD_DTLS_SERVER_HRR_ARMED`;
- `WBD_DTLS_SERVER_ACCEPT_START`;
- `WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3`;
- `READY role=server version=DTLSv1.3`.

DTLS client:

- `WBD_DTLS_CLIENT_CONNECT_START`;
- `WBD_DTLS_CLIENT_CONNECT_PASS version=DTLSv1.3`;
- `READY role=client version=DTLSv1.3`.

Probe:

- `SINGLE_FLOW_ECHO_PASS count=20`.

But tcpdump reported:

- `123 packets received by filter`;
- `0 packets captured`;
- `0 packets dropped by kernel`.

The pcap file was exactly 24 bytes, i.e. only the file header. Therefore the failure was a capture-drain race after the data plane had already passed, not a single-flow protocol failure.

### Fix

Commit:

- `fd2fbc10e5ce8225d8cbf47b7e1bb3990095dbaf`
- `test: drain single-flow capture before SYN proof`.

Changes were confined to `.github/workflows/single-flow-e2e.yml`:

- run tcpdump with `--immediate-mode -U`;
- retain explicit `listening on vc` readiness;
- after successful TLS/DTLS/echo, poll until the pcap size exceeds the 24-byte global header before sending SIGINT;
- only then parse the capture and enforce the one-SYN/one-4-tuple invariant.

No transport implementation changed.

## Qualification failure 2 — 20% pcap test missed the initial handshake

Workflow/run before the fix:

- `faketcp-pcap-20loss` run `33236623596`
- job `99058615201`.

The data/recovery evidence was healthy:

- first-arrival delivery ratio about `0.8175` under the staged 20% random loss window;
- p50 about `10.149 ms`;
- p75 about `10.157 ms`;
- client `Enqueued=800`;
- `FastRetransmits=21`;
- `RTOTransmits=3`;
- `SACKed=625`;
- client capture reported 1373 packets;
- server capture reported 1311 packets.

The only failing assertion was the pcap analyzer's `3-way handshake not visible`. The workflow launched tcpdump and used a fixed `sleep .1` before starting FakeTCP. It did not prove that both capture processes had actually attached before the SYN.

### Fix

Commit:

- `bfd9fa31382b07d35a00f2e7d9f0ae4712f822b9`
- `test: wait for pcap capture readiness`.

Changes were confined to `.github/workflows/faketcp-pcap-20loss.yml`:

- use tcpdump `--immediate-mode -U`;
- wait until client log contains `listening on c0`;
- wait until server log contains `listening on s0`;
- assert both capture points are ready before starting the FakeTCP server/client handshake;
- keep all existing 20% loss, first-arrival, SACK, fast retransmit and RTO assertions unchanged.

Again, no FakeTCP recovery algorithm was modified.

## Successful qualification after the harness fixes

At active implementation head `fd2fbc10e5ce8225d8cbf47b7e1bb3990095dbaf`:

### single-flow-e2e

Run:

- `33238806153`
- job `99064445216`
- result: **SUCCESS**.

The main network step `One SYN lineage carries TLS bootstrap then DTLS data` completed successfully.

The workflow emitted the decisive packet-lineage proof:

`SINGLE_FLOW_PUBLIC_INVARIANT_PASS unique_syn_seq=1 tuple=10.88.0.2:41001-10.88.0.1:443`

It then showed the complete same-flow progression:

- client `WBD_SINGLE_FLOW_BOOTSTRAP_READY tls=304 server_name=wbd.test same_flow=1`;
- client FakeTCP `READY ... single_flow_bootstrap=true`;
- server `WBD_SINGLE_FLOW_BOOTSTRAP_READY remote=10.88.0.2:41001 server_name=wbd.test same_flow=1`;
- inherited server DTLS worker `BOUND`;
- server `PEEK -> PEER_SET -> HRR_ARMED -> ACCEPT_START -> ACCEPT_PASS`;
- server `READY role=server version=DTLSv1.3`;
- client `CONNECT_START -> CONNECT_PASS`;
- client `READY role=client version=DTLSv1.3`;
- `SINGLE_FLOW_ECHO_PASS count=20`.

The test also asserts that there is no second client source port/4-tuple to public port 443 and that the original FakeTCP client and server mux processes remain alive through the mode switch.

This is the first automated pcap-backed qualification checkpoint that directly proves the user's core architecture invariant: one public SYN lineage carries the TLS/Reality-like bootstrap and then DTLS data without opening a second public connection.

### faketcp-pcap-20loss

Run:

- `33238806287`
- result: **SUCCESS**.

All original recovery and wire assertions remained intact. The success after capture-readiness repair confirms that the previous red status was a test harness race, not evidence requiring a TCP-like/recovery change.

### Other refreshed gates at this checkpoint

Confirmed success on the same implementation head included:

- `ci`;
- `windows-dtls-build`;
- `windows-ipv6-killswitch`;
- `linux-shared-port`;
- `linux-server-settings`;
- `faketcp-native`;
- `openwrt-tcp-tproxy`.

`mux-load-100m` had already returned to SUCCESS on the immediately preceding single-flow head, so the earlier sequence-61 RTT100 60/80M snapshot is historical rather than the current branch-wide status. The conservative release target remains the 40 Mbit aggregate-inner point; do not alter recovery simply to optimize optional 60/80M headroom without fresh evidence.

Some package/fullstack workflows were still queued/in progress at the instant this log was written and therefore are not called successful here.

## Windows runtime audit

The active PR #9 Windows runtime was re-read after single-flow qualification.

`Controller.Connect()` now:

1. validates/preflights;
2. discovers the physical underlay before opening the public session;
3. starts a single `wbd-faketcp` process containing the Reality-like bootstrap parameters;
4. waits for the same-flow ticket file produced only after TLS/admission succeeds;
5. builds the remaining plan around that same FakeTCP process;
6. hands the already-running process to `Executor.StartAfterFakeTCP()`.

`Executor.StartAfterFakeTCP()` then enforces:

- FakeTCP READY;
- DTLS READY;
- LINK `WBD_LINK_READY`;
- TUN `WBD_TUN_READY`;
- IPv6 kill-switch;
- route apply.

Thus the old separate public Reality process is no longer part of product Windows startup and routes are not applied before the single-flow transport stack is ready.

Npcap audit confirmed that the active branch retains:

- `npcapModeSendToRxClear = 0x0200`;
- required `pcap_setmode` export;
- fail-fast if `MODE_SENDTORX_CLEAR` cannot be applied;
- `WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared`;
- raw payload TX/RX boundary markers and kernel-RST observation.

No further Windows transport change was justified by the current evidence.

## Architecture-document audit

`ARCHITECTURE.md`, `PROJECT_CONSTITUTION.md`, and `README.md` on PR #9 already express the correct architecture:

- one public FakeTCP flow;
- one SYN lineage;
- TLS/Reality-like bootstrap is the first payload phase of the FakeTCP association;
- no parallel kernel TCP Reality listener as WBD admission owner;
- no second SYN after admission;
- temporary ordered setup semantics only;
- DTLS/datagram no-HOL steady-state.

One stale conflict was found in `ROADMAP.md`: SF1 still said `IN IMPLEMENTATION` and SF2 still said `NEXT` even though the implementation and dedicated E2E gate had passed.

Commit:

- `93281d0a88452cdebecae84b54187cee4f3474db`
- `docs: advance single-flow bootstrap milestones`.

The roadmap now records:

- SF1 implemented + no-HOL boundary unit-qualified;
- SF2 implemented + single-flow E2E qualified;
- SF3 fingerprint/fallback resemblance as the next architecture-facing gate;
- native pcap qualification green on the current single-flow line.

## Current development boundary

Do not reopen the already-qualified TCP-like data plane merely because there is more work to do.

Remaining work should focus on the surfaces that are actually not yet release-qualified:

1. Reality-likeness/fingerprint/fallback qualification (SF3): SYN/TLS fingerprint measurement and unrecognized-ClientHello decoy behavior;
2. shared-account two-client fan-out requalification using single-flow bootstrap for both clients;
3. final current-head load/release package completion;
4. Windows physical one-shot on Npcap/TUN using the single-flow build;
5. OpenWrt/Linux physical one-shot and cleanup;
6. secondary Windows BOM/Stop-idempotency cleanup;
7. secret-on-argv hardening on Linux.

The single-flow wire itself should be changed only if one of those focused gates produces deterministic evidence that requires a protocol change.
