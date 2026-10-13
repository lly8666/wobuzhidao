# N4 real child native Wintun lease/MTU/DNS write then forced crash WAL — candidate 2026-10-13

Repo `lly8666/wobuzhidao`, branch next/adaptive-fec-aes-tun-20261010, parent HEAD `b899c3f48946ca26c83a4c7c68fe514e5ac3183a`. New exact CODE SOURCE follows in the next commit; do not claim PASS until its own Actions finalize.

## Parent qualified scope

CODE SOURCE `a3bcd3bc786a09d3437437bcbc9f4e588ffb7340` N4 Windows2022+2025+Linux race [38420306021](https://github.com/lly8666/wobuzhidao/actions/runs/38420306021) FINAL SUCCESS, real Wintun driver regression [38420306011](https://github.com/lly8666/wobuzhidao/actions/runs/38420306011) FINAL SUCCESS, Foundation [38420306239](https://github.com/lly8666/wobuzhidao/actions/runs/38420306239) all seven and Lifecycle [38420306112](https://github.com/lly8666/wobuzhidao/actions/runs/38420306112) FINAL SUCCESS. It proved orphan WAL recovery from correctly formatted 2..4-stage receipts with Windows read-only OS absence and **FAKE WFP**. This was not evidence that those stages had been physically mutated before a crash. DOC-only SOURCE `b899c3f48946ca26c83a4c7c68fe514e5ac3183a` [Foundation38421110798](https://github.com/lly8666/wobuzhidao/actions/runs/38421110798) seven FINAL SUCCESS. Archive hashes and jobs recorded in `docs/devlog/20261013-n4-preroute-orphan-actions-final.md`.

## New exact native Windows stage-crash test

Adds `TestN4WindowsHostedRealPreRouteIPCrashWAL` with isolated Win2022/25 Actions substep and per-source JSON report. Actual child process (running isolated Go Windows test executable with pinned official Wintun 0.14.1) creates random owned Wintun, gets exact native GUID/LUID/index, starts checksum/fsynced single WAL with a **fake** persistent guard, and:
1. creates only 198.18.18.77/32 on the fresh disposable Wintun with real Windows CreateUnicastIpAddressEntry and waits for DAD Preferred, recording lease intent before write and owned after;
2. records MTU intent before real SetIpInterfaceEntry of only this Wintun; verifies and records owned;
3. captures pristine DNS baseline, records DNS intent before real SetInterfaceDnsSettings of two IPv4 resolvers only on this adapter, independently reads back exact state, then records owned.
4. emits readiness only AFTER fsynced full pre-route parent WAL. No server underlay, system route, global IPv6/DNS WFP, third-party NIC, physical user network, secrets or business traffic is touched.
5. parent verifies live original Wintun prevents WAL cleanup (ErrN4ForeignRoute), calls actual TerminateProcess on child WITHOUT normal Stop, requires full native interface enumeration prove original GUID/LUID/index disappeared, and only then attempts guarded parent WAL-only retirement with test FAKE WFP; confirms it is gone. If original persists, distinguish UNSUPPORTED/retain state, not a fake PASS.

The workflow checks report source and exact `FAKE_ONLY`, `routes=NOT_RUN`, `physical=NOT_RUN` and full network recover NOT_RUN. Any native failure is a test FAIL, not a reason to fake POSITIVE. Added new filename to workflow triggers. This may qualify a real crashed native lease/MTU/DNS stage window but not production WFP or route-stage crash recovery.

## Remains OPEN

Production global WFP `Arm` remains hard-disabled because safe whole-network guard is not qualified; five-stage product assembler remains `assemblyOnly`; official Windows CLI still uses PowerShell/.ps1; complete underlay /32 + two /1 host capture, route-stage crash, official native Apply/Stop and full N4/N5 integrated Actions, N6 3-target same CODE SOURCE P6 packages/manifest/hash not done. User permits single project Wintun final *physical* target only; Actions cannot substitute live global WFP with LAB or Linux/mock. No `ACTIONS_READY_FOR_PHYSICAL`, physical NOT_RUN; N2 C04/C05 efficiency FAIL, C10 GE long p99 and 80s S2C remain OPEN. No user-machine deployment, merge or qualification ref changes.
