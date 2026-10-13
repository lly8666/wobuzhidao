# N4 official Windows compact routing + real userspace TCP/UDP ingress — final scoped Actions receipt 2026-10-13

Repository `lly8666/wobuzhidao`, branch next/adaptive-fec-aes-tun-20261010. Latest exact tested code SOURCE `4a0467c9737fb20b1248ec8793738c158d51be03`; if this document-only receipt creates a new HEAD, do NOT confuse with new Go SOURCE.

## Real product changes (not another unattached facade)
First SOURCE `ac7be190b3fcbbfa5828eee1b4e11497bc7b4156` changed live `cmd/wbd-client/main_windows.go` to `BuildCompactNetworkPlan`: host capture two IPv4 /1 and at most configured DNS /32, separately server /32 escape on physical NIC. LAN/CN/Direct CIDR classifier remains in process memory and never generates host CIDR-complement routes. It wired the already-implemented gVisor/tun2socks `NewUserspaceTCPDirect` physically bound to underlay ifIndex and generation, plus `NewNativeUDPDirect`, through `NewN4NativeDirect` into the **actual** `NewTUNPacketDispatcher.ForwardOrProxy` official CLI ingress; protected lane sealed/sent once and direct unsupported/queue-full packets never fallback to proxy. TCP runtime ring is bounded, no new CLI/GUI parameters. Existing PowerShell-backed owned rollback remains used until native replacement.

Second SOURCE `4a0467c9737fb20b1248ec8793738c158d51be03` removed legacy enormous `wbd-capture-*.txt` prefix-file fallback in `runNetworkActionContext`; before any child process it refuses >8 captures, any DirectRoutes, or nonempty CapturePrefixFile4. Extended existing actual CLI AST unit to assert `BuildCompactNetworkPlan` + `NewUserspaceTCPDirect` + `NewN4NativeDirect` + `NewNativeUDPDirect` exactly once, and negative pre-spawn hard cap tests. These stop reintroducing 10k+ host routes and associated GUI Apply/Stop hang.

## Exact Actions — source ac7be190
All FINAL SUCCESS:
- native N4 Win2022/2025+Linux race+receipt [38366859081](https://github.com/lly8666/wobuzhidao/actions/runs/38366859081), artifact11796939429 SHA256:44b0c9cc76d0c668ef60d00aeab033fcc271ad3064dc1b3f86dc07c30577786c
- userspace TCP Win2022 [38366859114](https://github.com/lly8666/wobuzhidao/actions/runs/38366859114), job115193944574 RUN/PASS `TestN4TCPUserspaceStackConstructorAndRealMux`, artifact11796714893 SHA256:b3fa34689793ab71b45a59941d8c920efaa6635b89a6ec9bc66248f5a33d6630
- actual Windows entry/native build + Linux ingress/race 38366859109, owned network cancel/rollback 38366859085, Windows GUI38366859138, N3 record cipher CLI config38366859155, Lifecycle38366859121
- Foundation38366859082 all 7 required jobs SUCCESS, P5/P6 extended skipped, Foundation artifact11797392113 SHA256:45c3d0c5dc6e249b3dbbd77ee8f4a2fda67cc6dfb94ca412ad72c3b473499593

## Exact Actions — final SOURCE 4a0467c9737fb20b1248ec8793738c158d51be03
All FINAL SUCCESS:
- N4 dedicated [38367313926](https://github.com/lly8666/wobuzhidao/actions/runs/38367313926), Windows2022 job115195434884, Windows2025 job115195434987, Linux race115195434433, receipt115196033592; artifact11797453038 GitHub sha256:78291f85a9c859b7c80e3fff137e0233204740a0c4ae79e3550fa6bdbe707e03.
- Windows actual CLI entry [38367313937](https://github.com/lly8666/wobuzhidao/actions/runs/38367313937), hosted Win job115195435134 and Linux race115195435628 SUCCESS, live compact/mux AST and legacy >8/file refusal covered; no Wintun driver activation.
- Windows owned cancellation38367313991 job115195434915 SUCCESS; native GUI38367314055 SUCCESS; N3 cipher config38367313927 SUCCESS; lifecycle38367314093 SUCCESS.
- Foundation [38367314006](https://github.com/lly8666/wobuzhidao/actions/runs/38367314006) all seven required jobs SUCCESS, P5/P6 extended skipped, artifact11797700319 SHA256:a89c6729c999f5c4a80f1b10fd2c825396675b1d7599332f6c29561c3c94414b.

SHA256 digests above are GitHub Actions artifact archive metadata, not independent artifact payload hash. Performance samples: zero, seeds/config unchanged. No local Go build/test/perf. This verifies hosted Windows/Go integration and live CLI code wiring, NOT actual Wintun/admin/direct physical TCP delivery.

## Remaining minimal finish line (do not overdesign)
Native Windows OS configuration is still PowerShell/.ps1 in official product: needed true owned Wintun lease/DHCP/DAD, MTU native CAS+rollback, DNS native Set/clear/restore, persistent WFP DNS/IPv6 leak guard with multi-tunnel safety or provable exclusivity, physical server /32 and exact two /1/DNS /32 route transaction, Stop/cancel/crash recover and foreign preservation. Existing Win32 writer and unified WAL primitives are available but disconnected, and native admin driver capabilities on GH must be qualified or correctly UNSUPPORTED/NOT_RUN. No further generic journaling scaffold needed; wire and qualify concrete operations, then remove script and package N6 three targets same SOURCE P6 manifest/hash. Physical must remain NOT_RUN until all phases and Actions ready; no deployment/merge/mainline or qualification refs. N2 C04/C05 efficiency FAIL, C10 GE p99 ~1.8–1.95s and ~80s S2C tail OPEN.
