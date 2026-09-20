# 20260920-110500 P4 tunnel-owner production padding budget

## 原子任务

开始时重新同步 `next/tlslike-dataplane`，确认 HEAD 仍为：

`3d5e719b0dcfa995dd8c0d0c2934caf611ff37d4`

与上一 Game evidence closure 完全一致；`STATUS.next_task` 仍要求把 P3
已经通过 Actions 的显式 record padding 能力接到 P4 tunnel-owner 生产策略层。

本轮只做：
- padding 默认严格 off；
- per-record hard cap；
- tunnel-wide 累计 padding bytes cap；
- tunnel-wide padding/useful logical payload 比例 cap；
- 额度不足立即 0，不等待、不凑包；
- Normal sibling BusinessFlow 共用同一预算；
- Game useful payload 只按 logical packet 记一次；
- Game lane copies / FEC / parity 只能消耗额度，不能重复增加 useful credit；
- replacement / DORMANT / wake 不能重置累计额度；
- 不接 CLI/platform，不做 P5 外观结论。

本 atom 没有从 `old/` 提取新代码，因此 REUSE_LEDGER 不新增条目。基础能力来自
active P3 已通过的 `Lane.OutboundWithPadding`、`pathmtu.PaddingHeadroom` 和
`tlsrecord.SealWithPadding`。

## 现有 P3 能力复核

`internal/pathmtu.Budget.PaddingHeadroom(actualRecordPayload)` 给出每个已经形成的
LINK/FEC record payload 的精确剩余 wire headroom；不会缩小 LINK MTU，也不会额外分片。

`internal/datapath.Lane` 的 P3 padding：
- 默认 `Outbound` 使用 `Seal`，padding=0；
- 显式 `OutboundWithPadding` 只使用剩余 headroom；
- headroom 不足立即 0；
- 不多生成 fragment / record；
- 返回 ciphertext immutable，重传复用原 wire；
- Lane stats 已记录 request / actual / skip。

P4 的缺口是：P3 只解决单 Lane 单 request，不能阻止多个 BusinessFlow、Game 多 lane
副本或 FEC parity 各自独立请求而绕过 tunnel 累计成本上限。

## active policy

新增 `internal/datapath/padding_policy.go`。

### TunnelPaddingPolicy

零值：

```text
Enabled=false
BytesPerRecord=0
MaxPaddingBytes=0
MaxPaddingRatioPPM=0
```

是唯一默认路径，不产生任何 padding request。

启用策略：
- `BytesPerRecord > 0`
- `MaxPaddingBytes > 0`
- `MaxPaddingRatioPPM` 为 1..1,000,000

`BytesPerRecord` 是确定性的每 record request，同时也是 hard cap。首版不引入随机长度、
令牌桶、sleep、批处理或参数 sweep：一个 eligible record 要么立即得到该固定字节数，要么
立即得到 0。

累计允许值：

```text
ratio_limit = floor(useful_logical_payload_bytes * MaxPaddingRatioPPM / 1_000_000)
allowed = min(MaxPaddingBytes, ratio_limit)
```

实际已提交 padding 加当前 reservations 不能超过 `allowed`。

策略只能在第一条 transport incarnation attach 前配置；一旦 owner 已识别/attach transport，
再次 ConfigurePadding 明确返回 `ErrPaddingPolicyLocked`。这样 replacement 或
DORMANT/wake 不能通过重新配置把累计计数清零。

### useful payload 口径

Normal：
- source-valid business packet 进入 owner 发送路径时按 `len(packet)` 记一次；
- 所有 BusinessFlow 使用同一 TunnelOwner state。

Game：
- source-valid logical packet 完成 WGL1 WrapCopies 后，只按原始 `len(packet)` 记一次；
- 2/3/4 个 Game envelope copy 不重复记 useful bytes；
- Game 32-byte envelope、LINK fragments、FEC source/parity、record overhead 都不算 useful credit。

因此 transport amplification 不能“赚”更多 padding quota。

## Lane owner selector

P3 固定 request API 保留；另增加 package-internal owner selector：
- LINK/FEC 先形成实际 record payload；
- Lane 用统一 MTU budget 得到该 record 的 headroom；
- selector 向 TunnelOwner 原子 reserve `BytesPerRecord`；
- headroom 不足：0，记录 headroom skip；
- ratio/total budget 不足：0，记录 budget skip；
- reserve 成功后才调用 `SealWithPadding`；
- seal 成功：reservation -> actual padding bytes；
- seal 失败：释放 reservation，不把计划额度记成实际消耗。

reservation 也计入并发可用额度，所以并发 Game lanes 不能同时超卖 tunnel budget。

默认 off 时 selector=nil，Normal/Game 都继续直接走 `Lane.Outbound`，保持零 padding wire 路径。

## FEC / Game 语义

同步随业务 Encode 产生的 FEC parity record 会经过同一个 selector，所以它可以消耗已有额度，
但不会增加 useful credit。

本 atom 没有新增 owner-level timer/repair scheduler。现有底层 `Lane.Flush*` 不被包装成
第二套调度器；未来 platform/runtime owner 接定时 parity/repair 发送时必须复用同一 tunnel
budget selector，且 repair/parity 仍只能消耗、不能增加 useful credit。

Game `GameOutbound`：
- source fence仍在 PacketID 分配前；
- WrapCopies 仍只分配一个 tunnel-wide PacketID；
- useful credit 在 logical packet 层只加一次；
- 所有 authoritative lane copy 共享同一个 allocator；
- FEC 仍 lane-local；
- padding 不创建 lane，不改变 PacketID/dedupe。

## 专项测试

新增 `internal/datapath/padding_owner_test.go`：

1. 默认策略为 off；非法 enabled/disabled 混合配置 fail-closed；首条 lane attach 后策略锁定。
2. Normal 两个 BusinessFlow 共用 ratio + total budget：
   - 20-byte logical payload，25% ratio，10 bytes/record，20-byte total cap；
   - 5 次发送得到 padding 序列 0,10,0,10,0；
   - useful=100，actual padding=20，不能各 flow 独立拿额度。
3. FEC 20:4：
   - 20 个 20-byte logical payload -> 400 useful bytes；
   - 5% ratio 只允许20 bytes padding；
   - full block 产生额外4 parity records，但总 padding 仍20，证明 parity 不增加 credit。
4. Game 3 lanes：
   - 一个 40-byte logical packet 只产生10-byte ratio额度；
   - 3个 lane copies 中只有一份 record 可使用10 bytes；
   - 两个 logical packets useful 只记80，不是 3x。
5. same-ID replacement -> DORMANT -> wake：
   - total cap=20；
   - replacement 前后各消耗10；
   - wake 后即使新 lane 有 headroom，仍因 tunnel cap 得0；
   - 证明 lifecycle 不重置 budget。
6. 默认-off owner 仍不产生 Lane padding request。

## 明确未做

- 不接 CLI/config file；
- 不接 Linux/Windows/OpenWrt platform runtime；
- 不新增 padding timer、token refill、随机长度或 cover traffic；
- 不把 padding 当 payload activity/keepalive；
- 不新增 lane；
- 不改变 Game PacketID/racing；
- 不修改 FEC profile；
- 不做 P5 真实 HTTPS 外观或识别率结论；
- 不进入 P7。

## Actions

当前状态：IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。

按仓库规则，没有运行本地 `go test`、`go build`、race、fuzz 或网络实验。
本轮运行期资格只接受提交后的 exact SOURCE_SHA GitHub Actions。

上一已资格产品：
`7a32b425b66e1018bc536cd3af8bd8e621ed38ce` / Actions `35484254427`。
