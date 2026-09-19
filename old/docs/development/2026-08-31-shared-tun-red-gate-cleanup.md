# 2026-08-31 shared-TUN exact-head red-gate cleanup

## Scope

This log continues `2026-08-31-current-head-requalification.md` after feature head `fa353a98868f3e8dc246c3e80bbb6a6ec6b3702f` completed its first shared-TUN qualification wave.

Frozen product constraints remain unchanged:

- one public TCP-shaped 4-tuple, one FakeTCP sequence space and one SYN lineage per Transport Lane;
- Reality-like TLS 1.3 admission is the first payload phase of that same FakeTCP association;
- no second public SYN/FIN/RST transition at the admission -> DTLS switch within a lane;
- sustained transport is pinned wolfSSL DTLS 1.3 -> LINK -> lane-local FEC/raw-IP datagrams and must not inherit ordinary kernel-TCP stream HOL;
- mature FakeTCP recovery/FEC/TCP-like data-plane code is not changed by this cleanup;
- server raw-IP egress architecture is shared TUN + one host NAT, not per-session network namespaces.

## Exact-head Actions at `fa353a9...`

Green evidence included:

- `shared-tun-two-client` run `33350859521`;
- `single-flow-e2e` run `33350859502`;
- `single-flow-no-hol` run `33350859498`;
- `single-flow-tcp-persona` run `33350859516`;
- `faketcp-native` run `33350859528`;
- `faketcp-first-arrival` run `33350859439`;
- `faketcp-pcap-20loss` run `33350859578`;
- `fullstack-first-arrival` run `33350859480`.

Red evidence was investigated rather than ignored.

## Finding A — old `single-flow-two-client` gate was exercising a retired server topology

Run `33350859479`, job `99363816093`, produced useful evidence before failing:

- both same-account clients completed Single-Flow V2 Reality-like bootstrap;
- two distinct TunnelIDs were issued;
- two distinct `/32` leases were issued;
- both pinned wolfSSL DTLS 1.3 clients reached READY;
- both LINK clients reached READY and both tickets were consumed;
- the server created two raw-IP backends.

The artifact then showed one client probe succeeding while the other timed out. Server evidence included `WBD_LINK_RAW_IP_SPOOF_DROP` and the old `wbd-ip-gateway-server` creating one network namespace/TUN/NAT per session.

That topology is no longer the selected V2.4 architecture. The new `shared-tun-two-client` workflow on the exact same source head already proved the current invariant: two independent logical-tunnel leases share one server TUN and one host NAT while preserving lease/source isolation and same-inner-source-port traffic.

Decision: retire `.github/workflows/single-flow-two-client.yml` instead of weakening spoof protection or modifying mature transport code. `shared-tun-two-client` is the current two-client release qualification.

Commit: `2b77122840dfc45937018fdc87a5ff868c3aa7f1` (`ci: retire superseded per-session two-client gate`).

## Finding B — main CI and handoff-verify were red only because ADR-0010 lost an exact hard-constraint sentence

Main CI run `33350859475` completed `go test ./... -count=1` successfully. Handoff verifier itself also printed `HANDOFF_VERIFY_PASS`. The only failing Python contract was:

```text
assert "No second public SYN is permitted" in ADR-0010
```

ADR-0010 already described one FakeTCP SYN lineage and the same 4-tuple/sequence space per lane, but a later wording edit no longer contained that exact sentence.

Because the product requirement is still correct, the test is not weakened. ADR-0010 now explicitly states:

> No second public SYN is permitted for the same Transport Lane.

Commit: `17ced8cc020e5eb6b643691af55ee315198ef3e4` (`docs: restate per-lane no-second-SYN invariant`).

## Finding C — `linux-server-firewall` was a workflow-definition/dispatch red, not a firewall execution failure

Run `33350858560` on `fa353a9...` completed immediately with failure and zero jobs. Its display name degraded to the workflow path, and no firewall step executed. The old file also duplicated Linux release-bundle construction that already belongs to the dedicated `linux-server-release` workflow.

The firewall qualification has therefore been reduced to its actual responsibility:

- nftables WBD-owned input/RST rules apply and clean without deleting existing host policy;
- iptables WBD-owned input/RST rules apply and clean without deleting existing host policy;
- `linux_server_guard.sh` always performs automatic WBD-owned cleanup;
- it runs explicitly on both `dev/wbd-raw-fec-v2` and the active single-flow feature branch when firewall/guard code changes;
- Linux bundle construction remains solely in `linux-server-release`.

Commit: `91303b244094977d0d715019b20301d4c826a0c2` (`ci: isolate Linux firewall ownership qualification`).

## Delivery rule after this cleanup

No package is qualified by these documentation/CI changes alone. A later exact source HEAD must have current green evidence for:

1. main CI + handoff contract;
2. single-flow one-SYN / Reality-like / no-HOL gates;
3. shared-TUN two-client isolation;
4. Linux firewall ownership and Linux server release;
5. hosted Windows runtime/bundle qualification;
6. a combined Windows-runtime -> Linux-server qualification that uses the actual state machines and is explicit about any Npcap/privileged-network limitation.

Only then may Windows/Linux artifacts be offered for physical acceptance.
