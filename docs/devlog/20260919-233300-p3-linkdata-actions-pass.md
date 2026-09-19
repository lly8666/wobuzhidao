# 20260919-233300 P3 LINK fragmentation/reassembly Actions闭环

## 本轮目标和阶段

对应 STATUS/ROADMAP P3，对上一开发提交的精确 SOURCE_SHA 做资格闭环，不追加产品代码。

产品 SOURCE_SHA：

`310cdf95a2da601f6a6b4398ce9b8b178aa6a48a`

Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35451984199

顶层结果：`completed / success`。

## 修改与原因

本收尾提交只更新状态、复用账本证据和开发日志。真正被测试的 LINK 产品代码仍是 `310cdf95a2da601f6a6b4398ce9b8b178aa6a48a`，文档收尾 SHA 不替代它。

本次通过的 LINK 最小闭包包括：
- `WBDLFRG1` 20-byte fragment wire；
- non-zero uint32 PacketID，回绕跳过 0；
- 65535-byte 原始 datagram 和 1..65535 fragment count 边界；
- fragment frame 受调用方 LINK MTU 上限约束；
- 乱序重组；
- 相同 fragment duplicate 幂等；
- conflicting metadata / conflicting duplicate 明确拒绝且不覆盖 first arrival；
- 16 incomplete assemblies、65535 buffered fragment entries、`16 * 65535` buffered payload bytes、64 retired IDs；
- assembly 5s absolute TTL、retired 10s TTL；
- sparse fragment storage，声明 count=65535 不做 65535 项预分配；
- owner-callable `Expire(now)`，停流后仍能 retire incomplete datagram；
- PacketID 之间无 expected-window/HOL。

核心 no-HOL 用例已进入 unit/race：大数据报 A 永久缺一个 fragment，随后完整 B 到达，B 在 A 未恢复时先完成重组并交付。

## 复用来源

归档源 SHA：

`b5c848f4e9afdffd15d1bc451560edf4e9390a35`

来源：
- `old/internal/linkdata/datagram_fragment.go`
- `old/internal/linkdata/datagram_fragment_test.go`
- 直接接线参考 `old/internal/linkdata/path.go`

目标：
- `internal/linkdata/datagram_fragment.go`
- `internal/linkdata/datagram_fragment_test.go`

保留旧 wire/行为，移除 `internal/fec` error sentinel 与 Path/FEC runtime 耦合；新增稀疏状态、总资源边界和显式 idle expiry。REUSE_LEDGER 已更新为本 SOURCE_SHA / run 的 PASS 证据。

## Actions证据

Run 35451984199 的 jobs：
- repository-contract：PASS
- active-go-tests (windows-2022)：PASS
  - go list ./...
  - go test ./... -count=1
  - go build ./...
- active-go-tests (ubuntu-24.04)：PASS
  - go list ./...
  - go test ./... -count=1
  - go build ./...
  - go test -race ./... -count=1
  - internal/tlsrecord directed fuzz 15s
  - independent P1 reference vector generator
  - reference artifact upload

Artifacts：
- foundation-310cdf95a2da601f6a6b4398ce9b8b178aa6a48a: artifact 10587371480
- tlsrecord-reference-310cdf95a2da601f6a6b4398ce9b8b178aa6a48a: artifact 10586409782

本地没有运行 Go test/build/race/fuzz/network experiment。

## 问题、排查与风险

- 本轮只证明 LINK fragmentation/reassembly 原子闭包，不等于 P3 完成。
- FEC systematic fast-path 尚未迁移；目前不存在本新根 LINK+FEC 的完整接线资格。
- 统一 MTU 推导、steady-state FakeTCP recovery、session owner、platform I/O 仍未实现。
- hosted Go/serializer 证据不替代真实 raw/Npcap 或物理端到端。

## 下一项原子任务

定向读取 `old/internal/fec` 及直接 tests/imports，提取 lane-local systematic fast-path 最小闭包。首批固定 FEC off / 20:20，保持 source first-arrival 立即交付、3 秒绝对恢复期限、block 之间无 HOL、重复 shard 幂等和损坏拒绝；不在同一原子任务混入 MTU 调参、FakeTCP SACK/RACK、Tunnel/Game 或平台 I/O。
