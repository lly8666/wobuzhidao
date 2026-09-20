# 20260920-103000 P4 Game PacketID racing/dedupe owner API

## 原子任务

开始时重新同步 `next/tlslike-dataplane`，确认 HEAD 仍为
`2a4c013f9727488ac29cacf183972c53e718c50e`，与上一 source-fence
evidence closure 完全一致；`STATUS.next_task` 仍要求定向提取 Game
PacketID racing/dedupe 并接当前 owner API。

本轮只做：
- Game mode 的 2/3/4 authoritative logical lanes；
- 一个业务 packet 一个 tunnel-wide PacketID；
- 同 PacketID 在多个现有 lane 竞速；
- 首次有效到达交付，其余副本去重；
- unique PacketID 可乱序，无跨 lane HOL；
- FEC 继续 lane-local；
- PacketID/dedupe state 属于 Logical Tunnel，不随 lane replacement/DORMANT 重建；
- 不迁移旧 game 子进程/CLI/platform。

## 归档读取

Archive source SHA：
`b5c848f4e9afdffd15d1bc451560edf4e9390a35`

定向读取：
- `old/internal/gamelane/gamelane.go` / tests
- `old/internal/gamelane/control.go` / tests
- `old/internal/gamelane/activity_control.go` / tests
- `old/internal/gamepath/mtu.go` / tests
- `old/cmd/wbd-game-lane-client/main.go` / direct tests
- `old/cmd/wbd-game-lane-server/main.go` / direct tests
- current `TunnelOwner` / `Lane` / logicaltunnel lifecycle/policy
- PROJECT_CHARTER / WIRE_SPEC / ACCEPTANCE 的 Game hard requirements

归档核心语义：
1. WGL1 envelope 固定 32 bytes。
2. SessionID 16 bytes；一个 logical packet 分配一个 uint64 PacketID。
3. 同 payload+PacketID 针对不同 LaneID 形成 lane-distinct plaintext envelope。
4. 发送时对当前逻辑 lane fan-out，不做跨包 striping。
5. 接收端一个共享 decoder：first arrival wins；其他 lane copy duplicate suppress。
6. bounded replay window；窗口内 unique old PacketID 仍可独立交付，没有 expected PacketID。
7. replacement overlap 仍是同 LaneID/同 PacketID namespace，不是新 logical lane。
8. 归档 client/server 的 loopback UDP control、membership、rawip metadata、独立进程只是旧架构壳，不迁移。

`old/internal/gamepath/mtu.go` 也已检查。它的 overhead 同时包含旧 WBDP frame 和 Game
header；active 新数据面没有旧 WBDP frame，Game envelope 直接进入现有 LINK，所以本 atom
不复制该旧 MTU package。32-byte Game envelope 作为 LINK 输入的一部分继续经过已资格化的
一次 fragmentation、lane-local FEC 和 record/path MTU budget。真正平台 TUN MTU 暴露留在
后续 platform atom。

## active internal/gamelane

新增 `internal/gamelane/gamelane.go`：
- `HeaderSize=32`
- `MaxLanes=4`
- `DefaultReplayWindow=4096`
- `SessionID [16]byte`
- `Encoder.WrapCopies`
- `Decoder.Add`
- malformed / wrong-session / PacketID-wrap / stale errors

wire 保持归档 WGL1：
- bytes 0..3: WGL1
- bytes 4..19: SessionID
- bytes 20..27: uint64 PacketID
- bytes 28..29: payload length
- byte 30: LaneID
- byte 31: reserved zero
- remaining: inner business packet

decoder 使用 bounded seen set；first arrival 立即交付，duplicate suppress；窗口内乱序
unique PacketID 不等待历史 highest。

## TunnelOwner Game API

新增 `internal/datapath/game_owner.go`。

### Session/PacketID ownership

