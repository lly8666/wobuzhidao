# 20260923-175500 gap index 测试计数修正

## 背景

`610d5092886a160685f8a9d0ecc8fad139fa5bae` 是 sender shadow repair / receiver gap index 的首个实现候选。该SHA已通过 repository contract 和 one-sample policy，但产品测试矩阵首次运行失败。

本日志只记录并修复新增测试自身的计数口径，不修改 sender/receiver 产品实现、wire、参数或验收门槛。

## 原始Actions结果

同一SOURCE_SHA `610d5092886a160685f8a9d0ecc8fad139fa5bae`：

- `next-runtimeowner-recovery` run `35838952098` / job `107109125454`：unit FAIL，race因unit失败而skipped。
- `next-foundation` run `35838952037`：repository-contract job `107109125597` PASS，包含one-sample静态policy；Linux job `107109167644` 与Windows job `107109167567` 均FAIL。
- `next-lifecycle` run `35838952048` / job `107109125496`：FAIL。
- `next-p4-steady-targeted` run `35838952067`：Linux job `107109171211`、Windows job `107109171094` FAIL；lifecycle-focus job `107109171105` PASS；privileged jobs PASS。

上述所有产品测试失败均收敛到同一行：

`TestSteadyGapIndexBudgetExpiryWrapAndFINProtection: gap index steps=129 checks=64`

其他新增定向测试，包括满4096无ACK连续fresh、all-protected fresh旁路、Emit/fresh/ACK/Close并发身份、停流预算清理，均在可执行到的日志中PASS。

## 失败原因

测试在进入tick前先调用了一次 `earliestReceiveSpanLocked()` 验证uint32 wrap下heap头顺序。该helper会按设计增加一次 `GapIndexSteps`。

随后一个tick按 `steadyGapForgiveBudget=64` 连续forgive。每次成功forgive有两次有界heap peek：

1. 读取当前最早后继；
2. 删除/推进后读取下一后继，以继承同一阻塞区间的绝对gap evidence时间。

因此tick本身64次forgive对应128个index step；加上测试前置wrap检查的1次，累计值为129，而 `GapForgiveChecks=64`。原断言错误地拿累计值比较，误报实现超预算。

## 修正

只修改 `internal/runtimeowner/runtime_test.go`：

- wrap顺序预检查后先保存 `before := TransportStats`；
- tick后比较 `GapIndexSteps` 和 `GapForgiveChecks` 的增量；
- 仍要求 `stepDelta <= 2 * checkDelta`，没有放宽产品复杂度门槛。

修正后的测试blob：`9153c7be0ad69c1e3e429a450e17c09e94e0047c`。

## 当前结论

`610d5092` 仍是“实现候选未通过完整Actions”，不能宣称性能改善或最终正确性。当前已知失败是测试计数口径，不是新发现的sender/receiver行为错误；下一提交必须重新跑unit/race/foundation/lifecycle/targeted后才能更新结论。

性能样本仍未启动，旧FAIL和旧runtimeentry race证据保持原样。
