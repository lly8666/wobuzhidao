# N4 Windows native DNS setter current-owner identity fence — candidate, 2026-10-13

## Exact parent and prior Actions
Branch `next/adaptive-fec-aes-tun-20261010`, expected parent commit `c10a0732569af95a2cdabcad2402b457f620496f`. Current GitHub final receipts on previous SOURCE: dedicated Windows2022+Windows2025 and Linux -race Action38354764587 **SUCCESS**; Foundation38354764412 **SUCCESS all 7 required** (P5/P6 extended skipped); lifecycle38354764394 **SUCCESS**. These scope only exact compact route planning and hosted Go safety tests. No admin Windows network mutation. No local Go commands run.

## Identified hard ownership gap
Existing `n4OwnedDNSWrite.SetIPv4` validates persistent WFP guard and a caller-supplied GUID but does not independently authenticate Wintun ownership; a caller can pass an arbitrary Windows interface GUID when the guard happens to be active. The component remains unconnected from product but is intended for a future owned network transaction; harden it before wiring.

## Patch
`n4OwnedDNSWrite` now requires an `n4OwnedInterfaceProof` and interface index, then checks `IsCurrentOwnedWintun(ctx,index)`, and resolves `ConvertInterfaceIndexToLuid → ConvertInterfaceLuidToGuid` to require actual GUID/index agreement before `SetInterfaceDnsSettings`. Fail-closed for missing proof, foreign/borrowed or stale Wintun, index zero, invalid DNS, canceled context, missing or inactive persistent guard. Adds strict Windows no-mutation tests `TestN4NativeDNSWriterRequiresOwnedLiveAdapterBeforeOSRead`; existing negative test signatures updated. This does NOT yet ensure ownership remains locked from proof through the write, nor create a crash-durable DNS intent, nor implement an empty-DNS reversible restore. It is a prerequisite only.

## Required Actions and limits
Push should trigger `next-adaptive-n4-owned-native-transaction-matrix` on Win2022/25 with explicit `TestN4Native*` plus Linux race, and `next-foundation` branch Go matrix. At commit time new candidate is ACTIONS_PENDING. No Windows SetInterfaceDnsSettings positive mutation or actual Wintun driver in hosted tests. No performance sample/seed or user setting changed. Do not claim product N4 complete, physical remains NOT_RUN, not ready for physical. N2 curve efficiencies, C10/GE and ~80s tail remain OPEN; N6 P6 not started. No deploy or merges/ref movement.
