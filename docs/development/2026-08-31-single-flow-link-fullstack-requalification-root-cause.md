# 2026-08-31 current-head single-flow LINK requalification root cause

## Scope

This note records the first deterministic failure found while requalifying PR #9 at source HEAD `898f627ce041b10bf858f3824685d09dcd0cc7ef` after the Logical Tunnel / raw-IP changes.

Hard constraints remain unchanged:

- one public TCP-shaped FakeTCP association per Transport Lane;
- the bounded Reality-like TLS setup runs on that same association;
- the same association switches in-band to DTLS/LINK/FEC without FIN/RST/new SYN;
- sustained payload must not inherit ordinary kernel-TCP HOL;
- the mature FakeTCP recovery/ACK/SACK/FEC core stays frozen unless deterministic evidence isolates a core defect;
- no Windows/Linux candidate is deliverable while the exact same source HEAD has a red qualification gate.

## Failing qualification

Workflow: `single-flow-link-fullstack`

Run: `33324659186`

Matrix failures:

- FEC `off`: job `99292556121`, artifact `single-flow-link-off` (`9735895565`)
- FEC `20:20`: job `99292556034`, artifact `single-flow-link-20x20` (`9735897562`)

Both jobs passed checkout, dependency install, pinned wolfSSL build, WBD binary build and focused Go tests. Both failed in `One public flow reaches LINK service`.

## Artifact evidence

The two artifacts fail identically before any public packet is emitted.

`faketcp.log` in both matrix cases contains only:

```text
wbd-faketcp: single-flow bootstrap requires reality-server-name, route-key >=16 bytes, username/password, ticket-out, installation-id, tunnel-config-out and positive timeout
```

The server-side processes were ready:

```text
READY role=server-mux ... single_flow_bootstrap=true ... logical_tunnel=true
WBD_LINK_SERVER_MUX_READY ... logical_tunnel=1 ...
```

The packet capture proves that the client never entered the transport path:

```text
0 packets captured
0 packets received by filter
0 packets dropped by kernel
```

Therefore this is not a FakeTCP, FEC, DTLS or LINK data-plane regression. Those layers were never exercised by the failing jobs.

## Root cause

Logical Tunnel Phase 1 made two client bootstrap arguments mandatory:

- `--reality-installation-id`
- `--reality-tunnel-config-out`

`cmd/wbd-faketcp/main.go` validates both whenever single-flow bootstrap is enabled. `cmd/wbd-faketcp/bootstrap_v2.go` parses the installation identity, performs the V2 authenticated bootstrap, writes the one-time ticket and writes the authenticated Logical Tunnel config JSON.

`single-flow-link-fullstack.yml` was not updated when those CLI requirements landed. It still invokes `wbd-faketcp client` with the older ticket-only argument set, so the process exits during configuration validation.

## Fix direction

Update only the qualification harness first:

1. provide a deterministic valid 32-hex installation identity;
2. provide a per-run `--reality-tunnel-config-out` path;
3. require both the ticket and authenticated tunnel config files to exist after bootstrap;
4. parse the tunnel config as JSON so a truncated/invalid output cannot silently pass;
5. keep all FakeTCP recovery/FEC/data-plane code unchanged;
6. rerun both `off` and `20:20` matrix cases and inspect the next first failure, if any.

If the harness then reaches DTLS/LINK and exposes a product defect, fix that defect separately and record the evidence before touching transport-core behavior.

## Handoff continuity

Canonical machine-readable handoff was advanced to sequence 70 before development continued. Sequence 70 handoff commit is `3dd6eb5da03c6557ff83d46ee9841b782efca474`; `handoff-verify` run `33325769498` completed successfully.
