# N4 quick WinDivert-only fast-path feasibility gate — candidate 2026-10-13

## Prior exact HEAD and user choice
At initiation branch HEAD 16ddca33286eebaab83e1354e44a357dfb8bfef1, docs-only Foundation Action38339215769 FINAL SUCCESS. User asks a QUICK feasibility test of assistant-proposed WinDivert approach, continue development ONLY if passes, otherwise STOP and DISCUSS. Do not mutate production networking, do not schedule premature physical testing.

## Decision requirement and pinned upstream API evidence
Required: few Windows capture routes while Direct TCP retains original client kernel TCP state, no second Go TCP proxy socket, no leakage via Wintun loop, deterministic outgoing physical ifIndex with reverse native TCP flow/generation isolation. Proposed short path was WinDivert NETWORK interception+WinDivertSend with Network.IfIdx=physical. The official WinDivert 2.2.2 documentation doc/windivert.html (upstream git blob 85a870a9c501ea3e22fb0715903d667f14044353, tag v2.2.2) explicitly says outbound IfIdx/SubIfIdx are IGNORED. The NETWORK_FORWARD layer only sees packets passing through the machine, unlike locally originated app TCP. WinDivert injected packets can be captured again at other priority levels. Thus the simplified WinDivert-only physical egress route is NO_GO and MUST NOT be confused with validation of all WinDivert uses or all viable architectures.

## Atomic repo-only test
tools/n4_windivert_fastpath_gate.py downloads exactly pinned tag document with hardcoded git blob SHA check, derives the source-level requirements and writes JSON receipt stating candidate_verdict=NO_GO plus unsupported tests. .github/workflows/next-adaptive-n4-windivert-fastpath-gate.yml runs Linux hosted SOURCE-scoped gate and uploads a 30-day artifact; does NOT download/execute WinDivert DLL/SYS or install a signed driver, does not alter Windows routes, WinTun, DNS, firewall or Npcap. An Action job success indicates correctly evaluated NO_GO, NOT product GO. If the official upstream content changes/unavailable or evidence missing, fail closed as INDETERMINATE and do not continue.

## Explicit nonqualifications
This gate is API contract preflight, not real Windows driver load, dual NIC Wintun roundtrip, TCP SYN/SYNACK NAT, physical capture, route reentry or performance. Both no-go and inconclusive halt further N4 TCP coding pending user discussion. N4 existing UDP/owned scoped PASS remains, original 725d FAIL and N2 efficiency/GE tail issues remain. N6 packages and physical acceptance not started; no deploy/mainline.
