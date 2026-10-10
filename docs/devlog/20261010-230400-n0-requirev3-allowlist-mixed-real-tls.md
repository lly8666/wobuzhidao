# N0 V3 server required protocol, authenticated policy allow-list and 3-client covered real TLS

## Exact source and chain
Parent HEAD 2d5cea1c7add5d04d56c94bf98cf82a944b80d2a with tree 3fb4062b076c12e65ac212567b4fc78e122d8324, working branch next/adaptive-fec-aes-tun-20261010. Exact new SOURCE is this devlog's own commit SHA (Actions GITHUB_SHA); cannot self-reference it. Earlier source 5f60 foundation PASS, then parent source 2d5 foundation PASS run 38061700075 and lifecycle core PASS run 38061700072, both exact previous SOURCE, preserved in sole STATUS and evidence.

## Product changes
- ServerAdmissionConfig.RequireV3 when enabled rejects legacy V2 after authentication, before lease allocation, using explicit version failure; historical test-only V2 entrypoints remain available when RequireV3=false. Linux production server sets RequireV3=true, matched to Linux/Windows production V3 clients.
- Optional protected PolicyAllow callback enforces an operator application allow-list after auth + V3 SupportedNow and before lease allocation. Failure returns explicit unsupported and protected param-failure reply, never silently downgrades; tests ensure denied policies and older V2 do not consume a lease.
- New three-client deterministic real TLS/uTLS + FakeTCP association + runtimeowner + FEC + LINK + shared-TUN router memory-carrier test. Simultaneous independent Normal-off, Normal-20:4, Game-20:20 under shared credentials, per-Tunnel distinct leased IPv4 and same server port, server global parity intentionally wrong. Checks first upstream/downstream data and policy across client rotation. This is a functional in-memory segment carrier, NOT privileged netns and NOT native Windows TUN.
- Existing fixed/off timing, 32ms/3s FEC, late first-arrival and inherited V2 vectors untouched. N1 feedback/N2 auto/N3 AES/N4 Windows direct/N5 catalog UI/N6 P6 remain unimplemented.

## Actual parent SOURCE validation and limitations
- SOURCE 2d5cea1c7add5d04d56c94bf98cf82a944b80d2a: next-foundation run https://github.com/lly8666/wobuzhidao/actions/runs/38061700075 PASS; repository-contract, Windows/Linux active-go tests, p2 kernel fallback, nft/iptables shared TUN, OpenWrt TPROXY all success. next-lifecycle core https://github.com/lly8666/wobuzhidao/actions/runs/38061700072 PASS.
- New mixed V3 SOURCE tests **NOT_RUN_PENDING_ACTIONS** when authored. Existing parent PASS does not transfer. Need inspect Actions outcome, add real-space multi-client and performance when core passes.
- No local Go build or tests, cpu/quota/PSI/injected rate/local drops NOT_MEASURED. No performance dispatch. Historical FAIL 9729/da885 still FAIL, 80-second S2C and late probe deferred. No Windows native direct claim, deployment, merge, secrets or large pcaps.
