# P4 unified runtime transport candidate

Date: 2026-09-20  
Branch: `next/tlslike-dataplane`  
Archive source: `b5c848f4e9afdffd15d1bc451560edf4e9390a35`

## Recovered authority

Before editing, remote branch HEAD was `bde3ebda6ee6992d92fcbcee7591aa99f2fc6d66`. The docs-only closure run `35496541834` has completed with all seven jobs PASS. Product qualification remains the prior exact source `a8438872f948849ab378238a69dc1a1e28c86ac6` / run `35496415805`; the docs-only closure is not promoted to `last_tested_source_sha`.

## Atomic scope

This atom starts the P4 unified client/server runtime transport closure. It does not create a new wire protocol or restore old process topology.

- Add `internal/runtimeowner` as the single owner joining one leased `datapath.TunnelOwner` to per-lane post-admission FakeTCP transport state.
- Send existing `datapath.WireRecord.Wire` bytes directly as payload on the lane's existing TCP-shaped four-tuple/sequence space through a caller-supplied `faketcp.SegmentEmitter`.
- Route inbound steady payload to `TunnelOwner.InboundPayload` / `GameInboundPayload`, then to a direct packet sink such as shared-TUN, Wintun, or OpenWrt platform service handling.
- Keep business no-HOL: an out-of-order FakeTCP payload is passed immediately to the independent TLS-like record/FEC/LINK owner; FakeTCP cumulative sequence state is advisory repair state only.
- Bound post-bootstrap repair at 4096 outstanding records with a default 1s retry interval and 3s absolute first-send horizon. Retransmit is byte- and sequence-identical; horizon/capacity abandons repair rather than stopping fresh traffic.
- Own initial attach, same-ID replacement promote/fail/retire, DORMANT cleanup, and stale-generation handoff through the existing `TunnelOwner` lifecycle.
- Add the missing client admission -> `LaneConfig` handoff symmetric to the already-qualified server handoff.
- After P2 detach, keep bootstrap ACK validation only inside the bootstrap sender's sequence space; a larger steady ACK is left to runtime transport ownership rather than raising `ErrInvalidACK`.
- Expose `openwrtclient.SocketAdapter.DeliverFromOwner` so the reverse platform-service path is a public in-process call. The existing privileged OpenWrt test now uses it.

## Hosted contract added

`internal/runtimeowner/runtime_test.go` covers:

- Normal mode: drop record 1, deliver record 2 first without business HOL, retransmit record 1 with identical FakeTCP sequence/payload, then retire both on cumulative ACK.
- Same-ID replacement: promote both ends, reject traffic through the old generation, retire the old transport, and send through the fresh transport generation.
- Game mode: two already-existing lane transports carry the same tunnel PacketID; lane 2 may win and lane 1 is then suppressed by existing tunnel-wide Game dedupe without adding lanes.

`internal/faketcp/association_runtime_test.go` covers post-detach steady ACK/record ownership, and `internal/datapath/handoff_client_test.go` covers client-direction negotiated limits/MSS handoff.

## Explicit limits

This is the requested hosted in-process/loopback runtime integration step. `SegmentEmitter` is the transport-facing seam for the already-qualified Linux raw and Windows Npcap adapters, but this atom does not yet create the final client/server executable loops around those adapters. It also does not migrate the archived full SACK/RACK/adaptive-pressure steady recovery policy; the initial runtime uses cumulative ACK plus a bounded repair horizon. No claim of physical Windows/Npcap or P7 qualification is made.

No localhost UDP WBD bridge, DTLS shim, `wbd-platform-proxy-*` subprocess, or old Controller topology is restored.

## Qualification state

Candidate has not yet received exact-SHA Actions evidence. `docs/STATUS.json:last_tested_source_sha` intentionally remains `a8438872f948849ab378238a69dc1a1e28c86ac6` until this product candidate passes the full workflow.
