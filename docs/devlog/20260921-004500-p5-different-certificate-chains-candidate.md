# P5 different certificate chains candidate

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base docs HEAD: `4dd4354632cd32123159ee8a031815026185d3df`
- Current product qualification: `1fb65c90b272454e09a624eaa23e6a432de7955a` / Actions `35522367146` / 8/8 PASS
- Previous docs-only closure: `4dd4354632cd32123159ee8a031815026185d3df` / Actions `35522517629` / 8/8 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Atom scope

This atom covers only the P5 **different certificate chain** dimension while preserving the already-qualified full/resumed and sequential-close regression.

The controlled inner HTTPS sequence becomes:

1. chain A, TLS 1.3 full handshake;
2. fully close flow 1 and retire both client/server business flow state;
3. chain A, TLS 1.3 resumed handshake using the same explicit chain-A client-session cache;
4. fully close flow 2;
5. chain B, TLS 1.3 full handshake using an independent trust chain and independent client-session cache;
6. fully close flow 3.

All three inner flows must reuse the same already-established outer FakeTCP + protected TLS/admission Normal lane. The outer initial client SYN count remains exactly one.

## Independent controlled chains

The harness generates two independent chains with system cryptographic RNG:

- `chain-a`: Root CA -> Intermediate CA -> leaf `target.test`
- `chain-b`: independent Root CA -> independent Intermediate CA -> independent leaf `target.test`

Each test HTTPS server sends leaf + intermediate. The client trust store contains only that scenario's root CA, so a successful handshake requires real path building and verification.

After each handshake the harness requires exactly one verified chain of length 3 and compares the verified leaf/intermediate/root DER SHA-256 fingerprints against the scenario provenance. It also requires all three certificate identities to differ between chain A and chain B.

This is stronger than swapping two self-signed leaf certificates: the trust anchors and intermediate issuers are independently generated.

## Preserving the previous handshake atom

The first two flows keep the third P5 atom's full/resumed evidence:

- flow 1: chain A, `DidResume=false`, real session-ticket Put;
- flow 2: chain A, `DidResume=true`, real cache Hit.

Flow 3 uses a separate chain-B cache and must be `DidResume=false`; it must store its own session state but have zero resume hits.

Therefore the certificate-chain comparison uses full handshakes flow 1 versus flow 3, while the existing full/resumed regression remains active in the same dedicated P5 gate.

## Raw evidence extension

Each `https_flow` JSONL event additionally records:

- certificate scenario name;
- chain ID;
- certificate verification=true;
- verified chain length;
- verified root SHA-256;
- verified intermediate SHA-256;
- verified leaf SHA-256.

The manifest adds two certificate scenario records with chain IDs, fingerprints, verified chain length, server name, and flow ordinals. Session-cache provenance becomes per-chain as well as aggregate.

The raw PCAP, TLS-like record events, direction/timing/burst data, handshake/business latency, wire bytes, repair inventory, capture loss, network drop, FEC and padding accounting remain unchanged.

## Actions gate

No second measurement job is created. The existing dedicated `p5-https-measurement-base` gate is strengthened in place because it already:

- runs only the controlled P5 harness;
- validates artifacts independently against exact `GITHUB_SHA`;
- uploads raw PCAP / JSONL / manifest / summary / test log.

The validator now requires:

- exactly 3 HTTPS flows and 3 distinct business FlowIDs;
- handshake modes `[full,resumed,full]`;
- certificate chain IDs `[chain-a,chain-a,chain-b]`;
- two distinct controlled certificate scenario entries;
- real 3-level verified chains for every flow;
- matching per-flow and manifest certificate fingerprints;
- independent root/intermediate/leaf identities;
- preserved chain-A ticket Put + resume Hit;
- chain-B full handshake with no resume Hit;
- sequential close and one reused outer connection;
- FEC off, padding off and no network injection.

Expected marker:

`flows=3 ... handshakes=full,resumed,full certificate_chains=2`.

## Non-claims

- No sparse or natural-concurrency result.
- No weak-network, load or soak result.
- No FEC-on result.
- No classifier or indistinguishability claim.
- No production padding enablement; padding remains `0/off`.
- The controlled certificate chains are test provenance, not claims about reproducing a target website's server certificate chain or complete TLS fingerprint.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 remains `NOT_IMPLEMENTED`.
- No `old/` implementation is reused, so `docs/REUSE_LEDGER.json` is unchanged.