Game state 是 `TunnelOwner` 的 tunnel-wide state：
- SessionID 直接 copy 稳定 16-byte TunnelID；
- encoder/decoder 各一份；
- desired 必须是 2..4；
- DORMANT 只清 transport maps，不清 Game state；
- Close 才释放 Game state。

### GameOutbound

`GameOutbound(packet, now)`：
1. 要求 Game mode 与至少一个 active authoritative lane；
2. leased RoleClient 先执行已有 IPv4 source==lease fence；
3. 校验通过后才分配 PacketID；
4. 对 sorted current active LaneIDs 调一次 `WrapCopies`；
5. 每个 copy 分别进入对应现有 `Lane.Outbound`；
6. 每 lane 仍独立 LINK fragmentation/FEC/record PN；
7. 每份结果再次 `FenceOutbound` generation；
8. 单 lane failure 记录在 result.Failures，不让它抹掉 sibling 成功结果。

业务 flow registry 与 Game fan-out 解耦；OpenFlow 增长不创建 lane。

### GameInboundPayload

receive owner boundary：
1. 先要求当前 authoritative LaneRef/generation；
2. 用该 Lane 完成 record/FEC/LINK decode；
3. 再次 generation fence；
4. parse Game envelope；
5. SessionID 必须等于 owner TunnelID；
6. envelope LaneID 必须等于实际 transport LaneRef.ID；
7. leased RoleServer 在 inner packet 上执行 source==lease；
8. 只有以上均有效后才调用共享 `gamelane.Decoder.Add` 提交 PacketID dedupe。

第 7 步故意放在 dedupe commit 之前：恶意/错误 source 的“先到副本”不能占用 PacketID，
不能让另一 lane 上同 PacketID 的合法副本被误当 duplicate。LaneID mismatch 同样不进入
dedupe。

RoleClient receive 不执行 source fence，所以 server->client 的互联网 source 地址合法。

## 专项测试

`internal/gamelane/gamelane_test.go`：
- 4 lane copies 共用一个 PacketID、envelope 因 LaneID 不同而不同；
- lane3 first arrival，其他 lane duplicate；
- unique PacketID 100/102/101 乱序无 HOL；
- replay state <= window、very-late stale；
- wrong session、duplicate/invalid LaneID、reserved byte fail-closed。

`internal/datapath/game_owner_test.go`：
- 3 active lanes：一个 logical packet fan-out 3 copies；
- 每 lane 各进入自己的 Lane 一次，证明不是 cross-lane FEC；
- lane3先到交付，lane1/2随后 duplicate suppress；
- PacketID 2/3 逆序仍分别交付；
- server->client 8.8.8.8 source 回包不误拦；
- client spoof source 在 PacketID allocation 前拒绝，首个合法 packet 仍 PacketID=1；
- 绕过 client owner 直接封装 source-spoof PacketID=100，server拒绝且合法同ID副本仍交付；
- lane2 envelope 经 lane1 transport 到达被拒绝，正确 lane2 同ID仍交付；
- same-ID replacement 后 PacketID 连续；
- DORMANT 时不分配 PacketID，wake 后继续 namespace；
- 两个 BusinessFlow 注册不增加 active/physical lane；
- Normal single-lane owner 调 Game API 明确拒绝。

## Actions

状态：IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。

按仓库规则没有运行本地 `go test`、`go build`、race、fuzz 或网络实验。
本任务只接受提交后 exact SOURCE_SHA GitHub Actions。

上一已资格产品：
`8d922df623305d169eee581ef02cea8dd6446a26` / Actions `35483525187`。

## 明确未做

- 不迁移 archived `control.go` / membership UDP wire；
- 不迁移 game client/server 独立子进程；
- 不迁移 InnerPacer；
- 不迁移 rawip metadata/service bridge；
- 不改 Game 成跨包条带化；
- 不使用 candidate/retiring headroom 当额外 authoritative logical lanes；
- 不接完整 Linux/Windows/OpenWrt platform；
- 不启用 production padding；
- 不进入 P5/P7。
