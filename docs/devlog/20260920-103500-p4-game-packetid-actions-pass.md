# 20260920-103500 P4 Game PacketID racing/dedupe Actions闭环

## 最终资格

SOURCE_SHA：

`7a32b425b66e1018bc536cd3af8bd8e621ed38ce`

GitHub Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35484254427

顶层结果：`completed / success`。

## Actions jobs

- repository-contract：PASS
- Windows 2022 active packages/unit/build：PASS
- Ubuntu 24.04 active packages/unit/build：PASS
- Linux race：PASS
- directed parser fuzz：PASS
- independent tlsrecord reference generator：PASS
- P2 kernel fallback + continuous pcap：PASS

P2 regression：
- `TestKernelTLSFallbackVerifiedHTTPAndNormalClose` PASS，1.24s
- 28 packets captured
- 56 packets received by filter
- 0 packets dropped by kernel
- existing pcap analyzer PASS

Artifacts：
- foundation：10596572469
- tlsrecord-reference：10596312986
- p2-kernel-fallback：10596987541

本轮无中间失败或资格修复提交。

## 已资格产品语义

### internal/gamelane

提取归档 WGL1 data envelope：
- 32-byte header
- stable 16-byte SessionID
- uint64 PacketID
- LaneID 1..4
- 同一 logical payload/PacketID 为各 lane 生成 lane-distinct envelope
- 默认 bounded replay/dedupe window 4096
- first valid arrival 立即交付，后续同 PacketID duplicate suppress
- window 内 unique old PacketID 仍可乱序交付，没有 expected PacketID/no cross-lane HOL
- stale/wrong-session/malformed fail-closed

### TunnelOwner Game outbound

`GameOutbound` 只用于 desired=2/3/4：
- Game SessionID 直接绑定 stable TunnelID；
- 一个 business packet 只分配一个 tunnel-wide PacketID；
- fan-out 到当前 authoritative logical lanes；
- 每个 copy 各自进入该 lane 的 LINK/FEC/record path；
- 不做跨包 striping；
- FEC state 继续 lane-local；
- per-lane failure 不擦掉 sibling 成功结果；
- late generation 仍由现有 owner fence 拒绝。

leased RoleClient 的 source==lease fence 发生在 PacketID allocation 之前，所以 spoofed/IPv6/malformed client packet 既不触碰 Lane，也不消耗 Game PacketID。

### TunnelOwner Game inbound

`GameInboundPayload`：
1. 验证 authoritative LaneRef/generation；
2. 用对应 Lane 完成 record/FEC/LINK decode；
3. 再做 generation fence；
4. Game SessionID 必须等于 TunnelID；
5. envelope LaneID 必须等于实际 transport LaneRef.ID；
6. leased RoleServer 先验证 inner IPv4 source==lease；
7. 全部合法后才提交 tunnel-wide PacketID decoder。

因此 source-spoofed copy 或 wrong-LaneID copy 即使先到，也不能占用 seen PacketID，另一 lane 上相同 PacketID 的合法副本仍能成为 first valid arrival。

RoleClient receive 不套 client-source fence，server->client 互联网 source 回包正常。

### lifecycle

Game codec state 属于 TunnelOwner：
- same-ID replacement 不重建 PacketID namespace；
- DORMANT 关闭 transport lanes，但保留 encoder/decoder；
- wake 后 PacketID 继续递增；
- Close 才释放 Game state。

多个 BusinessFlow 注册与 Game transport membership 解耦，增加业务 flow 不创建 lane。

## Actions专项覆盖

- 3 authoritative lanes 对一个 logical packet 生成3份同 PacketID copy；
- 每 lane 只进入自己 Lane/FEC；
- lane3先到交付，lane1/lane2后到 duplicate suppress；
- PacketID 3 先于 PacketID 2 到达，两者均独立交付；
- client spoof 在 PacketID 分配前拒绝，首个合法包仍 PacketID=1；
- 绕过 client owner 的 spoof PacketID=100 被 server source fence 拒绝，同ID合法 lane copy仍交付；
- lane2 envelope 从lane1 transport进入被拒绝，同ID正确lane2 copy仍交付；
- server->client 互联网source合法；
- same-ID replacement 后 PacketID 连续；
- DORMANT 不分配PacketID，wake后继续；
- BusinessFlow registry增长不增加active/physical lane；
- Normal desired=1 owner明确拒绝Game API；
- gamelane codec另有4-lane envelope、wrong session、bounded replay/stale、invalid lane/reserved byte测试。

## 未扩展范围

没有迁移：
- archived Game UDP loopback client/server process
- membership/control sockets
- activity control
- InnerPacer
- rawip metadata/service bridge
- old game CLI
- old WBDP+Game gamepath MTU wrapper
- platform TUN/Npcap/OpenWrt runtime

没有：
- 把 Game 改成跨包条带化
- 把 candidate/retiring headroom 当额外 logical lanes
- 启用 production padding
- 做 P5 流量外观/抗识别结论
- 进入 P7

## 下一原子任务

进入 P4 owner-level production padding budget：
- 默认 off；
- 每 record padding cap；
- tunnel-wide 累计 padding / useful payload 比例与总量预算；
- budget不足立即0，不等待/不凑包；
- Normal sibling BusinessFlow 共用预算；
- Game 一个 logical packet 的 useful payload 只记一次，2..4 lane copies/FEC/parity/repair 不重复增加预算；
- replacement/DORMANT/wake 不绕过预算；
- 暂不接CLI/platform配置。
