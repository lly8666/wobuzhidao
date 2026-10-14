# 2026-10-14 Fast closeout: prioritize bounded N5 cross-feature Actions; N4 ghost-MTU stays UNSUPPORTED

Repository `lly8666/wobuzhidao`; branch `next/adaptive-fec-aes-tun-20261010`; parent exact HEAD `b16756f348eb33be2d894f1ccfff84cd92601755`. This next commit introduces a CI-only **N5 scoped** same-source integration gate (not N5 full completion) and updates the sole status; it does NOT edit production Go sources or relax any hard gate. New workflow evidence is **PENDING** until its own exact SHA Actions finish.

## Verified inherited evidence

- CODE SOURCE `c57ea67cdde2be6dcc2e39b5bcfeb7d63363af99`: N4 matrix [38500618292](https://github.com/lly8666/wobuzhidao/actions/runs/38500618292) Win2022/25 + Linux race FINAL SUCCESS; real created Windows Wintun [38500618294](https://github.com/lly8666/wobuzhidao/actions/runs/38500618294) both FINAL SUCCESS with **strictly scoped ghost DNS-only native CAS and two independent read-only lease-absence probes PASS**, but its full crashed original NIC cleanup **UNSUPPORTED** due PnP CM_NO_SUCH_DEVNODE 13, IPv4/IPv6 GetIpInterfaceEntry 1168 and absent MTU row. Remaining cleaning WAL guard+lease+MTU retained and fake test guard armed. Foundation [38500618289](https://github.com/lly8666/wobuzhidao/actions/runs/38500618289) seven required SUCCESS, Lifecycle [38500618300](https://github.com/lly8666/wobuzhidao/actions/runs/38500618300) SUCCESS. Exact jobs and GitHub ZIP archive SHA256 metadata are in `docs/devlog/20261014-n4-ghost-lease-readonly-scoped-actions-final.md`.
- Subsequent DOC ONLY SOURCE `b16756f348eb33be2d894f1ccfff84cd92601755` Foundation [38501087226](https://github.com/lly8666/wobuzhidao/actions/runs/38501087226) FINAL SUCCESS. Do not inherit this as fresh production-code qualification.
- Old N2 C04/C05 tier efficiency FAIL, C10 GE p99 and approximate 80s S2C stall remain OPEN. No changes to historical failed results.

## User's fast-close instruction and bounded new scope

The user explicitly requested fast closeout without repeatedly researching uncommon complex conditions. Therefore stop iterating ambiguous ghost-MTU cleanup just to gain a green mark. A missing original IP Helper MTU row is **not** proof that the original configured MTU was restored; do not shrink the remaining WAL, recreate/borrow an adapter alias, or disarm a persistent global WFP guard. Keep ghost crash recovery `UNSUPPORTED/NOT_RUN` unless a genuinely supported OS mechanism and independent Windows 2022/2025 Actions positive proof arise.

New `.github/workflows/next-adaptive-n5-scoped-integration.yml` performs no performance samples or privileged host-network mutations and cannot qualify normal Windows native routing. It runs exact-commit:
- Ubuntu repeated real V3 TLS/FakeTCP three-client independent first deliveries both directions and rotation; N3 AES lane tests, adaptive FEC and splitroute functional unit.
- Ubuntu `-race` on selected V3 and AES tests (no performance measurement).
- Windows 2022 repeated V3/AES functional + N4 Windows guarded orphan fail-closed unit tests. No Wintun driver privilege requirement in the new scoped test itself.
- A receipt job marks production WFP, global /1, direct underlay, Windows real crash recovery and physical all `NOT_RUN`; it refuses to aggregate if any required unit/race job fails.

This is useful early **N5 PARTIAL functional requalification** while prioritizing the standard normal Apply/Stop/Recover product wiring. Do not call it full N5, N4 production or `ACTIONS_READY_FOR_PHYSICAL`. No Linux/mock replacing actual host-wide WFP or Windows full route. Full N4 remains: `n4AssembleWindowsNativeOwnedTxn` has `assemblyOnly`, `n4WindowsPersistentWFPGuard.Arm` is hard disabled, official Windows CLI still runs PowerShell/.ps1, production compact server escape /32 plus two /1 and Direct real return path cannot be qualified on the current shared-host network without an isolated Actions Windows environment. True N5 mixed business realpath/normal/auto/Game/AES/rotation and full N6 exact one CODE SOURCE three targets + independent extracted SHA256 manifest remain OPEN. Physical `NOT_RUN`; no user-machine deploy, mainline merge or qualification ref motion.
