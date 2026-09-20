# P4 single-process client/server entry + endpoint loop — Actions PASS

## 结果

产品 SOURCE_SHA `79c6e9ddcbc3e7f74b85dcc6d162d5c1a3c4e3f9` 在 `next-foundation` Actions run 35500769979（attempt 1）完成，7 jobs 全部 PASS。本 atom 因而取得 hosted exact-SHA 资格。

该结论只覆盖最小单进程入口与 endpoint loop：Normal 单 lane、静态 lease 配置、Linux raw / Windows Npcap seam、平台入口和真实 TLS/admission -> runtimeowner handoff。真实 Windows/Npcap driver + 物理 NIC 仍为 P7 `NOT_RUN`；命令级 Game 2..4 / rotation / replacement / DORMANT/wake 尚未据此宣称完成。

## 本 atom 的产品闭包

- `faketcp.ClientAssociation` 补齐客户端 SYN -> SYN-ACK -> final ACK -> BootstrapStream -> explicit detach；peer MSS/WS/SACK 与 sequence handoff 被固定到同一 association。
- `runtimeentry.DialClient` 在 endpoint read/tick loop 中完成真实 TLS + protected admission，随后把 send/receive sequence 移交给 `runtimeowner.AttachClientAdmission`。
- `runtimeentry.Server` 复用 `ServerAssociationTable`、`realityfront.HandleServerAssociation`、leased `TunnelOwner`、Linux `SharedTUNRouter` 与 `platformflow.Server`；detach 到 runtime attach 之间有界排队。
- server 只有在 `HandleServerSegmentQualified` 成功处理首条 detached steady record 后才允许 shared-TUN business egress，避免在客户端证明 post-admission sequence ownership 前消耗/发送新模式 record。
- `cmd/wbd-server`：Linux shared-TUN runtime + raw IPv4 endpoint。
- `cmd/wbd-client`：
  - Linux/OpenWrt：TPROXY runtime + transparent SocketAdapter + raw IPv4 endpoint；
  - Windows：PhysicalUnderlay + Npcap + Wintun + existing owned-state PowerShell Apply/Cleanup。
- 未恢复 localhost UDP、DTLS shim、platform-proxy 子进程、旧 Controller 或 archived CLI compatibility。

## Actions 证据

Run: https://github.com/lly8666/wobuzhidao/actions/runs/35500769979

### repository-contract

PASS。

### Ubuntu 24.04 active-go-tests

- 整仓 unit/build PASS。
- `internal/faketcp`: PASS 0.134s。
- `internal/runtimeentry`: PASS 0.011s。
- `go test -race ./... -count=1`: PASS。
  - `internal/faketcp`: PASS 1.148s。
  - `internal/runtimeentry`: PASS 1.027s。
- tlsrecord directed fuzz: PASS，15s 阶段累计 164,994 execs（最终日志 16s/164,994）。

### Windows Server 2022 active-go-tests

- 整仓 unit/build PASS。
- `internal/faketcp`: PASS 0.156s。
- `internal/runtimeentry`: PASS 0.026s。
- Wintun Render contract marker：
  `WBD_WINDOWS_CLIENT_PLAN schema=wbd-windows-client-state/v1 adapter=WBD lease=10.66.0.7/32`
- Npcap hosted marker：
  `WBD_WINDOWS_NPCAP_HOSTED_CORE_PASS physical=NOT_RUN`

### P2 kernel fallback

- `TestKernelTLSFallbackVerifiedHTTPAndNormalClose`: PASS 1.24s。
- 29 packets captured。
- 58 packets received by filter。
- 0 packets dropped by kernel。
- analyzer result PASS。

### Linux shared-TUN privileged

- iptables：`WBD_P4_LINUX_SHARED_TUN_PASS backend=iptables active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged`，test 0.08s PASS。
- nft：`WBD_P4_LINUX_SHARED_TUN_PASS backend=nft active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged`，test 0.11s PASS。

### OpenWrt privileged

- `TestPrivilegedOpenWrtTPROXYRuntime` PASS 0.22s。
- marker：`WBD_P4_OPENWRT_TPROXY_PASS tcp=1 udp=1 underlay_bypass=1 cleanup=owned-only ipv6=NOT_IMPLEMENTED`
- `TestPrivilegedOpenWrtSocketTunnelAdapter` PASS 0.13s。
- marker：`WBD_P4_OPENWRT_SOCKET_TUNNEL_PASS tcp=1 udp=1 eim=1 eif=1 tunnelowner=1 lanes=1 service_tun_leak=0 ipv6=NOT_IMPLEMENTED`

## Artifacts

- foundation: 10602685418, sha256:4a290ea40232396d539011028117e4c31a3f5768ecf269a8fdcd8fece53b9741
- tlsrecord-reference: 10602276961, sha256:d2706276ebddc139934cf19681289b117f9a502f34db96ea2adae01e7829f9d8
- p2-kernel-fallback: 10602496386, sha256:876267991e1bc1867d1551b8afa3c4c864ebb2666a3497cb715d542409903348
- shared-TUN iptables: 10602635555, sha256:1eef01e803b948672187c6ad670fafcb10d4e2d5a9d4e2190a2444f10397911c
- shared-TUN nft: 10602017146, sha256:072a9c27fa65642d26b441c0da5bb4f2fea4d9b72c67a6a20ede9097aee97943
- OpenWrt: 10601952488, sha256:6d3577a5b66035cf5902ace6ddaa9258be59de4624d7172ce8eabaddae6bca03

## 资格边界

- `last_tested_source_sha` 更新为产品 SHA `79c6e9dd...`；后续 docs-only closure SHA 不得替代它。
- Windows/Npcap physical 仍 `NOT_RUN`，不能标为 PHYSICAL_PASS。
- OpenWrt IPv6 仍 `NOT_IMPLEMENTED`。
- 当前 cmd 入口固定 Normal 单 lane + 静态 lease。核心 Game、replacement、DORMANT 已分别在此前 atoms 资格，但尚未全部编排进最终入口，所以 P4 仍保持 IN_PROGRESS。
- steady recovery 保持当前 runtimeowner cumulative-ACK + 4096 metadata + 1s RTO / 3s repair horizon；未迁入完整 archived SACK/RACK/adaptive-pressure。

## 下一原子任务

把已资格的 Game 2..4 lane 生命周期、same-ID make-before-break replacement/retire、DORMANT/wake 和稳定 lease 身份编排接入当前 runtimeentry/cmd；保持单进程、同 association 每 lane、4 权威 lane / 10 physical incarnation 上限，不进入 P5。
