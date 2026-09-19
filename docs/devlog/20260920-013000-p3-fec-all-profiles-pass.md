# 20260920-013000 P3 FEC 全固定挡位 Actions闭环

## 本轮目标和阶段

对用户要求的一次性 FEC 全固定挡位提取进行精确 SOURCE_SHA 资格闭环。

产品 SOURCE_SHA：

`e8e38d09b98b1898b41ad30335546989bed4f0a5`

Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35454264241

顶层结果：`completed / success`。

## 修改与原因

真正被测试的产品代码仍是 `e8e38d09...`；本收尾提交只更新 STATUS、REUSE_LEDGER 与 Actions devlog，不把文档 HEAD 冒充产品测试 SHA。

本轮已一次性固定并通过：
- FEC off
- 20:4
- 20:8
- 20:10
- 20:12
- 20:16
- 20:20

全部 fixed profile 使用 live FEC v1 56-byte `WF` wire；没有迁移/启用归档 profile-v2 平行 wire。

通过语义包括：
- systematic source `Add` 即发/即 first-deliver；
- full block 发送 R 个 parity；
- partial block 发送 min(dataCount,R) 个 parity；
- parity budget 内恢复；
- A block 缺 source 时 B block source 不等待 A；
- identical duplicate 幂等；
- active conflicting duplicate payload 明确 ErrShardMismatch；
- wrong profile/header mismatch 不污染后续合法 source；
- heavy maxBlocks + compact retired fallback；
- retired state 8192 hard bound；
- completed BlockID 连续历史压缩；
- 3 秒 absolute recovery deadline，进展/重复不刷新；
- 无 inbound 流量时显式 Expire 仍执行 retirement；
- expiry 后 late systematic first-arrival 可交付一次且不复活 heavy parity state；
- LINK fragmentation -> FEC -> LINK reassembly；
- 所有六个 fixed profile 均能修复缺失 LINK fragment；
- 一个逻辑 datagram 跨越 20-source FEC block 时返回 wire ownership 保持稳定；
- FEC path/lane state 不共享。

## 复用来源

归档 SHA：

`b5c848f4e9afdffd15d1bc451560edf4e9390a35`

来源/目标与行为差异已在 `docs/REUSE_LEDGER.json` 逐项登记。特别保留：
- live v1 parity byte；
- archived live policy 的完整固定矩阵；
- fast systematic encoder；
- bounded decoder/retired state；
- 3 秒恢复语义。

明确未迁移：
- profile-v2 平行 wire；
- planner/simulator；
- 动态 FEC；
- old runtime/CLI/DTLS 接线。

## Actions证据

Run 35454264241：

- repository-contract：PASS
- Windows 2022：
  - go list ./...：PASS
  - go test ./... -count=1：PASS
  - go build ./...：PASS
- Ubuntu 24.04：
  - go list ./...：PASS
  - go test ./... -count=1：PASS
  - go build ./...：PASS
  - go test -race ./... -count=1：PASS
  - existing internal/tlsrecord directed fuzz 15s：PASS
  - independent P1 reference vectors：PASS
  - artifact upload：PASS

Artifacts：
- foundation-e8e38d09b98b1898b41ad30335546989bed4f0a5：10587627303
- tlsrecord-reference-e8e38d09b98b1898b41ad30335546989bed4f0a5：10587349743

本地未运行 go test/build/race/fuzz/network experiment。

## 问题、排查与风险

- 本资格是 hosted core/unit/race；不是完整 steady-state FakeTCP + raw platform I/O。
- SourceMTU 当前仍由调用方输入；统一 MTU authority 尚未接入。
- compact retired state 不保留旧 shard payload 做无限期 conflicting-payload 比较，这是维持状态有界的设计；外层 TLS-like AEAD 仍负责 wire integrity。
- wrong lane ownership 由 lane/path owner 隔离，不在 FEC wire 新增 lane ID。

## 下一项原子任务

P3 统一 MTU。定向读取 `old/internal/pathmtu` 及直接 tests/imports，把：
- operator connection MTU
- peer MSS
- actual IP/TCP headers/options
- negotiated record wire limit
- TLS-like 31-byte overhead
- FEC v1 56-byte header
- LINK fragment 20-byte header

收敛为单一预算来源，并覆盖 576/1280/1400/1500/1600/9000 中的合法组合与无效配置拒绝。通过 Actions 后再接必要 session/owner 串接。
