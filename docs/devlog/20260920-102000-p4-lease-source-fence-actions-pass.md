# 20260920-102000 P4 lease source fence Actions闭环

## 最终资格

SOURCE_SHA：

`8d922df623305d169eee581ef02cea8dd6446a26`

GitHub Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35483525187

最终采用 run attempt 2，顶层结果：`completed / success`。

## 产品行为范围

### logicaltunnel source validator

`internal/logicaltunnel/source.go`：

- 只接受 well-formed IPv4 business packet；
- 最大 9000 bytes，延续归档 product packet ceiling；
- IHL 必须合法；
- IPv4 total length 必须精确等于 packet slice；
- inner source 必须 byte-exact 等于该 Logical Tunnel /32 lease；
- IPv6、另一 lease source、malformed packet fail-closed；
- active 实现不重新依赖 archived dataplane package。

### client owner boundary

leased `RoleClient` 的 `BusinessFlow.Outbound` 在调用 `Lane.Outbound` 前执行 lease source fence。

Actions专项证明：
- spoofed source 不进入 Lane；
- invalid packet 不增加 OutboundDatagrams/OutboundRecords；
- 不消耗 record PN；
- 一个 flow 的非法包不影响 sibling flow，后续首个合法包仍 PN=0；
- IPv6/malformed 同样在 lane state 之前拒绝。

### server owner boundary

新增 `TunnelOwner.InboundPayload(ref,...)`：

- 只接受当前 authoritative LaneRef；
- existing Lane decode 完成后再次 generation fence；
- leased `RoleServer` 对每个 decoded inner datagram 再做 source==lease；
- invalid datagram 不返回给 business caller，并加入 `PathErrors` / `SourceDiscards`；
- 即便恶意调用者绕过 client owner 直接用 Lane 封装 spoofed inner packet，server owner 仍拒绝；
- retired generation 在 business delivery 前拒绝；
- client receive 不套 source fence，因此 server->client 互联网 source 回包保持合法。

## 中间失败证据

### d7ca5ce product candidate

`d7ca5ce1d393499429048cbd9cb41aed6b46bd9c` / Actions `35483439066`：FAIL。

repository-contract 和 P2 kernel fallback PASS；Windows/Linux unit 都失败于上一轮既有 `TestLeasedOwnerPreservesIdentityAcrossReplacementDormantAndWake`，因为该旧测试仍把 `"flow-a"` 等字符串作为 leased client business packet。新 source fence 正确返回 `logicaltunnel: invalid IPv4 packet`。

修复只更新测试夹具为 source==lease 的合法 IPv4；产品源代码不放宽。

### 8d922df attempt 1

修复后的同一 SHA 在 attempt 1：
- repository-contract PASS；
- Windows unit/build PASS；
- Linux unit/build/race/fuzz/reference PASS；
- 唯一失败：hosted P2 raw network test 中 `RawIPv4Endpoint.ReadSegment` 的 `Recvfrom` 返回一次 `interrupted system call (EINTR)`，kernel test随后超时。

读取 active `internal/faketcp/raw_linux.go` 后确认该 hosted qualification adapter 对 EAGAIN/EWOULDBLOCK 重试，但未显式重试 EINTR；本轮 P4 source-fence 没有触碰 faketcp/raw 路径。

为了不扩展 P4 atomic scope，没有修改 P2 产品/adapter 代码；只对**同一 exact SHA 的失败 P2 job** targeted retry 一次。

## attempt 2 最终 Actions

- repository-contract：PASS
- Windows 2022 active packages/unit/build：PASS
- Ubuntu 24.04 active packages/unit/build：PASS
- Linux race：PASS
- directed parser fuzz：PASS
- independent reference vector generator：PASS
- P2 kernel fallback/continuous pcap：PASS

P2 final attempt：
- kernel Go test：PASS，1.24s
- 29 packets captured
- 58 packets received by filter
- 0 packets dropped by kernel
- existing pcap analyzer：PASS

Artifacts：
- foundation：10596676318
- tlsrecord-reference：10597051311
- p2-kernel-fallback final attempt：10597121327

## 未扩展范围

本 atom 没有：
- 迁移旧 TUN/raw-IP gateway/netns/NAT/CLI；
- 接 Linux/Windows/OpenWrt platform runtime；
- 实现 Game PacketID/racing/dedupe；
- 修改 Game lane 数；
- 启用生产 padding；
- 进入 P5/P7。

## 下一原子任务

按 `MODULE_MAP` 进入 Game：
- 定向读取 `old/internal/gamelane/`、`old/internal/gamepath/` 及直接测试/调用；
- 提取 PacketID racing/dedupe 最小 active 闭包；
- 接现有 `TunnelOwner` / `Lane` API；
- Game authoritative lanes 仍为2/3/4；
- 同 PacketID 多 lane竞速，首次有效到达交付，其他副本去重；
- FEC 仍 lane-local；
- 不做跨包条带化、不恢复旧独立子进程或CLI。
