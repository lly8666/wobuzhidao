# 20260920-040000 P3 显式有界 record padding 实现

## 起点

开始前最新文档收尾 HEAD：

`0265b1412b095baf92dd4e8079dbab5e9a2778e9`

其 Actions 35460798214 已 completed/success；产品 owner 资格仍锚定 `05a544818ff04f49332b09779c9ab3a4cf9b09fd` / Actions 35460675364。

本轮是 owner 完成后的独立 P3 padding 原子任务。P2 CLOSED 不修改；FEC wire/挡位/3秒退役、LINK fragmentation、统一 MTU 公式和默认零填充容量均保持。

## tlsrecord 能力

`Seal(payload)` 默认行为不变，仍固定 padding=0。

新增 `SealWithPadding(payload, padding)`：
- padding 位于 inner_type 之后；
- 使用同一个 kind/version/PN/AAD/ChaCha20-Poly1305/HP；
- negative -> `ErrInvalidPadding`；
- 基础 payload 本身超限 -> `ErrPayloadTooLarge`；
- padding 超当前 record headroom -> `ErrPaddingTooLarge`；
- 成功 open 后完全去掉 padding，payload（包括真实尾部 0x00）byte-exact；
- SealerStats 记录 explicit request、requested/applied padding bytes、padded records 与失败数；
- 默认 Seal 不计作 padding request。

已有 OpenRecord 的 last-nonzero inner_type 解析正好符合 V1 `payload || inner_type || zero padding`，专项测试补齐尾零 payload 与 padded auth failure。

## 统一 MTU

新增 `pathmtu.Budget.PaddingHeadroom(actualRecordPayload)`：

`record_wire_mtu - tlsrecord.FixedWireOverhead(31) - len(actual_record_payload)`

它只消费已经存在的 record 空余，不修改 `LinkFrameMTU`、FEC SourceMTU 或 LINK fragment payload MTU。payload 超预算明确 `ErrPayloadBudget`。

576/1280/1400/1500/1600/9000 均增加精确 headroom 测试，并覆盖 peer MSS / record limit 已限幅后的结果。

## datapath owner 接口

新增 `PaddingRequest{Bytes}` 与：
- `OutboundWithPadding`
- `FlushDueWithPadding`
- `FlushWithPadding`

这里是策略层 desired-padding hook，不是严格 record encoder：
- negative 在进入 FEC 前立即拒绝，避免改变 block state；
- 每个实际 FEC/LINK payload 独立计算 headroom；
- 足够则调用严格 `SealWithPadding`；
- 不足立即 padding=0，并增加 `PaddingBudgetSkips`；
- 不等待 token/其他包；
- 不生成额外 record；
- 不改变 fragmentation/FEC；
- full-size systematic/parity 可原尺寸零 padding 发送。

默认 `Outbound/Flush` 仍调用 `Seal`，不启用任何生产 padding。

owner stats 新增 request/applied bytes、padded records、budget skip 和 negative reject；无逐包日志。

## 所有权和重传

padding 发生在 FEC 输出后、record seal 前。返回 `WireRecord` 仍是 owned immutable bytes，并增加仅本地元数据 `PaddingBytes`。

重传接口不重新调用 padding/seal；FakeTCP 后续 repair 继续持有最终密文。专项测试保留 snapshot 并在后续 seal/FEC 操作后验证字节不变。

## 测试

新增：
- 默认 Seal vs explicit padding=0 wire 完全一致；
- non-zero padding + 真实 payload 尾零 round-trip；
- negative / over-headroom / base payload overflow；
- padded tag 破坏后 valid same-PN 仍可交付；
- fuzz seed 加入合法 padded record；
- unified MTU padding headroom 矩阵；
- FEC off + 20:4/8/10/12/16/20：默认与 padded record open 后 FEC/LINK payload byte-exact，应用 datagram 一致；
- full-size systematic/parity padding headroom=0 -> owner 即时 skip，不多 record；
- 576/1280/1400/1500/1600/9000：padding 不增加 fragment/record 数，wire 不越 record/peer MSS/outer packet；
- negative owner request 不改变 PN/FEC shard state；
- padded A 永久丢失时，50ms 后 padded B 各 FEC 挡位均立即交付；
- padded ciphertext 后续工作后保持不变。

## 独立固定向量两阶段

`tools/tlsrecordvector` 仍不 import `internal/tlsrecord`，仅用 primitive 按 WIRE_SPEC 实现。新增：
- PN=7
- payload=`41 00 42 00 00`
- padding=9
- artifact key `pn7_padded_tailzero_p9`

本提交先让 Actions 生成该独立 artifact。最终资格前必须读取该精确 SHA artifact，把 wire hex 写入静态 Go vector test，然后第二个 SHA 再跑完整 Actions。

因此本日志当前状态 **NOT_RUN / NOT_FINAL**，不能把第一次绿灯直接写成 padding PASS。

## 边界

- 不读取内层 TLS；
- 不识别 ClientHello 大小；
- 不加入 timer、假流量、凑包等待；
- 不实现 P4 per-packet/cumulative/tunnel总 padding 策略预算；
- 不宣称“防止 TLS-in-TLS 识别”；
- P5 真实 HTTPS 外观与成本仍是后续验收。
