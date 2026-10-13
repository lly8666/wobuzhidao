# N4 real WFP exact-owned cleanup in narrowly scoped TEST-NET LAB — candidate, 2026-10-13

Repository `lly8666/wobuzhidao`, branch `next/adaptive-fec-aes-tun-20261010`, baseline HEAD `673f90431533322d03fcca66db84001a00bb8b53`. The official `docs/STATUS.json` is sole handoff. No physical testing, no deployment, no mainline merge or qualification ref movements.

## User scope clarification

The user allows the final *physical* target to be treated as having **no unrelated Wintun adapter**. This removes multi-Wintun compatibility from the present final physical acceptance assumptions. It does NOT waive authenticated payload isolation, native bounded CAS recovery, complete production DNS/IPv6 safety or Windows Actions coverage before physical qualification. In particular a GitHub-hosted Windows runner is NOT thereby proven an isolated whole-network sandbox. Host-global `IPLocalInterface != ownerLUID` WFP or two /1 default-route capture must not be installed there.

## Current known exact source evidence

- CODE SOURCE `c2c70ee0a3f9e22f356c833fcc3b0232e72517dd`: N4 [matrix38408895055](https://github.com/lly8666/wobuzhidao/actions/runs/38408895055) Windows 2022, Windows 2025 and Linux race SUCCESS; [real Wintun38408895379](https://github.com/lly8666/wobuzhidao/actions/runs/38408895379) two Windows jobs SUCCESS; [Foundation38408894698](https://github.com/lly8666/wobuzhidao/actions/runs/38408894698) seven required SUCCESS; [Lifecycle38408895455](https://github.com/lly8666/wobuzhidao/actions/runs/38408895455) SUCCESS. WFP deletion was *FAKE* in the new orphan guard states, while Windows GUID/LUID/index probe was genuinely read-only native. Archive digest metadata in previous devlog. Do not inflate these results into a real persistent host-wide guard PASS.
- DOC SOURCE `673f90431533322d03fcca66db84001a00bb8b53` [Foundation38410911731](https://github.com/lly8666/wobuzhidao/actions/runs/38410911731) FINAL seven SUCCESS. Previous docs-only SOURCE50cf [Foundation38409758181](https://github.com/lly8666/wobuzhidao/actions/runs/38409758181) FAIL for missing newly added devlog remains historical FAIL, not rewritten. No new product qualification was inherited.

## New atomic implementation and Actions acceptance

- Refactors `n4WindowsPersistentWFPGuard.disarm` to use package-private `n4WFPDisarmExactPlan`: same production native `wf.Session` exact sublayer/rule/CAS validation, reverse deletion and **independently reopened** readback required to confirm full absence. A plan with missing/invalid IDs is rejected before OS mutation. Production `Arm` stays hard-disabled.
- Extends the existing self-cleaning real Windows persistent WFP LAB strictly to non-routable `203.0.113.254:65530` with randomized test GUIDs and **no** blanket DNS/IPv6 filters. LAB uses this exact same cleanup helper. New test installs one safely scoped persistent rule, proves foreign/altered rule-plan refusal without state mutation, manually interrupts deletion to leave a known empty owned sublayer, requires strict delete refusal, permits partial cleanup only through explicit after-routes semantics, verifies idempotent repeated cleanup and OS state absence. `t.Cleanup` always attempts exact LAB-owned cleanup if interrupted.
- WFP LAB workflow on Windows2022 + Windows2025 requires explicit `--- PASS` log for both real OS LAB tests; reports limited `SCOPED_REAL_OS_TESTNET_CAS_PARTIAL_RETRY_PASS`, while production guard remains `NOT_RUN`. N4 matrix, Foundation all seven and Lifecycle must validate same new SOURCE. New code and workflow initially **PENDING**, no inherited PASS or CPU/performance claims.

## Unresolved delivery hard gates

The official Windows client still executes PowerShell network setup/cleanup and large CN CIDR complement routing. Native five-stage assembly remains `assemblyOnly`. Global production WFP `Arm` remains disabled. Real complete crash recovery for confirmed lease/MTU/DNS/routes after OS removes original Wintun, isolated protected server /32 + two /1 route validation, production WFP/IPv6/DNS behavior, N5 integrated actions and N6 three-target same SOURCE P6 artifacts/manifests/hashes are **OPEN/NOT_RUN**. These are not replaceable with LAB or physical trial. N2 C04/C05 efficiency FAIL, C10 GE long p99 and approx 80s S2C remain OPEN under finite inheritance. `ACTIONS_READY_FOR_PHYSICAL` must remain unset and `physical=NOT_RUN`.
