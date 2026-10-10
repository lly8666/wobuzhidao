# N0 protected V3 follow-up: first Actions regression, same-Tunnel policy fence

## Precise provenance
- Branch `next/adaptive-fec-aes-tun-20261010`. Before this commit, verified parent SOURCE `9729fe1d3e34d2a0cb85772e2ef5ef5868bff70d` with tree `50a8471356ac2f318f21dbfd99e1dde057b2a277`; inherited historical product SOURCE `7fb98fab79834a351a1dbe04eebb207f66bea28b`.
- The exact new SOURCE is the SHA of the commit containing this log, its code, single STATUS, and evidence. The new commit SHA cannot be self-referenced in its own contents.
- Connector-based Git object writes; no usable local checked-out worktree. No local Go/build/test, no deployment or merge.

## Evidence from first SOURCE, not edited away
- GitHub Actions next-foundation run [38060813844](https://github.com/lly8666/wobuzhidao/actions/runs/38060813844): **FAIL**. Linux active-go-tests job 114238557014 and Windows active-go-tests job 114238557087 both failed `TestAdmissionUnknownRecordVersionIsExplicitlyRejected` expecting ErrAdmissionVersion for `RecordVersionV2+1`, which is now defined as valid V3. The V3 parser returned ErrAdmissionParams for the artificial incomplete malformed V3 request. Other privileged repository-contract, p2-kernel-fallback, p4-openwrt-tproxy, and p4-shared-tun iptables/nft jobs completed success, but cannot convert overall FAIL to PASS.
- GitHub Actions next-lifecycle run [38060813846](https://github.com/lly8666/wobuzhidao/actions/runs/38060813846): core job 114238524678 **FAIL**, same unit assertion. This is a test expectation obsoleted by explicit V3 addition, not justification to change historical result statuses.
- Machine-specific runner CPU/flags/quota/PSI/steal, actual traffic injection and local drops not yet measured; no performance qualification of this source.

## Changes in this atomic SOURCE
- Change that unit's *unknown* version test to `RecordVersionV3+1`; retain legacy V1 rejection and V2 fixed vectors unchanged.
- LifecycleServer serializes admission and rejects a different V3 policy, cipher, FEC range, or version under the same TunnelID while active; after lane binding it pins version and canonical policy to the serverTunnel. V3 mode desired-lanes mismatch checked, old V2 defaults unmodified.
- Add real datapath Lane encoder/sealer/FEC decoder directed tests that intentionally disagree on caller global FEC parity while V3 protected policy selects off/20:4/20:20; verify first upstream and downstream source deliver independently without waiting for flush. This tests the real lane codec but is NOT a five-netns end-to-end or native Windows driver test.

## Honest current qualification
- Exact current-source foundation/lifecycle/race results: **NOT_RUN_PENDING_ACTIONS** at authoring. First-source failures still recorded separately in sole docs/STATUS.json and docs/evidence.
- N0 is **IN_PROGRESS**. Server allowlist, production CLI/JSON policy exposure, multi-client live TLS→FakeTCP→TUN/socket and policy binding throughout rotation still require real-path Actions evidence; no N1..N6 or product wide PASS.
- No diagnostic-on performance test, no perf Action dispatch, no pcap or secret upload. Prior Q2 FAIL, multi-second late, ~80s S2C, Windows Apply/Stop bug remain OPEN and unchanged.
- Next: inspect new SOURCE runs before any further code/STATUS qualification transition; then finish N0 protected per-client working configuration and appropriate Actions. No historical ABBA exception.
