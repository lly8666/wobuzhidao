# P5 controlled real HTTPS measurement base candidate

- Date: 2026-09-20
- Branch authority: `next/tlslike-dataplane`
- Base authority HEAD: `f66187c0bdf8632b94b485deab88a16a8800565d`
- Previous product qualification: P4 `76a20c865bf86edf2098f0ef40c44a31298679dd` / Actions `35510241639` / 7/7 PASS
- P4 docs-only closure run checked before work: Actions `35511252962` / 7/7 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Atom scope

Start P5 with only the minimum reproducible controlled real-HTTPS measurement base. This atom does not add a weak-network matrix, make performance conclusions, tune FEC, enable padding, build a classifier, or touch P7 Windows/Npcap physical qualification.

The harness uses the active production seams already present in the tree:

- `runtimeentry.DialClient` / `runtimeentry.Server` for the existing FakeTCP + outer TLS + protected admission + steady runtime handoff,
- the existing leased `TunnelOwner` / Normal lane,
- `platformflow.Client.AddTCP` and the server service dial path for the inner application flow,
- existing runtimeowner and TunnelOwner statistics for repair/padding inventory.

No implementation was copied from `old/`; `docs/REUSE_LEDGER.json` is therefore unchanged. Archive source SHA remains `b5c848f4e9afdffd15d1bc451560edf4e9390a35`.

## Controlled business path

The dedicated Actions test starts a real Go TLS HTTP server and drives a real TLS client + HTTP/1.1 request through the current formal runtime path. It opens two sequential inner TCP/HTTPS flows:

1. `first_https_flow_on_initial_outer_connection`
2. `subsequent_https_flow_existing_lane`

The test requires the outer lane ref to remain unchanged and requires exactly one client initial FakeTCP SYN across both flows. The second HTTPS flow therefore reuses the already-established outer association rather than creating a per-business-flow transport lane.

This is a hosted fullstack measurement seam: the client-side application socket is an in-process `net.Pipe` handed to the existing platformflow TCP adapter, while the server-side platformflow service performs a real TCP dial to the TLS HTTP target. It is not a kernel/NIC physical capture claim.

## Raw evidence schema

The harness writes `wbd-p5-https-measurement/v1` artifacts under the dedicated Actions artifact directory:

- `outer.pcap`: DLT_RAW IPv4/TCP packets serialized from the exact SegmentIO emit boundary,
- `events.jsonl`: raw outer-packet, steady TLS-like-record, and HTTPS-flow events,
- `manifest.json`: SOURCE_SHA/harness SHA, scenario, seed/RNG provenance, MTU/lane/FEC/padding/network-injection config, runner/toolchain, capture accounting, runtime transport and owner stats,
- `summary.json`: validator output,
- `test.log`: test marker/output.

Event fields include direction, monotonic timestamp, packet/record length, packet/record inter-arrival, burst id/bytes, HTTP handshake and business latency, and per-flow c2s/s2c wire bytes.

Loss/cost categories remain separate in the manifest: hosted capture loss, injected network drop, FEC recovery, repair retransmits, and padding bytes. For this first atom, network injection is none, FEC parity is 0, and production padding remains off/0.

The PCAP is explicitly labeled `hosted-segmentio-serialized-pcap`; its capture-loss value is zero because interception occurs synchronously at the in-process SegmentIO emit boundary and therefore does not represent a kernel capture queue measurement.

## Actions gate

`.github/workflows/next-foundation.yml` gains `p5-https-measurement-base`. The job runs only the controlled harness, validates its raw evidence with `tools/check_p5_measurement.py` against the exact `GITHUB_SHA`, and uploads the raw artifacts. Existing repository-contract, Windows/Linux tests/race/fuzz/reference, P2 kernel fallback, Linux shared-TUN privileged iptables/nft, and OpenWrt privileged jobs remain required in the same workflow.

Local Go unit/build/race/network results are deliberately not used as qualification authority. Product qualification remains PENDING until the exact candidate SHA is green in GitHub Actions.

## Explicit non-claims

- No weak-network performance conclusion.
- No load/soak result.
- No classifier or indistinguishability claim.
- No FEC parameter selection.
- Padding remains production default 0/off.
- Hosted SegmentIO PCAP is not a P7 physical NIC capture.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 TPROXY/capture remains `NOT_IMPLEMENTED`.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s absolute repair horizon remains unchanged.
