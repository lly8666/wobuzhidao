# Configuration probe corrections and exact-source packaging

## 本轮目标和阶段
P5/P6 pre-delivery, starting9e3dca5. Correct first live-canary framework failures and prepare final same-source package validation.

## 修改与原因
Go boolean flag syntax in generated CLI must use --tls-startup-padding=false/true. A separate false token is positional and stops the Go flag parser, leaving later lifecycle values from JSON; first priority probes correctly rejected that mismatch. Use equals form and fixture asserts exact boolean value. FEC encoder stats have explicit source_shards/parity_shards JSON names; checker now reads actual serialized names. JSON-only first probes already executed real FEC/DNS/TCP/HTTPS but were wrongly rejected by this field-name error. Production parsers/runtime unchanged.
Add independent P6 packaging workflow, three supported platform targets, source/version/file-hash manifest and actual native --version where supported. Aggregate three receipts checks target/source/status; never claims physical qualification. Linuxarm64 remains cross-build; Windowsclient only and no bundled Npcap/Wintun drivers. Add PREDELIVERY_ACCEPTANCE as coverage/limitations document under sole existing STATUS authority.

## 复用来源
Current p6_build_release.py/check_p6_release.py, existing supported target matrix. No old modules or protocol changes.

## Actions证据
9e3dca5 foundation37120155483 and tools37120155385 PASS, including30-repeat lifecycle race. Main gated controller37120194786 dispatched7 independent canaries. Default live configuration37120281993 PASS. Original priority-false37120275848 and priority-FEC20/Game437120277867 FAIL on actual effective lifecycle/padding and/or stats-name assertions; JSON-FEC1037120279404 andMTU1280/Game437120280643 FAIL solely on incorrect encoder field lookup. Do not erase these failures or call them product regressions. Normal/Game180s37120272754/37120274350 still running at log creation. Fixed canaries/new-source core and P6 NOT_RUN. Tests only Actions.

## 问题、排查与风险
30-repeat race now passed but two earlier barrier repetitions failed; retain diagnostics and original failures, no production race claim. Config canaries must be rerun before expanding70. Long-target harness still unqualified. Packaging green cannot close P5 performance; physical and full website-fingerprint claims remain excluded.

## 下一项原子任务
Read short-soak raw/summary, repair any defect. Require new exact-source fixture/foundation gates, then fixed configuration canaries. All70 and1800sNormal/Game after canaries; final18 same-source; gated P6 packages and final receipts.
