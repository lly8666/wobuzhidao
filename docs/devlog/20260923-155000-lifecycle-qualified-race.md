# 20260923-155000 LifecycleServer qualification publication race

## 触发证据

SOURCE_SHA `25a0ad7aea674dee6ccb9e7b3d1c0d61170af1d9`，`next-foundation` run **35825736077**，Linux active-go-tests job **107066961358**。

普通 active unit 全通过；`go test -race` 报真实 data race：

- writer: `LifecycleServer.admit`，`internal/runtimeentry/lifecycle.go:1383`，queued post-admission steady record被认证后执行 `fresh.qualified = true`；
- reader: `LifecycleServer.groupReadyLocked`，`lifecycle.go:1702`，由 `TunnelQualified` 在 `s.mu` 下读取 `lane.qualified`；
- 触发测试：`TestLifecycleEntryAutomaticRotationTimerConverges`。

这不是75b5 raw-send性能改动造成的路径变化，但属于最终SHA必须修复的真实共享状态错误，不能按flaky处理。

## 所有权分析

新server lane在admit中先构造 `fresh`，随后在 `s.mu` 下安装到 `group.lanes/byFlow`。安装后，TunnelQualified/RoutePacket等观察者可在 `s.mu` 下读取 `lane.qualified`。

admit之后还会逐个处理admission期间缓存的 `pending` segments。原代码一旦某个segment返回qualified，就直接在锁外把 `fresh.qualified=true`；之后循环结束才调用 `markLaneQualified`。因此“安装后、pending处理未结束”窗口内，字段已经受server mutex保护读取，却被无锁写。

## 最小修复

queued loop不再直接写 `fresh.qualified`。当某segment认证成功时只设置goroutine局部变量 `steadyQualified=true`。全部pending处理完成后，沿用既有 `markLaneQualified(fresh, now)`：

- 获取 `s.mu`；
- 验证 `group.lanes[laneID]` 仍是当前 `fresh`；
- 在锁内发布 `lane.qualified=true`；
- 解锁后按既有逻辑retire replacement。

没有新增等待、队列、goroutine或buffer；没有修改资格判据、首次到达交付、FakeTCP序列、MTU/nonce/FEC/repair、4096、client主导休眠、server PeerFIN跟随、keepalive/missing语义、lease/generation或tls-startup-padding。

额外语义更严格：若queued列表中前一个record已认证、但后续queued处理出现错误并提前返回，不再把半处理lane提前暴露为qualified；只有queued处理完整结束才统一发布。

## 验证要求

因为修改生命周期共享状态：
- `next-foundation` Linux race必须PASS；
- `next-lifecycle` core/race必须PASS；
- `next-p4-steady-targeted`相关race/rotation必须PASS；
- `next-lifecycle-fullstack` 36/36必须PASS。

strict 18性能矩阵已在前一提交改为显式 `docs/qualification/STRICT_WEAKNET_TRIGGER`，所以本race修复不会违反“无损目标仍不稳时不重复整矩阵”的执行顺序。

性能状态仍OPEN；ordered A/B的runner split与Game4 PASS事实不因此改写。
