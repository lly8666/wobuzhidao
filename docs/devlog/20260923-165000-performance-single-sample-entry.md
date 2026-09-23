# 20260923-165000 性能单run单样本入口落地与交接修正

## 本轮目标和阶段

接手 `next/tlslike-dataplane` 的 PERFORMANCE_RECOVERY，第一个原子任务仅收敛性能测试入口。基线先确认远端包含 `a5b1a3f` 且保留其后 `a67b489`；中断恢复时远端已前进到 `5949570`，其父 `11fa656` 正是单样本入口主体，未回退或覆盖并行工作。

## 修改与原因

- 吞吐/弱网/容量/校准诊断入口改为显式 `workflow_dispatch` 单样本；旧 `next-performance-ab-game` 与混合 correctness+performance 的 `next-performance-recovery` 退役，不再启动测量。
- `next-foundation` 内历史 P5 多样本 measurement jobs 保留源码用于证据追溯，但强制 `if: ${{ false }}`，普通 unit/race 继续多用例执行。
- 新增 `tools/perf_sample_guard.py`：同一 workflow run 的第二次 sample claim 直接失败，并生成 sample identity receipt。
- 新增 `tools/check_performance_workflow_policy.py`：静态拒绝 active measurement workflow 的 push 自动测量、matrix 扇出和多 measurement job。
- 新增显式版本 `tools/check_strict_weaknet_loss_tolerant_v1.py` 与只读 artifact aggregate；旧 analyzer 不改、旧 FAIL 不追溯改绿。新口径按 5/20/30% 配置允许相当业务损失，无损仍零损失。
- 本交接修正补上 analyzer 输出中遗漏的 `analysis_version`/analyzer SHA，使 artifact-only aggregate 能识别样本。

## Actions证据

`5949570aeaea0dba5f4427b90536f84a89ba5ab2` 自动触发：
- next-foundation run `35833620384` / job `107091768927`：FAIL，`check_repository.py` 报 `Each change needs STATUS update` 与 `Each change needs a new development log`，policy 步骤尚未执行。
- next-p4-steady-targeted run `35833620361` / job `107091768756`：同样在 repository contract 处因缺 STATUS/devlog 失败，产品测试未执行。

因此上述失败不作为产品或one-sample policy失败证据；原始失败保留。本提交补齐唯一 STATUS 与本日志，等待下一次 Actions 验证。

## 风险和未验证项

- 单样本入口尚未取得 Actions PASS；所有性能 sample 当前仍 NOT_RUN。
- `loss-tolerant-v1` 的跨独立run RTT增量由 artifact-only aggregate 检查，单个测量run不读取对照样本。
- sender/receiver热路径尚未修改；`evictRepairForFreshLocked` 两轮线扫和 `forgiveGapLocked` received-map线扫仍只是已确认代码路径，尚未用新计数证明其对历史容量失败的贡献，不能写成确定根因。

## 下一项原子任务

先让当前入口契约在 Actions 通过；随后联合修改 `internal/runtimeowner/recovery.go`、`indexes.go`、`runtime.go`：sender满窗固定成本淘汰、receiver有界有序缺口索引/绝对期限、FIN/控制保护、Emit/ACK/Close身份竞态与scan/耗时计数。所有编译、unit、race仅由Actions执行。
