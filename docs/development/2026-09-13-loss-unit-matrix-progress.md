# Loss-unit 因果矩阵：执行进度与当前证据

更新日期：2026-09-13

本页只记录已经实际运行的 `fec-loss-unit-single-case` Action 和可复查 artifact。完整背景、判决树与接手步骤见：

- `docs/development/2026-09-13-loss-unit-agent-bootstrap.md`
- `docs/development/2026-09-13-loss-unit-review.md`

## 实验共同基线

四个 case 都从同一个 review parent `3b64e281058cc820e9e4c57d8bf35e6d55300b28` 建 sibling run 分支；每条实验只改 `.github/loss-unit-case.json`。因此不同 case 的运行提交 SHA 不相同，但产品恢复算法、诊断代码和实验工具基线相同。

固定 helper：`cd5a78f7fd34df2d83854fbee7b3cf5e2f09632d`。

固定条件：logic64+horizon64 observer overlay、`fec-flush-ms=32`、1 lane、5 Mbps/方向源业务、45s、realistic-mix-v1（平均约482.4B）、300ms单程、iid、FEC20:20。

注意：run 分支提交包含本轮诊断代码/文档/脚本，artifact 另存 `source-sha.txt` 与 `test-overlay.patch`；最终发布资格仍须在修复完成后冻结一个 coherent exact-source candidate，不能拿这些诊断 SHA 直接当发布 SHA。

## 1. 30% / carrier MTU 1500 — 已完成，clean

- 分支：`run/fec-loss-unit-30-1500-a1`
- source SHA：`05e5c4c025dde16fe654805c9da88e5179250e33`
- Action run：`34736788260`
- artifact：`10311710949` (`loss-unit-34736788260-1`)
- workflow/job：`fec-loss-unit-single-case` / `single`
- workflow conclusion：success
- audit：`steady_state_sample_valid=true`
- size errors：0
- `lane_fail=0`
- `dormant_drop=0`

### 业务/资源

- app packet loss：**8.1486%**
- byte loss：**15.3719%**
- goodput：**4.2314 Mbps**
- RTT p99：**666.65 ms**
- timely <=1s：**91.361%**
- host busy：**2.273 / 4 cores**
- UDP InErrors/RcvbufErrors/SndbufErrors：0
- softnet dropped/time_squeeze：0
- client/server netem：**29.988% / 30.104%**
- PeakPending：4096
- PeakBufferedOO：3584
- repair/fresh bytes：17.26%
- RepairEvicted：2514

### 实际 FakeTCP carrier fragmentation

客户端侧：

- payload budget：1460
- datagrams：116659
- fragmented：32013
- fragmented ratio：**27.4415%**
- mean frames/datagram：1.2744

server mux侧：

- payload budget：1460
- datagrams：111251
- fragmented：29361
- fragmented ratio：**26.3917%**
- mean frames/datagram：1.2639

所以“MTU1500 时真实 workload 有约四分之一 datagram 被下层拆片”已经是实测事实，不再是假设。

### FEC observer

link-1：

- settled：2781
- under-required：228 = **8.1985%**
- missing-required：480
- under组平均缺：**2.105**
- reconstruct success/calls：2611/2611
- peak heavy in-flight：64
- pressure retire：166

link-server：

- settled：2774
- under-required：204 = **7.3540%**
- missing-required：506
- under组平均缺：**2.480**
- reconstruct success/calls：2628/2628
- peak heavy in-flight：64
- pressure retire：146

## 2. 30% / carrier MTU 1600 — 已完成，clean

- 分支：`run/fec-loss-unit-30-1600-a1`
- source SHA：`bc2079d561143532fde64541fb4366d6ff698c3f`
- Action run：`34737026409`
- artifact：`10311765375` (`loss-unit-34737026409-1`)
- workflow/job：`fec-loss-unit-single-case` / `single`
- workflow conclusion：success
- audit：`steady_state_sample_valid=true`
- size errors：0
- `lane_fail=0`
- `dormant_drop=0`

### 业务/资源

