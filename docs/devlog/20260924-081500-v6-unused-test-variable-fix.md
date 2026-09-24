# 20260924-081500 V6新增测试未使用变量编译修正

V6 SOURCE_SHA `81ae365a9d9818c1040fe76334f1fbd62495ee68` 的 `next-runtimeowner-recovery` run `35940027275` 在 `go test ./internal/runtimeowner` 编译阶段失败：

```
internal/runtimeowner/runtime_test.go:1441:2: declared and not used: firstHead
```

这是新增 `TestSteadyRecentlyEvictedReserveRepairsNewCumulativeHead` 中的测试局部变量清理遗漏；尚未执行到任何V6 unit语义，也没有V6性能样本。

V6.1只删除未使用的 `firstHead := wire[0]`。不修改：

- active shadow 4096
- one-shot recently-evicted reserve 1024
- reserve sizing依据
- first-repair SACK/RACK/armed persistence
- 1s RTO / 3s horizon
- fresh/5 credit / 128KiB burst
- FEC / wire / loss threshold
- fresh no-HOL / O(1) active eviction

V6 FAIL及其Actions永久保留。V6.1重新从新exact SHA跑unit/race/targeted/foundation/lifecycle；全绿前不dispatch性能样本。
