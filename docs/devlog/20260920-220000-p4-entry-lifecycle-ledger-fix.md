# 20260920-220000 P4 multi-lane/lifecycle REUSE_LEDGER 契约修复

## 本轮目标和阶段

仍为 P4 同一原子任务。候选产品 SHA `173f9de06153bed38454adf27ac39a9601754c6a` 触发 next-foundation run 35505359301，但 repository-contract 在进入任何 Go/product job 前失败。本轮只修复复用账本格式，不改 runtime 行为、wire、FEC/recovery 或平台逻辑。

## 修改与原因

`docs/REUSE_LEDGER.json` 原新增两条记录把多个归档 source 和多个 active destination 用分号写进一个字段。repository contract 把字段当单一路径校验，因此报告两个 `Missing reuse source` 和一个 `Invalid reuse destination`。现拆成四条一对一真实路径记录：
- activity_control.go -> runtimeentry/lifecycle.go
- qualification.go -> runtimeentry/lifecycle.go
- replacement_recovery.go -> runtimeentry/lifecycle.go
- serialized_reconnect_test.go -> runtimeentry/lifecycle_test.go

产品代码相对 173f9de0 不变。

## 复用来源

源 SHA 仍为 `b5c848f4e9afdffd15d1bc451560edf4e9390a35`。仅修复 ledger 的路径表达，不新增归档复用范围。

## Actions证据

SOURCE_SHA `173f9de06153bed38454adf27ac39a9601754c6a`
Run: https://github.com/lly8666/wobuzhidao/actions/runs/35505359301

- repository-contract: FAIL
- 错误：`Missing reuse source`、`Invalid reuse destination`、`Missing reuse source`
- foundation artifact: 10603722462, digest sha256:f8cd2117a2df83c55e2cfb810b44992bf84a1809cf87de08a1588bc89a632b2c
- Windows/Linux active-go-tests、P2 kernel fallback、Linux shared-TUN、OpenWrt privileged：全部因 dependency SKIPPED，不能解释为产品失败或PASS。
- `last_tested_source_sha` 保持上一资格产品 SHA `79c6e9dd...`。

## 问题、排查与风险

这是 repository metadata 契约错误，不是编译、单测、race、网络或 runtime lifecycle 失败。修复后必须重新以新 SOURCE_SHA 跑完整矩阵，不能重用 173f9de0 的任何产品资格。

## 下一项原子任务

仍是本 atom：提交 ledger-only 修复，跑完整 exact-SHA next-foundation；有产品 gate 失败则继续最小修复，全部 PASS 后再闭包 P4 multi-lane/lifecycle。