- app packet loss：**0.3241%**
- byte loss：**0.2740%**
- goodput：**4.9863 Mbps**
- RTT p99：**641.35 ms**
- host busy：**1.374 / 4 cores**
- client/server netem：**30.208% / 30.127%**
- PeakPending：3281
- PeakBufferedOO：3584
- repair/fresh bytes：18.10%
- RepairEvicted：0

### 实际 FakeTCP carrier fragmentation

两端 payload budget 都为1560，且：

- client fragmented ratio：**0%**
- server mux fragmented ratio：**0%**
- mean frames/datagram：1.0

因此1600控制组确实成功消除了当前 workload 的下层 carrier fragmentation；不是“改了MTU但仍然在拆”。

### FEC observer

link-1：

- settled：2865
- under-required：6 = **0.2094%**
- under组平均缺：**1.833**
- peak heavy in-flight：7
- pressure retire：0

link-server：

- settled：2820
- under-required：10 = **0.3546%**
- under组平均缺：**1.500**
- peak heavy in-flight：11
- pressure retire：0

## 30% pair 当前因果判断

这是**强因果支持**，但仍按项目纪律不把一次A/B写成“最终唯一根因”。同时满足：

1. 两条样本都 clean、lane 存活、无 size error；
2. netem 强度相同（约30%）；
3. 1500 下实际 fragmentation 约26–27%；
4. 1600 下 fragmentation 精确降到0；
5. app loss 同步从8.15%降到0.324%；
6. FEC under-required 同步从约7–8%降到约0.2–0.35%；
7. heavy pressure 从 peak=64/大量 retire 降到 peak=7/11、retire=0；
8. CPU压力也下降，而不是用更高CPU换结果。

因此当前最合理的因果链是：

`DTLS datagram 超过1500路径下的 FakeTCP single-carrier budget → carrier fragmentation → 一个上层恢复/业务 datagram 依赖多个下层 loss units → 高loss时有效完整交付概率和恢复余量恶化 → FEC under-required增加 → heavy state/pressure成为次生症状 → 业务残损升高。`

这比此前只凭 `(1-p)^2` 理论交点强得多，因为现在已经直接操纵了 fragmentation 自变量，并观察到 FEC 与业务结果同步变化。

但还需完成20%健康侧对照，并按 bootstrap 计划至少重复关键30% pair一次，再决定产品 wire/source sizing 修复。

**不要把 MTU1600 本身当产品修复。** 它只是同机 veth 因果控制。

## 3. 20% / carrier MTU 1500 — 已触发，等待 runner

- 分支：`run/fec-loss-unit-20-1500-a1`
- source SHA：`7c12096dab8ed1514b41e16dc1336ad095db0f9f`
- target Action run：`34737224438`
- 当前状态：queued（记录本页时仓库为0个in-progress、8个queued，因此是runner/Actions队列状态，不是该job内部失败）

完成后必须先下载 artifact、确认 audit clean，再创建20/1600分支。不要提前并行第四条。

## 4. 20% / carrier MTU 1600 — 尚未触发

严格等待20/1500完成并审计后再触发。

## 下一步

1. 收 `34737224438` artifact，跑 audit v2。
2. 若 clean，再从同一 parent `3b64e281...` 建 `run/fec-loss-unit-20-1600-a1`，只改 case JSON。
3. 四个 audit 齐后运行 `.github/scripts/compare_loss_unit_audits.py` 生成 `loss-unit-matrix.json`。
4. 若20%控制不推翻30%因果链，重复30/1500与30/1600各一次。
5. 重复仍稳定后，进入产品修复设计：**根据真实 DTLS→FakeTCP single-carrier budget约束上游 source/chunk/wire sizing**，而不是提高默认物理MTU，也不是先扩大64 window。
6. 产品修复必须有真实DTLS路径回归，确认关键 encoded/recovery datagram 不再依赖多个carrier loss units；不能只用固定假设overhead的单测。
7. 修复形成 coherent exact-source candidate 后，再独立跑 constant20、constant30、5%→30%→5% 验收。
