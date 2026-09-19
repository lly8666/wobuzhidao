# 20260920-065000 P3 显式 record padding Actions闭环

## 最终产品 SOURCE_SHA

`47483a4250ad240d000c5bf0d65e7be6f1af0a97`

GitHub Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35473500019

顶层结果：`completed / success`。

该 SHA 同时包含 padding 实现、owner/pathmtu 专项测试和独立 padded 静态固定向量，因此它是本能力的最终资格锚点。文档收尾提交不会替代该产品 SHA。

## Actions 结果

- repository-contract：PASS
- Windows 2022：
  - go list / unit / build：PASS
- Ubuntu 24.04：
  - go list / unit / build：PASS
  - `go test -race ./... -count=1`：PASS
  - tlsrecord directed parser fuzz：PASS
  - independent `tools/tlsrecordvector`：PASS
  - reference artifact upload：PASS
- privileged P2 kernel fallback 回归：PASS

Artifacts：
- `foundation-47483a4250ad240d000c5bf0d65e7be6f1af0a97`：10593321530
- `tlsrecord-reference-47483a4250ad240d000c5bf0d65e7be6f1af0a97`：10594096705
- `p2-kernel-fallback-47483a4250ad240d000c5bf0d65e7be6f1af0a97`：10594041753

独立 generator 在该最终 SHA 再次输出：

```text
pn7_padded_tailzero_p9 =
1703030028e3d8459f5f18173ec163a7d2d5ab11d076c7e54a0b2032728b93c8712cd3cf74b3987efd151496db
```

源码静态 `TestIndependentReferencePaddedWireBytes` 与它 byte-exact 一致。

## 已通过的 padding 能力

### tlsrecord

- 既有 `Seal(payload)` 默认 padding=0，历史静态 vectors 不变。
- 独立显式 `SealWithPadding(payload, padding)`。
- padding 位于 encrypted plaintext 的 `inner_type` 之后，不修改 record version、kind、PN、nonce、AAD 或 cipher。
- 负 padding 返回 `ErrInvalidPadding`。
- 基础 payload 自身超限仍返回 `ErrPayloadTooLarge`。
- 显式 padding 超过实际 record headroom 返回 `ErrPaddingTooLarge`。
- open 先认证再去尾部零 padding；真实 payload 自身尾零 byte-exact 保留。
- 认证失败不污染 recent PN。
- Sealer 记录 requested/applied padding bytes、padded records 与 failed requests，不逐包打印日志。

### unified MTU / owner

`padding_headroom = record_wire_mtu - 31 - len(actual_record_payload)`。

- 使用已通过的唯一 pathmtu Budget，不新增第二套公式。
- FEC 输出之后才决定 padding；padding 不进入 FEC，不改变 shard original_lengths。
- owner 的显式 PaddingRequest 若超过当前 record headroom，立即选择 0 并计 `PaddingBudgetSkips`；不等待、不排队、不加 timer。
- 不缩小无 padding 的 LinkFrameMTU；满尺寸 source/parity 仍按既有容量直接发送，padding 自动跳过。
- 不为了 padding 多切 LINK fragment，不生成额外 record。
- off、20:4、20:8、20:10、20:12、20:16、20:20 全集合保持同一 lane immutable profile。
- MTU 576/1280/1400/1500/1600/9000、peer MSS 与 negotiated record limit 专项覆盖。
- 丢 A 后 50ms 的 B 仍立即交付，不引入 expected-PN / 跨包 / 跨lane等待。
- 返回 `WireRecord.Wire` 为最终不可变密文；重传使用原 wire，不重新 padding/seal。

## 历史证据保留

历史失败：
- SOURCE_SHA `046d84e11561aef74578b7c1ede3b6e05306b001`
- run 35461114443
- 原因：padding 专项测试字符串静态语法错误，整仓 unit 未通过。

中间成功但非最终资格：
- SOURCE_SHA `1c21817d2cd2ae9158f39a30d70e457b0875493d`
- run 35461256242
- 实现/unit/race/fuzz/reference 全绿，并首次获得独立 padded vector；但源码尚未钉静态 padded vector，所以只记中间资格。

上述历史均保留在 STATUS，没有删除或改写成 PASS。

## 能力边界

本次只证明**显式 padding 编码/解码及其有界 owner 接口正确**。

生产仍默认 `0/off`。P3 没有：
- 自动 padding 策略；
- 按内层 TLS/ClientHello 内容识别或选 padding；
- 假流量、凑包等待、额外 timer；
- 每 tunnel/server 的累计生产预算策略；
- 流量分类器或“检测失败”实验。

因此不能表述为“已防止 TLS-in-TLS 识别”“已降低流量分析识别率”或任何类似结论。真实 HTTPS 长度/方向/突发/时序与资源成本留给 P5。

## P3 退出与 P4

至此 P3 的当前 ROADMAP 范围均有精确 Actions 资格：
- LINK 单数据报 fragmentation/reassembly；
- FEC off + 20:4/8/10/12/16/20；
- 3 秒绝对 FEC retirement / no-HOL；
- unified MTU；
- lane-owned session/datapath；
- 显式有界 record padding。

STATUS 执行位置推进到 P4。

P4 首项按现有方案处理既有长生命周期 lane 的多业务复用与 lifecycle：
- 不增加 lane；
- Normal=1、Game=2/3/4 约束不变；
- 不修改 Game 竞速；
- 不破坏 rotation/generation fencing/DORMANT；
- padding 配置仍默认 off；
- 若接生产配置，必须同时执行每包和累计额外字节预算，无额度立即 0；
- FEC 挡位变更通过候选 lane/incarnation 平滑替换，不在原 block 热切换。
