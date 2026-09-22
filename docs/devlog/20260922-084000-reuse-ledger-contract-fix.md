# 20260922-084000 REUSE_LEDGER contract 修正

## 证据

第3原子同产品源码候选的 targeted Actions：
- run: https://github.com/lly8666/wobuzhidao/actions/runs/35671272520
- result: FAIL / PRODUCT_NOT_EVALUATED
- 唯一错误：`Invalid reuse destination`
- steady-core、race、lifecycle-focus、privileged均因contract失败被SKIPPED。

## 根因与修正

REUSE_LEDGER合同要求每个 `destination` 是一个实际存在的单一路径。我把 `old/internal/faketcp/arq.go` 的复用目标写成分号分隔的三个路径，违反结构合同。

本提交不修改任何产品Go源码，只将该条拆成：
- `old/internal/faketcp/arq.go -> internal/runtimeowner/recovery.go`
- `old/internal/faketcp/arq.go -> internal/faketcp/packet.go`

原有 `repair_horizon.go -> runtimeowner/recovery.go` 条目保持。复用范围、旧参数审计、4096不扩容和old topology不迁移的结论不变。

## 下一步

由本SHA重新触发per-SHA targeted workflow。只有产品jobs实际执行并PASS后，第3原子才关闭。
