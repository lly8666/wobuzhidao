# 20260920-064000 P3 padding 静态独立向量最终候选

## 起点

同步分支后 HEAD：

`1c21817d2cd2ae9158f39a30d70e457b0875493d`

该 SHA 是 padding 专项测试语法修复后的中间候选。其 GitHub Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35461256242

结果为 `completed / success`：
- repository-contract PASS
- Windows unit/build PASS
- Linux unit/build PASS
- Linux race PASS
- tlsrecord directed fuzz PASS，164538 executions
- independent `tools/tlsrecordvector` PASS
- P2 kernel fallback 回归 PASS

历史失败 `046d84e11561aef74578b7c1ede3b6e05306b001` / run 35461114443 保留在 STATUS，不删除。

## 独立 padded vector 来源

Actions run 35461256242 的 Linux job 使用 `go run ./tools/tlsrecordvector` 生成 reference artifact。该 generator 不 import `internal/tlsrecord`，按 WIRE_SPEC 直接使用 ChaCha20-Poly1305、ChaCha20 header protection 与固定 key derivation 参数计算。

Artifact：
- `tlsrecord-reference-1c21817d2cd2ae9158f39a30d70e457b0875493d`
- artifact ID 10588914159
- artifact zip SHA256 `45c31116f4709d7b9dc83ce4e0e2e46ac13238cb0aae18c39aea438d0676ff17`

独立输出：

```text
name: pn7_padded_tailzero_p9
PN: 7
payload: 41 00 42 00 00
padding: 9
wire:
1703030028e3d8459f5f18173ec163a7d2d5ab11d076c7e54a0b2032728b93c8712cd3cf74b3987efd151496db
```

## 本提交

只修改 `internal/tlsrecord/vector_test.go`，新增 `TestIndependentReferencePaddedWireBytes`：

- 从 `newSealerAtPN(..., 7)` 经公开显式 `SealWithPadding(payload, 9)` 生成 padded record；
- 与上述独立 generator 的静态 wire bytes 做 byte-exact 比较；
- 再用 `OpenRecord` 打开 **静态 expected wire**，确认 PN=7 且尾部带零的真实 payload `41 00 42 00 00` byte-exact 恢复；
- 既有 `TestIndependentReferenceWireBytes` 的全部零 padding vectors 不改，继续证明默认 `Seal` wire 没有变化。

本提交不修改：
- cipher / keys / nonce / PN
- record version/kind
- FEC wire 或 original_lengths
- pathmtu 公式
- padding 策略
- FakeTCP repair / retransmit
- P2 faketcp/realityfront
- P4/P5 lifecycle 或流量外观策略

## 为什么需要第二次 Actions

中间 SHA 1c21817d 已证明 padding 实现、全挡 FEC/MTU/no-HOL/race/fuzz 能通过，但当时 padded vector 只存在于独立 generator 输出/artifact，没有被产品测试源码固定。

因此本提交形成最终候选：只有它自己的精确 SOURCE_SHA 完整 Actions success，才能说“实现和静态独立 padded vector 同时通过”。在此之前 STATUS 仍不标 padding PASS，`last_tested_source_sha` 继续保留上一已正式资格的 P3 owner SHA `05a544818ff04f49332b09779c9ab3a4cf9b09fd`。

本地未运行 build/test/race/fuzz/network experiment。

## 后续

若最终候选 Actions PASS：
1. 记录精确 SOURCE_SHA/run/artifacts；
2. 将显式有界 padding 能力标为 P3 PASS；
3. 明确生产仍默认 0/off，不声称“防止 TLS-in-TLS 识别”；
4. 按 ROADMAP/STATUS 进入 P4 的既有 lane 多业务复用、lifecycle 与可选预算配置，不在 P3 做策略选型或 P5 检测实验。
