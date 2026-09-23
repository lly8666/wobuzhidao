# 20260923-174000 sender shadow repair 与 receiver gap 索引候选

## 本轮目标和阶段

继续 PERFORMANCE_RECOVERY，执行 `docs/WEAKNET_QUALIFICATION.md` 第10/10.4节。代码基线为 `bbed265de92b613b7c4ce29cf395af77c0336fa9`，该基线已保留 `a5b1a3f` 及其所有后续提交。没有重置分支或覆盖其他agent工作。

本轮目标不是证明历史容量失败的唯一根因，而是消除两个已确认的无界/线性热点：sender满4096时 `evictRepairForFreshLocked` 的双遍历，以及receiver `forgiveGapLocked` 的整map扫描；同时保护fresh优先、3秒repair horizon、FIN/PeerFIN和不可变重传。

## 单样本入口证据

入口修正源码 `bbed265de92b613b7c4ce29cf395af77c0336fa9`：
- next-foundation run `35836702591`：PASS。repository-contract job `107101776268` 通过，包含 `tools/check_performance_workflow_policy.py`；Linux/Windows active-go、privileged和P2 fallback均PASS；历史foundation性能measurement jobs全部SKIPPED。
- next-p4-steady-targeted run `35836702694`：总体FAIL，但 runtimeowner 普通测试和 `go test -race ./internal/runtimeowner` 均PASS。唯一失败来自 `internal/runtimeentry/lifecycle.go:1188` 与 `:1344` 的既存 admission/read data race；Linux race job `107101808423`，artifact `10739507988`。其余contract、Windows steady、lifecycle-focus、iptables/nft privileged均PASS。该失败原样保留，不归因于本轮尚未提交的recovery代码。

`loss-tolerant-v1` analyzer 当前Git blob为 `9bb6d6ec70c88231a07f71f40c108949b3661d69`；旧 analyzer 与旧 FAIL 均未改写。

## 实现候选

### Sender

- 为可淘汰业务备份增加侵入式 `evict` 链，为SACK/retired元数据增加 `retired` 链；满窗淘汰只查看链头，不再每个fresh扫描4096。
- 增加独立按首次发送顺序的 `expiry` 链；每tick最多清理固定预算，保持3秒绝对期限。repair选择也显式拒绝已过horizon记录，清理预算不会把修复期限拖长。
- record进入 `repairInFlight` 时临时移出淘汰候选，Emit返回后仅在 `pending[seq]` 仍是同一record指针时恢复；ACK/SACK并发不能回收正在Emit引用的密文。
- 4096 shadow槽全部受保护时，fresh业务仍直接发送但不保存本条shadow backup，并计 `FreshWindowBypass`；不等待ACK、不返回 `ErrOutstandingBounds`、不伪造确认。FIN/Close仍走独立受保护控制路径。
- 保留既有repair credit/RTO/RACK；过期或预算不足只跳过repair。

### Receiver

- `received` 改为map + 有界最小堆索引；最早后继查询不再遍历整map，插入/删除O(log N)，peek O(1)，最大节点仍4096。
- gap forgive每tick最多固定预算。第一次后继证据形成的期限向同一阻塞区间后续洞继承，避免每跳一个洞重新等待3秒。
- FIN永不作为普通gap-forgive目标；receive metadata满且只有受保护控制边界时，宁可暂时省略业务coverage metadata也不伪造PeerFIN。业务payload仍首次到达立即交付，精确重复可在有空间时恢复coverage。
- 不新增ABANDON/NACK/wire字段，不声称receiver知道sender缓存状态。

### 观测与测试

新增/扩展计数：`RepairEvictionCalls/ScanSteps/MaxScan`、`RepairEvictionNS/MaxNS`、`FreshWindowBypass`、`FreshEmitFailures`、fresh mutex wait/critical时间、`RepairExpiredSkipped`、`RecoveryTicks`、`GapForgiveChecks/GapIndexSteps/GapMetadataDropped/GapExpiredForgiven`。

新增定向测试覆盖：
- 4096长期满、ACK缺失、无SACK连续2048次淘汰，要求每次scan max<=1且fresh持续；
- 全槽位受保护时fresh无shadow发送；
- receiver Seq回绕、160个深洞、固定budget与不重置3秒期限；
- FIN不能被gap forgiveness发布；
- repair Emit期间同时fresh/ACK/Close，校验record身份和密文引用；
- 停流后按预算清理，并证明过horizon不继续repair。

另新增 `.github/workflows/next-runtimeowner-recovery.yml`，只运行runtimeowner unit/race；它不是性能测量，不启动样本。

## 本轮源码归属

本日志所在提交将包含以上候选；在提交完成前无法把提交SHA自引用进自身内容，因此本日志记录基线与文件对象，下一轮Actions回执会写精确SOURCE_SHA。关键候选Git blob：
- `internal/runtimeowner/runtime.go`: `dc102642ee4f1eeb51e0eba08e9fa6015caf6dc7`
- `internal/runtimeowner/indexes.go`: `fde63c07eb43e596bddaf05ad89a13492c1b62a1`
- `internal/runtimeowner/recovery.go`: `316ff27a914aa7f67ba9a15f4d4c6001a1540f0f`
- `internal/runtimeowner/receive_index.go`: `421894eb5edba92008a35caed8b6db9215fbc2b9`
- `internal/runtimeowner/perf_diag.go`: `b75745fe5162a66bbdc460593639bb37c7a1c18a`
- `internal/runtimeowner/runtime_test.go`: `a4bb6553c64362c27c45eca19c0923213f6ab581`
- `internal/runtimeowner/health_test.go`: `b3451df860c815bc6fc5e80e33dd5321672c3525`

## Actions状态与下一步

当前候选编译/unit/race均 `NOT_RUN`；没有本地编译或测试。提交后只看Actions原始结果：
1. 优先检查 `next-runtimeowner-recovery` unit/race；
2. 检查 foundation 和 targeted，区分本轮runtimeowner失败与既存runtimeentry race；
3. 正确性通过后才分别启动Normal10/FEC20:20与Game4/FEC20:20无损性能run，再分别做弱网/突发。任何失败样本保留，不改门槛追溯变绿。

性能整体仍 `IN_PROGRESS`，本日志不宣称吞吐、CPU或弱网资格已改善。
