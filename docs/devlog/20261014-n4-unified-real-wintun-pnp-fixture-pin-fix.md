# N4 real Wintun unified WAL fixture must carry original PnP pin — 2026-10-14

Branch `next/adaptive-fec-aes-tun-20261010`; exact source parent `f2c5768a91a987335014110ff17da760f8ffa5f3`. This is a new test-fixture correction candidate; no product production WFP, global route, Go parameter or protocol changes.

## Exact final Actions evidence on parent

- [N4 matrix38478051243](https://github.com/lly8666/wobuzhidao/actions/runs/38478051243) FINAL SUCCESS: Windows2022 job115573558426, Windows2025 job115573558443, Linux race job115573558739, receipt115574152859.
- [Foundation38478051237](https://github.com/lly8666/wobuzhidao/actions/runs/38478051237) FINAL SUCCESS all seven mandatory jobs, and [Lifecycle38478051267](https://github.com/lly8666/wobuzhidao/actions/runs/38478051267) core SUCCESS.
- [Real Wintun38478051227](https://github.com/lly8666/wobuzhidao/actions/runs/38478051227) FINAL **FAIL** Windows2022 job115573558317 and Windows2025 job115573557924. Real child created Wintun, recorded pinned original PnP in WAL, performed actual native lease/MTU/DNS, TerminateProcess, and kept WAL/fake WFP while original PnP devnode disappeared (`CR_NO_SUCH_DEVNODE=13`) and IPv4/IPv6 MIB rows were absent (1168). That crash recovery subtest was explicitly SKIPPED UNSUPPORTED, NOT rollback PASS. A separate pre-existing `TestN4WindowsHostedUnifiedWALRealLeaseMTUDNS` went through genuine same-process Wintun WAL stage Apply/Recover but failed at the product **dry assembler** preflight with `ErrN4JournalConflict` ("existing native route journal; recover first"). Root cause: this legacy hosted test constructed N4UnifiedIdentity with GUID/LUID/index/installation/generation but no now-mandatory `PnPInstanceSHA256`, whereas production assembly correctly refuses unpinned WAL. This is a test fixture not a permission to remove production identity checks. Previous failed SOURCES ff34, a918, 03d remain failed as recorded in STATUS.

## New candidate

After a newly created real driver Wintun's GUID/LUID/ifIndex is observed, and BEFORE any WAL fsync or lease/MTU/DNS write, invoke existing bounded SetupAPI `n4ProbeOriginalWintunPnP`, validate full SHA256, set `id.PnPInstanceSHA256` and verify `n4RequireOriginalPnPBinding`. The same pinned id is used by owner reservation, actual staged WAL Apply/Recover, and strict production dry assembly; its Arm/Apply remains intentionally disabled. No alias reuse or foreign adapter mutation.

The exact new CODE SOURCE must re-run Win2022/25 hosted Wintun, N4 matrix Win2022/25+Linux race, Foundation seven and Lifecycle. Inspect actual test selection and outcome, not workflow status alone. The original real postcrash ghost PnP problem is still UNSUPPORTED until native OS owned clean-up is proven: do NOT delete journal, disarm WFP or pretend missing row is safe rollback.

Remaining production hard gates: WFP `Arm` disabled, native five-stage `assemblyOnly`, official Windows CLI uses PowerShell, protected underlay server /32 + two /1 global route/WFP Actions needs demonstrable isolation, N5 full integration and N6 exact SOURCE three-target P6 artifacts/manifest/hash absent. Physical `NOT_RUN`, no `ACTIONS_READY_FOR_PHYSICAL`, no deploy/merge/qualification ref moves. N2 C04/C05 efficiency FAIL, C10 GE p99 and ~80-second S2C long-tail remain OPEN.
