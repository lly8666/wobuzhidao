# 20260920-223000 P4 lifecycle readiness test 修复

## 本轮目标和阶段

仍为同一 P4 multi-lane/lifecycle atom。SOURCE_SHA `a696e50287f9b785655a60c2a441e5ba10da8faf` / Actions 35505592364 为 6/7 PASS；旧单lane qualification race 已转绿，仅新 lifecycle race test 的 readiness 条件过早。

## 修改与原因

Game tunnel 的 server egress 规则保持严格：desired lanes 中每条 association 都必须至少接收一个 post-admission steady payload，不能因为任一 Game copy 已成功交付就假定其他 lane 已 detach/attach。

原测试在 wake 后只等：
- client/server ActiveLogicalLanes=2
- client GameLogicalOutbound=3
- server GameDelivered=3

`GameDelivered` 是 tunnel-wide first-arrival 指标；第一条 lane copy 即可让它递增，第二条 lane 的 steady record 可能仍在调度途中。随后立即 reverse egress 会正确返回 `ErrTunnelNotQualified`。

修复：
- `LifecycleServer.TunnelQualified(TunnelID)` 暴露只读 readiness；内部仍用现有 per-lane qualified + `TransportStats.Received` 无等待刷新。
- 集成测试在 reverse egress 前等待该明确状态，而不是固定 sleep。
- 不改变 wire、Game PacketID、owner lane 数、replacement、idle、FEC/recovery 或产品资格门槛。

## 复用来源

无新增归档复用；REUSE_LEDGER 不变。

## Actions证据

SOURCE_SHA `a696e50287f9b785655a60c2a441e5ba10da8faf`
Run: https://github.com/lly8666/wobuzhidao/actions/runs/35505592364

- repository-contract PASS
- Windows active-go-tests PASS
- P2 kernel fallback PASS
- Linux shared-TUN iptables PASS
- Linux shared-TUN nft PASS
- OpenWrt privileged PASS
- Ubuntu full unit/build PASS；`internal/runtimeentry` unit 0.065s
- Ubuntu race FAIL，仅 `TestLifecycleEntryGameReplacementDormantWakeKeepsStableLease`：
  `lifecycle_test.go:243: runtimeentry: server egress blocked until first client steady record`
- fuzz/reference 未因 race failure 继续完成，因此该 SHA 仍不是产品资格。
- `last_tested_source_sha` 继续保持上一资格 SHA `79c6e9dd...`。

## 问题、排查与风险

不能用 tunnel-wide GameDelivered 替代 per-lane sequence ownership 资格，否则可能在尚未完成 steady handoff 的 sibling lane 上过早 server-send。测试修正为观察真实 readiness，不使用 sleep。

## 下一项原子任务

提交本 readiness/test 修复并重跑 exact-SHA 全矩阵。全部PASS后闭包本 atom。
