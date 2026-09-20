# 20260920-130500 P4 Windows Wintun platform core Actions闭环

## 最终资格

SOURCE_SHA: eb7d0850b4f50f0d2de06c3d6523a72182c3444b
GitHub Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35489890541
Result: completed / success.

## Jobs

- repository-contract: PASS
- Windows 2022 active packages/unit/build: PASS
- Windows PowerShell Render contract: PASS
- Ubuntu 24.04 active packages/unit/build/race/fuzz/reference: PASS
- P2 kernel fallback + continuous pcap: PASS
- Linux privileged shared-TUN iptables: PASS
- Linux privileged shared-TUN nft: PASS

P2 regression: TestKernelTLSFallbackVerifiedHTTPAndNormalClose PASS 1.24s; pcap 29 captured / 58 received by filter / 0 kernel drop.

Artifacts:
- foundation: 10598966009
- tlsrecord-reference: 10597799038
- p2-kernel-fallback: 10597753963
- p4-linux-shared-tun-iptables: 10599080822
- p4-linux-shared-tun-nft: 10597709133

## Windows hosted evidence

internal/windowsclient unit tests passed on Windows 2022. The active PowerShell Render gate emitted:
- WBD_WINDOWS_CLIENT_PLAN
- UNDERLAY 198.51.100.10/32 pinned to the supplied physical path
- ADDRESS_EXCLUSIVE 10.66.0.7/32
- DNS_NRPT namespace=. servers=1.1.1.1,8.8.8.8
- IPV6_FAIL_CLOSED ranges=::/1,8000::/1
- CLEANUP state_owned_only=1

## Qualified semantics

- One Wintun L3 adapter per client runtime, not one per application flow.
- Wintun receive drops non-IPv4 before bytes enter active datapath.
- TUN->owner requires strict IPv4 and source==stable lease; desired=1 uses NormalOutbound, desired=2..4 uses GameOutbound.
- owner->Wintun requires strict IPv4 and destination==lease.
- WBD adapter lease /32 is exclusive; DHCP/APIPA/stale addresses are not allowed to coexist.
- Server underlay /32 is pinned to the pre-WBD physical interface/next-hop before broad capture, preventing recursive steering into Wintun.
- Optional direct prefixes use that same physical path.
- Full IPv4 capture is 0.0.0.0/1 + 128.0.0.0/1; DNS resolver /32s are captured.
- Ordinary DNS uses one exact WBD-owned NRPT namespace='.' rule; no global adapter DNS rewrite.
- Until IPv6 proxying exists, device-wide inbound/outbound IPv6 is fail-closed using ::/1 + 8000::/1 exact WBD firewall rules.
- Apply/Cleanup state records exact WBD-owned address/routes/NRPT/firewall objects and removes only those objects.

## Failure history

db2c990bfbc3efcb6e2b13b96471bd760df8a458 / Actions 35489839173 failed repository-contract before product tests because one REUSE_LEDGER record named two destinations separated by semicolon. eb7d0850 split that ledger entry into one destination per record; product Go/PowerShell/workflow were otherwise unchanged.

## Qualification boundary

This is hosted Windows core/PowerShell contract qualification. It does not prove real wintun.dll administrator-level Apply/Cleanup, Npcap raw packet underlay, or a physical Windows NIC. Those remain separate platform/P7 evidence.

## Next atom

Npcap/physical Windows underlay: extract only the physical interface selection and Npcap raw FakeTCP IO boundary needed by the active single-process lane owner, keeping the already-qualified Wintun route plan's server /32 underlay pin. Do not restore old Controller/Game child/DTLS subprocess architecture. Hosted adapter tests and physical Npcap qualification must remain clearly separated.
