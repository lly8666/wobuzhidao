# 20260921-235600 runtimeowner repair/gap ACK 原子修复

## 本轮目标和阶段

分支 `next/tlslike-dataplane`，起点 `f714420c763b11b9d20abd0c5b0a36b668e70e32`。对应重新打开的 P4 第1项：核实并修复 runtimeowner tick 中 timer repair 被 gap-forgiveness ACK 覆盖的问题，同时把统计拆为选择、尝试、成功、失败，不能把未实际发出的修复记为已重传。本原子任务不同时迁入 SACK/RTT-RTO、生命周期关闭或参数调优。

## 修改与原因

- `internal/runtimeowner/runtime.go`
  - `tick` 不再用一个可被后续逻辑覆盖的 `emit` 指针同时承载 repair 与 ACK-only。
  - 先选择到期 repair，但不预先推进 `lastSent/retries`，也不预先增加 `Retransmitted`。
  - gap forgiveness 如同时到期，先推进接收 ACK 状态；若 repair 已选中，把更新后的 ACK 搭载在 repair 上，避免 ACK-only 覆盖 repair payload。
  - 新增 `RepairSelected/RepairAttempts/RepairSucceeded/RepairFailures`；只有 Emit 成功才增加 `Retransmitted` 并推进对应 pending record 的发送时间/重试次数。Emit 失败保留原 repair debt，使下次 tick 可立即重试。
- `internal/runtimeowner/runtime_test.go`
  - 增加 repair 与 gap forgiveness 同时到期测试，要求真正发出原 Seq/相同 payload 的 repair，且携带 forgiveness 后的新 ACK。
  - 增加 repair Emit 失败测试，要求失败仅计 attempts/failures，不记 retransmitted、不推进 pending 的 lastSent/retries；随后恢复 Emit 时同一 repair 可再次发出并计成功。

修复保持 no-HOL、4096边界、现有3s horizon不变；没有扩大缓存、恢复严格累计ACK、增加随机等待或修改FEC数学。

## 复用来源

本原子任务未读取或迁移新的 old 代码；只修改当前 active runtimeowner 已有实现。REUSE_LEDGER 无新增条目。

## Actions证据

- 代码提交：`da7a9c492af3351fa9b3ac6dc15b9916762cad7a`
- Actions：<https://github.com/lly8666/wobuzhidao/actions/runs/35612515457>
- 结果：`FAIL / PRODUCT_NOT_EVALUATED`
- 失败发生在 `repository-contract`，明确报错：
  - `Each change needs STATUS update`
  - `Each change needs a new development log`
- 因合同job提前失败，active-go-tests、race及网络回归全部 SKIPPED；不能把该 run 当产品失败或通过。
- foundation artifact：10645822068，workflow 日志记录其上传 ZIP digest `sha256:72aaf3b602cd1971db1167b5481d898dfb27177d21b1daeedfaa389d43eca422`。
- 当前提交只补 STATUS/devlog，不改变上述产品代码；下一 exact-SHA Actions 才给本原子任务产品 verdict。

## 问题、排查与风险

已确认原实现会在 repair 选中后先更新重传统计和RTO状态，再被 gap-forgiveness ACK-only 覆盖，造成“统计已发、线上未发”并错误延后下一次修复。修复将发送状态更新移到 Emit 成功之后。当前尚未取得 Go/race/真实路径回归，不能标 P4 第1项 PASS。

## 下一项原子任务

先让本产品代码在含本日志/STATUS的 exact SHA 上通过 repository contract、Windows/Linux unit/build、Linux race及受影响网络回归。通过后再独立提交第2项：将 FIN/RST/半关闭接入真实稳态 runtimeentry/lifecycle，使用稳态 Seq/ACK，覆盖 replacement/rotation、DORMANT、显式退出与尾部数据，且 detach 不等同网络关闭。
