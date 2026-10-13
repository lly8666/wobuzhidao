# N4 native owner GUID/alias preflight changed expected error order — focused test fix candidate

Parent SOURCE `cdc6116264d4ff77495e94bbf92d946971df16a1`, branch next/adaptive-fec-aes-tun-20261010. Verified final Actions from this exact source:

- [Real hosted Win2022/25 Wintun Action38397969045](https://github.com/lly8666/wobuzhidao/actions/runs/38397969045) final **SUCCESS** for isolated new adapter, OS-observed GUID/alias WAL and deliberate wrong GUID rejection; this does NOT cover crash native Recover, global production WFP, or two /1 protected routing.
- [Lifecycle38397968965](https://github.com/lly8666/wobuzhidao/actions/runs/38397968965) final SUCCESS.
- [N4 matrix38397968999](https://github.com/lly8666/wobuzhidao/actions/runs/38397968999) final **FAIL** on BOTH Windows2022 job115297935391 / Windows2025 job115297934888. Linux race job115297935266 SUCCESS, receipt SKIPPED. Exact failing test `TestN4WindowsNativeWALAssemblerRequiresOwnedWintunAndPersistentGuard` expected `ErrN4NativeGuard` but observed `ErrN4JournalConflict`: the new earlier GUID/alias hard gate correctly refuses the fixture's missing identity before evaluating missing WFP.
- [Foundation38397968939](https://github.com/lly8666/wobuzhidao/actions/runs/38397968939) final **FAIL** on Windows active-go job115298020134 for same test; other required jobs succeeded as concluded. Failures preserved without renaming or granting partial scope full PASS.

Candidate correction changes **only** the existing guard-negative test fixture: populate valid syntax WintunGUID and AdapterAlias before asserting missing/foreign WFP guard; independent `TestN4WindowsNativeAssemblyRejectsUnboundLegacyGUID` explicitly asserts missing/partial identities remain rejected with `ErrN4JournalConflict`. No production code, native OS mutation, parameter, routes, WFP or controller change. New exact code SOURCE Actions required.

Prior real child crash SOURCE 1c2bf68 Action38395038373 Windows2022 and Windows2025 returned `UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED`, NOT recovery PASS; adapter auto removed after TerminateProcess. WFP Arm remains disabled, product CLI script/PowerShell active, N4 full + N6 physical NOT_RUN; N2 C04/C05 efficiency FAIL, C10 GE and ~80s S2C OPEN. No mainline merge, ref motion, deploy, or physical tests.
