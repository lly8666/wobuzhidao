# N4 native WFP read-only Windows 2022/2025 matrix completed; remaining scriptless-owned lifecycle (2026-10-13)

## Exact SOURCE and committed evidence
Exact tested SOURCE `243da7bf59dd8601f1e2e5fac879d114b2c6129e`, parent `a4558ce9a6d4cc25c3b76606bacfbe22b00ba195`.
- Dedicated N4 native Go transaction / WFP layer probe Actions https://github.com/lly8666/wobuzhidao/actions/runs/38347526977 FINAL SUCCESS all four jobs: Windows 2022, Windows 2025, Ubuntu-24.04 -race, receipt. Artifact id11789707379, zip SHA256 sha256:6f191a1511c2716a2f9d626cb7bce6f7f5d64d9600f63b25975f7261dfabf83a.
- Same SOURCE Foundation https://github.com/lly8666/wobuzhidao/actions/runs/38347526964 FINAL SUCCESS all seven required jobs: repository contract, Windows and Linux active Go including race, P2, P4 Linux shared TUN iptables/nft and OpenWrt. Extended P5/P6 skipped NOT_RUN. Artifact id11789632356, zip SHA256 sha256:1c1580a7a6cc09cac7f143b6663e0f94739394c704c840b00227a858c7031f9c.
- Same SOURCE lifecycle https://github.com/lly8666/wobuzhidao/actions/runs/38347527006 core job SUCCESS.
- V3 three-client independent TCP-like TLS admission, first packets BOTH directions and rotation with different negotiated FEC/cipher profiles, old SOURCEb7 Action38346699603 Linux20x/Windows20x/Linux race8x SUCCESS after fixing premature server policy read.
- Preserve original failures: 132e source map test assertion FAIL Action38345945687/Foundation38345945574; 4fd scanner false-positive FAIL Action38346113126; 02f V3 registration timing FAIL Foundation38346361555; b7 unsafe Win SDK initialization flags FAIL Foundation38346699600; 50172 native type FAIL Actions38343998563/38343998549/Foundation38343998570; old WinDivert IfIdx-only NO_GO Action38341416320. Each exact-source original FAIL remains documented and is not retroactive PASS.

## What this DOES and DOES NOT prove
The Go-only Windows code now has an inactive userspace TCP Direct stack (tun2socks v2.6.0), existing UDP Direct, constant two /1 capture plan with China IP sets in memory, native IPv4 route ABI and strict scoped ownership API, SHA256/O_EXCL/intent-before-create journal, crash ambiguity conservative hold, read-only Windows Wintun IPv4/DAD/APIPA check, Win10 Build19041+ native DNS and WFP API exports, and native read-only WFP IPv4 connect + IPv6 connect/inbound layer enumeration. Tests exercise failure injection, cancellation, concurrency, repeated recovery and multi-OS Go coverage.
**These are scoped feasibility/source tests only**, NOT native admin route Apply/Cleanup or real Wintun/external Direct TCP/UDP. WFP layer reads are NOT IPv6 or DNS block rules; missing crash-persistent WFP ownership and DNS interface configuration are hard stop. This is not safe to activate.

## User's controlling requirement and explicit next engineering boundary
Final Windows binary must not invoke or require PowerShell, nor ship active *.ps1 network helpers; user explicitly cannot assume all Win versions have PowerShell. Do not install thousands of China IPv4 route prefixes. Instead finish:
1. Native Win WFP **persistent** IPv6/DNS anti-leak block filters with stable owner GUIDs, exact cleanup, verification after engine reopen/process crash, no dynamic WFP session cliff.
2. Native DNS Set/GetInterfaceDnsSettings with per-interface verified ownership, OS version >=19041 fail closed for older versions or a clearly supported native fallback; protection before any capture route.
3. Strict WBD-owned Wintun adapter index/IPv4 lease/DHCP+MTU restoration, physical server /32 escape and small bounded capture route transaction with fail-closed journal, robust partial/cancel/crash restart recovery.
4. Windows hosted administrator real Wintun Actions for capture Apply/Cleanup, DNS/IPv6 block, foreign address/route retention, no loop/no leak, changes between NICs, unusual termination and repeated Stop (if capability available).
5. Then wire native transaction and disabled gVisor TCP Direct + NativeUDPDirect/compact classifier into single-process cmd/wbd-client; remove old script spawn, flags, active files and packager. Preserve old/ archived evidence.
6. N6 same-source Windows Linux AMD64/ARM64 artifacts and SHA256, only then physical machine. N2 C04/C05/C10 efficiencies and 80s S2C long tail remain OPEN.
No deployment, mainline merge/ref move, local tests or physical machine. Test/build/race only GitHub Actions.
