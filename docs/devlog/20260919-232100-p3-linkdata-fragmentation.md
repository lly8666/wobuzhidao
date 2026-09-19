# 20260919-232100 P3 LINK fragmentation/reassembly

## 本轮目标和阶段

对应 STATUS/ROADMAP P3。开始分支 `next/tlslike-dataplane`，提交前重新确认 HEAD 为 `4669a5251bf890d14c9564fceac230decf3adccc`。本轮严格只做 `old/internal/linkdata` 的单数据报 LINK fragmentation/reassembly 最小闭包；不接 FEC、MTU 调参、raw socket、Npcap、steady-state FakeTCP recovery、Tunnel/Game 或 CLI。

## 修改与原因

- 新增 `internal/linkdata/datagram_fragment.go`，固定归档已有 `WBDLFRG1` 20-byte fragment wire：version/flags、fragment index/count、original length、non-zero uint32 PacketID。
- 普通合法且不以保留 magic 开头的数据报保持无额外 LINK fragment header；保留 magic 前缀通过 fragment envelope 转义。
- reassembly 以 PacketID 独立管理，不使用 expected ID/window；乱序片可立即进入自己的 assembly。
- identical duplicate 幂等忽略；conflicting duplicate 或同 PacketID 元数据冲突明确报错，并且不覆盖已经保存的首次 fragment。
- 保留归档 16 assemblies / 5s assembly TTL / 64 retired IDs / 10s retired TTL；增加显式 buffered fragment/payload 上限。
- 将旧的按 declared fragment_count 预分配 slice 改为按实际到达片稀疏 map，避免一个 count=65535 的首片触发大额预分配。
- 增加显式 `Expire(now)`，允许 owner 在停止收包后仍按 timer 退役 incomplete datagram；不把 retirement 绑在下一次 Push。
- 新测试固定 wire、PacketID 回绕、长度和 fragment count 边界、乱序/重复/冲突、超时、状态边界、恶意输入隔离，以及核心 no-HOL：A 永久缺一片时，后到完整 B 在 A 未恢复前完成交付。
- 更新 `docs/WIRE_SPEC.md` 记录 LINK fragment envelope 和资源/重复语义。

## 复用来源

归档源 SHA：`b5c848f4e9afdffd15d1bc451560edf4e9390a35`。

定向读取：
- `old/internal/linkdata/datagram_fragment.go`
- `old/internal/linkdata/datagram_fragment_test.go`
- 直接接线参考 `old/internal/linkdata/path.go`

目标：
- `internal/linkdata/datagram_fragment.go`
- `internal/linkdata/datagram_fragment_test.go`

保留：LINK fragment wire、PacketID、乱序重组、重复语义、assembly/retired TTL 与数量边界。

删除的旧耦合：`internal/fec` error sentinel、Path/FEC 编解码与旧 runtime 统计接线；本轮没有搬 `old/internal/fec` 或整个 linkdata runtime。

`docs/REUSE_LEDGER.json` 已新增精确来源、目标、行为差异与测试覆盖。

## Actions证据

本开发日志随首次代码提交一起创建，因此无法在提交内容中自指该提交 SHA。提交创建时状态为 **NOT_RUN**，`docs/STATUS.json` 明确写为“实现完成，等待精确 SHA Actions”，且 `last_tested_source_sha` 仍保持上一轮 P2 产品资格 SHA `079a11a51a8afb7e977f5140d8cdcd21db1c81c5`。

本地未运行 `go test`、build、race、fuzz 或网络实验。代码格式仅做 gofmt；资格只取 GitHub Actions 精确 SHA。

## 问题、排查与风险

- 本轮没有 FEC systematic fast-path；LINK fragment 只提供 FEC 前后的最小业务边界。
- 统一 MTU 推导尚未接入；当前 Fragmenter/Reassembler 只消费调用方给出的 LINK datagram/MTU 上限。
- 没有 steady-state FakeTCP、platform I/O 或完整 client/server 接线，因此不能把本轮视为 P3 整体完成。
- 资源压力下 oldest incomplete assembly 会被 retire，以保持有界；这不允许制造跨 PacketID HOL。

## 下一项原子任务

先等待并核对本提交精确 SHA 的完整 GitHub Actions。若失败，读取具体 job logs 后只修 LINK 问题并保留 FAIL 证据；若顶层 run completed/success，再补 STATUS/devlog PASS receipt，然后进入 `old/internal/fec` 的 systematic fast-path。
