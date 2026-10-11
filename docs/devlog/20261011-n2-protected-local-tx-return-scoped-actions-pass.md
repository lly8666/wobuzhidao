# N2 protected returned local-TX source proposal — scoped Actions PASS

Date 2026-10-11, exact product SOURCE `dcf0a5549738672a05934ccf810bfd7ca7875a96` on `next/adaptive-fec-aes-tun-20261010`. The preceding actual sealed source-cohort fixture SOURCE `238afa53` foundation38130772105/lifecycle38130772069 PASS, and joint size/k/r evidence SOURCE `08be77e2` foundation38131135195/lifecycle38131135165 PASS. Historic PN0 test oracle red runs 38130647820/38130647838 remain FAIL.

## Implemented and proved

Pure `qualityFirstAttemptBoundFromSnapshot` is now the one reusable same-source age/nonce/generation/Pn/repair-coverage gate for read-only access and an authenticated V3 receive callback. On an already quality-enabled, active **single Normal lane**, a deliberately test-only `ConfigureN2ControllerForTests` installs a separate guarded controller for that one incarnation. The existing protected quality mailbox + returned-echo acceptance must finish successfully before the same lane's local **TX** source bound reaches the controller. It never consumes this endpoint's independent RX-side quality as an outbound choice. A bounded read-only `N2ProposalForTests` publishes the decision; no FEC encoder changes, no new report, timer, ACK, wake-up or queue on fresh. Duplicate returned source, stale/unmatched/wrong generation and missing source HOLD. No reuse across dormant/replacement refs. `realityfront.AdmissionPolicy.SupportedNow` still blocks FECAuto/FECAggressive.

## Four exact-source Actions

- [Foundation #38131512231](https://github.com/lly8666/wobuzhidao/actions/runs/38131512231): 7 enabled jobs PASS, including Linux and Windows Go/unit/race and repository/kernel/native privileged scopes; P5/P6 SKIPPED. Foundation artifact 11699610222 SHA256 `eed257d2e3e7537ac12eda8fd6963ac63f7242306d4f939f3090759eea782544`.
- [Lifecycle #38131512143](https://github.com/lly8666/wobuzhidao/actions/runs/38131512143): core job 114446632712 PASS.
- [N1 codec/race #38131512116](https://github.com/lly8666/wobuzhidao/actions/runs/38131512116): job 114446632826 PASS.
- [N0 hosted Linux 3-client #38131512159](https://github.com/lly8666/wobuzhidao/actions/runs/38131512159): job 114446632888 PASS, **functional only**, NOT N2 link impairment or Windows Wintun.

Complete run/job/artifact/source receipt at `docs/evidence/adaptive-n2-protected-local-tx-return-actions-20261011.json`. No local Go, local compilation or tests; no N2 10Mbps/performance or physical machine deployment.

## Next unqualified gates

1. Existing `SwitchN2ParityForTests` prepares its new encoder under Lane TX mutex. Before shipping, move cold encoder allocation OUTSIDE the TX mutex and commit an O(1) new-block-only switch with monotonic BlockID, bounded old partial draining, stable worst-case 20:20 record/TUN MTU, shared mixed decoder and 3s recovery. Fresh must not wait for change preparation.
2. Apply proposals only through generation-pinned active Normal TX owner, not inbound RX, wrong server client, retiring lane or game, with capacity/feedback interruption HOLD and no idle quality refresh.
3. Complete once-protected per-client V3 Normal fixed/off/auto/aggressive admission and capability-accurate synchronized CLI/JSON/catalog; Game fixed, old fixed unchanged.
4. Only then one SOURCE/config/seed/impairment scenario/measurement job per Actions for 10Mbps each direction, 0→1→5→20→5→0, low/300ms RTT, sparse/off, asymmetric uploads/downloads, small MTU and Game4 fixed3Mbps; distinguish measured insufficient/capacity/not-run/real failures. N3 AES/N4 Windows TUN/N5 Chinese GUI/N6 multi-target P6 still NOT_STARTED.

Historical off-v2 MEASURED_NOT_QUALIFIED is not PASS, default auto-MTU off returning probe p99 ~2.73s/missing and ~80s early S2C/late remain OPEN without causality claim. N2 overall remains **IN_PROGRESS**, no MAIN merge, no old qualification move, physical **NOT_RUN**.
