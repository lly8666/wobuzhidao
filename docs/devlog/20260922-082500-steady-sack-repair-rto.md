# 20260922-082500 steady SACK / repair credit / RTT-RTO

## 基线和本轮范围

基线 SOURCE_SHA `1cfe913199f7df67f05158e044cbec02771f29a8`。第2原子 steady FIN/RST/half-close 已在 Actions 35669709080 取得定向PASS：repository-contract、Windows active-go、Ubuntu active-go+race、HTTPS base、OpenWrt privileged、shared-TUN iptables/nft全部通过。

本提交只做专项第1节第3项的最小闭包：
- 成熟、有界的选择性确认能力；
- SACK证明已到达后立即释放repair payload；
- fresh业务不被可选repair debt/credit阻塞；
- 有限shadow repair credit与fresh evidence优先；
- 必要的Karn-safe RTT/RTO估计与退避。

首次乱序业务仍立即交付，不恢复严格累计ACK等洞，不扩大4096，不改FEC数学、padding或业务pacing。每ACK/每SACK仍存在扫描与SACK range排序，明确留给紧接着的第4原子做有界增量化；本轮不把它包装成最终低开销实现。

## old专项读取与参数登记

只读取了用户授权的冻结来源及直接依赖/测试：

| 来源 | SHA | 旧值/单位/触发 | 新架构判断 |
| --- | --- | --- | --- |
| `old/internal/faketcp/arq.go` | `0abab4c16235f39caa69c7016a724095fc2e0084` | minRTO=1s，maxRTO=60s；repair credit每5 fresh bytes赚1 byte；burst=128KiB；credit不足defer=100ms；重复repair虚拟成本1x/2x/4x/8x；3个上方SACK触发首个fast repair；重复fast repair要求更新的delivery evidence；RACK窗口=max(10ms,SRTT/4)；SRTT alpha=1/8、RTTVAR beta=1/4、RTO=SRTT+4*RTTVAR | 迁SACK记账、payload释放、fresh-evidence repair、credit和RTT估计概念；不迁旧Controller/DTLS/拓扑。旧60s maxRTO不适用于当前3s有限repair horizon。 |
| `old/internal/faketcp/repair_horizon.go` | `acd91d6b20b1113fed8307ef4f520a12bbc473ed` | active repair 4096；旧tracked tombstone上限=8192 | 只迁fresh-first原则；明确拒绝8192扩容，当前总pending metadata继续硬上限4096，先淘汰SACK-retired tombstone，再淘汰最老可选repair。 |
| `old/internal/faketcp/adaptive_pressure.go` | `af80a90277d311b88ee8f52c7902ed78db46d922` | emergency=4096-512=3584；rate interval=100ms；hole age=1 RTT；soft≈BDP records+max(128,BDP/10) | 已阅读并登记，但本原子**不迁移**。当前gap forgiveness保持现有有限horizon；是否需要adaptive pressure必须由真实路径证据触发，不能一次性把旧参数全搬回来。 |

直接测试/依赖还核对了 `arq_fresh_evidence_test.go`、`bounded_shadow_test.go`、`shadow_repair_policy_test.go`、`rto_episode_test.go`、`sack_coverage_rotation_test.go`、`adaptive_pressure_test.go`、`steady_state_repair_horizon_test.go`、旧 `packet.go`、`steady_state_batch.go`、`window.go`。

旧数值不是推荐值。本轮保留 128KiB / 1:5 / 100ms 只作为受4096和3s horizon双重约束的**临时兼容起点**，后续P5真实路径若证据不支持必须按单一根因修正，不能参数搜索。

## 实现

### RFC2018 SACK wire

`internal/faketcp/packet.go`：
- `Segment`增加最多4个标准SACK block；
- `MarshalSegment`在非SYN ACK/control上编码RFC2018 kind=5并32-bit padding；
- parser解析最多4块；
- SYN option/persona顺序不变；
- steady业务数据segment本轮不piggyback SACK，避免在第5项统一MTU头长核对前偷偷把数据TCP头从20字节扩大。ACK/control可带SACK。

### runtimeowner selective recovery

新增 `internal/runtimeowner/recovery.go`，`runtime.go`接线：
- handoff只有在peer协商SACK后启用；
- SACK证明某pending已到达时立即将payload置nil，只保留小tombstone到累计ACK或4096元数据压力淘汰；
- fresh发送在4096满时优先删除SACK-retired metadata，再放弃最老可选repair state；FIN/control不会被repair debt挤掉；
- credit只约束repair，fresh成功上网后才赚credit；Emit失败会返还预留credit且不推进retry/RTO；
- 首次fast repair要求累计hole之上至少3个SACKed记录；重复fast repair要求更新的delivery transmission evidence并满足max(10ms,SRTT/4)；
- RTT只采未重传、未重复采样记录；RTO=SRTT+4*RTTVAR，当前实现以既有InitialRTO为下限、RepairHorizon为上限。默认因此是1s..3s，而不是照搬旧1s..60s；
- timeout repair成功后退避，累计ACK覆盖该timeout episode后恢复base RTO；
- atom1的selected/attempt/success/failure语义保持：选择不等于发出，真正Emit失败不伪记重传。
- repair segment在真正Emit边界才形成，携带最新recv ACK/SACK；若选中后已被并发ACK/SACK证明到达，则取消repair并返还credit。

### 测试

- `internal/faketcp/packet_test.go`：两块SACK标准round-trip、header length和checksum。
- `internal/runtimeowner/runtime_test.go`：
  - 4个later SACKed记录payload立即释放，并由3+证据fast repair最老hole；
  - repair credit耗尽时repair defer，但fresh仍立即发送；
  - 600ms clean RTT得到1.8s base RTO，1.8s前不timeout，timeout后退避到3s horizon；重传ACK不污染SRTT，覆盖episode后回1.8s；
  - 协商SACK的ACK-only携带block，业务data segment不额外长SACK头。

## REUSE_LEDGER

新增两条：
- `old/internal/faketcp/arq.go` → active `runtimeowner/recovery.go` / `runtime.go` / `faketcp/packet.go`；
- `old/internal/faketcp/repair_horizon.go` → active `runtimeowner/recovery.go`。

`adaptive_pressure.go`只读未迁，因此没有伪造复用条目。

## 调度口径提醒

当前FEC lane可声明 `FlushAfter=8ms`，但正式runtime lifecycle默认 `TickInterval=100ms`，而partial-FEC flush由 `Runtime.Tick -> owner.TickLane` 驱动。因此“配置8ms”**不等于实际每8ms执行**。后续真实路径采集必须报告真实tick/flush执行间隔与queue age，不能把配置值冒充runtime调度证据。

## 尚未完成

本原子仍有两个已知低开销问题，故P4/P5不能关闭：
1. cumulative ACK、SACK apply和pending compaction仍可能扫描数千pending；receiver SACK block形成仍会遍历/排序out-of-order map。这是下一原子第4项的明确目标。
2. 数据TCP实际头长、window/scale/persona/MSS continuity尚未统一进MTU；这是第5项。

本提交创建时Actions尚未执行，不能继承上一SHA结论。
