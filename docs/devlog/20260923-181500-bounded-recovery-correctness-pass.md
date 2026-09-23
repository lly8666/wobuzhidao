# 20260923-181500 有界 recovery 正确性 Actions 通过

## 范围

本轮只收口 `610d5092886a160685f8a9d0ecc8fad139fa5bae` 的 sender shadow-repair / receiver gap-index 候选正确性。测试计数修正在 `6c4169f7854d2c6b9a00945bb80ea82e3c54b9d5`；没有新的产品代码、wire或参数修改。

## 首轮失败保留

`610d5092` 的首次Actions因新增测试使用累计 `GapIndexSteps` 而误把测试自身wrap预检查的1次peek计入tick预算，出现 `129 steps / 64 checks`。该失败已记录在前一日志，不删除、不重跑覆盖。

`6c4169f7` 只把断言改为比较tick前后增量，仍要求每次gap检查最多两次有界heap peek。

## Exact-SHA正确性证据

SOURCE_SHA `6c4169f7854d2c6b9a00945bb80ea82e3c54b9d5`：

- `next-runtimeowner-recovery` run `35840386207` / job `107113777504`: PASS。
  - runtimeowner unit PASS。
  - `go test -race ./internal/runtimeowner` PASS。
- `next-lifecycle` run `35840386203` / job `107113777375`: PASS。
- `next-p4-steady-targeted` run `35840386132`: PASS。
  - contract `107113777991` PASS；
  - Linux steady/race `107114009235` PASS；
  - Windows steady `107114009244` PASS；
  - lifecycle-focus `107114009402` PASS；
  - iptables `107114009269`、nft `107114009410` PASS。
- `next-foundation` run `35840386237`: PASS。
  - repository-contract/one-sample policy `107113778753` PASS；
  - Linux active-go/race/fuzz/vector `107113831959` PASS；
  - Windows active-go `107113831937` PASS；
  - P2 fallback `107113831957` PASS；
  - privileged nft `107113831943`、iptables `107113831962`、OpenWrt `107113832060` PASS；
  - 历史P5 measurement jobs按单样本政策均SKIPPED。

因此本轮 sender/receiver 实现可以标记为 hosted correctness/race PASS，但没有性能证据，不能从正确性PASS推导容量、CPU、RTT或弱网PASS。

## 第一阶段性能样本定义

正式 `scripts/strict_weaknet_sample.sh` 固定：
- `--fec-parity 20`，manifest `fec=20:20`；
- padding off，MTU 1400，单向300ms；
- Normal资格只允许1 lane / 10Mbps每方向；
- Game资格只允许4 lanes / 3Mbps每方向；
- 场景为 lossless、5->20->5、5->30->5。

按 one-run-one-sample 顺序：

1. `next-strict-weaknet`: mode=`normal`, scenario=`lossless`, seed=`601`, rate_mbps=`10`, lanes=`1`。
2. 独立run：mode=`game`, scenario=`lossless`, seed=`601`, rate_mbps=`3`, lanes=`4`。
3. 无损结果落账后再分别跑弱网，禁止同run A/B或顺序双样本。

analyzer仍为 `loss-tolerant-v1`，Git blob `9bb6d6ec70c88231a07f71f40c108949b3661d69`；旧analyzer和旧FAIL保持不变。

## 当前状态

性能：`NOT_RUN`。没有新artifact、没有吞吐或资源结论。

下一步是对上述第1条执行GitHub Actions `workflow_dispatch`，完成后再读取artifact并更新STATUS/devlog。
