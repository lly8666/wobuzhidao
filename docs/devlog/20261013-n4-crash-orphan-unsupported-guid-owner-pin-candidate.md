# N4 real crash-orphan unsupported and native GUID/alias owner hardening — candidate 2026-10-13

Repo `lly8666/wobuzhidao`; branch next/adaptive-fec-aes-tun-20261010. Previous exact CODE SOURCE `1c2bf68ffc9fb2444dc1629fe02b94978668b52c`. New CODE SOURCE is the ensuing commit, pending Actions; no local Go/build/performance or physical test.

## Verified prior-source evidence

- [Real Win Wintun Action38395038373](https://github.com/lly8666/wobuzhidao/actions/runs/38395038373) FINAL SUCCESS in jobs Windows2022 115287979632 / Windows2025 115287979429. Both ran actual child fsynced checksum WAL, parent TerminateProcess, then `OpenExistingTUN` for five seconds, but OS removed adapter. Test `TestN4WindowsHostedCrossProcessCrashReadOnlyIdentity` **SKIPPED** `UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED` with `WintunOpenAdapter: Element not found`. Workflow PASS is not native cross-process crash Recover, borrowed owner mutation, or same-name replacement permission. Win2022 artifact11811284704 ZIP sha256 `fa668e3420d15e2b6717373e4a0334051df00e43f6a0d83a0c58ed19f1dfa498`; Win2025 artifact11811284744 ZIP sha256 `fec289bee38bab70c8d32f6b7628397554b0fc6a234ab7ae86025911809bcef2`, reported GitHub archive digests only.
- [N4 matrix38395038348](https://github.com/lly8666/wobuzhidao/actions/runs/38395038348) Win2022, Win2025 and Linux race SUCCESS; [Foundation38395038330](https://github.com/lly8666/wobuzhidao/actions/runs/38395038330) seven required jobs SUCCESS (P5/P6 extended SKIPPED); [Lifecycle38395038305](https://github.com/lly8666/wobuzhidao/actions/runs/38395038305) SUCCESS, same parent SOURCE.

## New atomic candidate

- `N4WindowsCreatedWintunReservation` refuses half-populated WAL GUID/alias, foreign alias and independently observed OS GUID mismatch whenever GUID/alias are supplied; existing installation/generation/created-by-process/LUID/index fence stays.
- `n4AssembleWindowsNativeOwnedTxn` now requires BOTH native OS GUID and exact adapter alias to reject legacy LUID/index-only identity before the five-stage transaction can be assembled; separately reads GUID via Win32 against live Wintun.
- New Windows no-driver negative tests reject half-bound/foreign identities; real Win2022/25 hosted Wintun WAL now carries real OS GUID+alias and tests wrong GUID refusal.
- The compiled five-stage assembler remains `assemblyOnly`, `n4WindowsPersistentWFPGuard.Arm` remains disabled; no new mutation/route/product activation. Relevant exact new SOURCE Actions must finish before PASS.

## OPEN

Broad persistent WFP LUID-not-equal DNS/IPv6 rules would affect other independently managed Wintun adapters. Do not enable without genuinely safe shared policy/exclusive proof. Original Wintun disappears at cross-process crash; no borrowed alias takeover. Production native own WAL/route orphan recovery, OS actual WFP and two /1 capture, product cmd/wbd-client live PowerShell removal, complete N4/N5, P6 3-target same SOURCE, physical remain OPEN/NOT_RUN. N2 C04/C05 efficiency FAIL, C10 GE p99 and ~80s S2C remain OPEN, no deploy/merge/qualification ref moves.
