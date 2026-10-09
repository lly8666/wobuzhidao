# 恢复后 E4 游戏无损socket突发诊断＋异构CPU比较边界（2026-10-09）

分支 `next/performance-efficiency-20261008` 精确父HEAD `f5da4c854e7b83b9eb348d612664e0103ee8816c`，产品SOURCE固定`ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`。HEAD重核未发现别的agent commit。**不修改产品源码、队列/缓冲区/RTO、物理设备、主线或outer线协议**。第一优先是 [Game4 0loss run37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) 原始`FAIL`本地packet socket drop client33/server86，即使双向UDP103062/103062、probe全部、TCP各304和HTTPS20/20完整也不更名PASS。原带资源artifact 1s采样server drop只在98-99s内跳86，client在154-155s内跳33；单点`ss r0`不排除瞬时突发。原始`capacity_limited_evidenced=false`不许归因host。先补诊断而不重复OFF抽好runner。

唯一正式工作流配置 `.github/efficiency-e0-sample.json`只把`diagnostic_mode=off → on`。维持源、Game4、混合业务、seed1840、loss0、两向逻辑3Mbps、300ms单向、300s+3s、FEC20:20、padding OFF、auto record cap0/实测TUN MTU；触发 **单独一个run/one-fullstack-sample**，Go CPU/诊断计时 ON，记录真实lane 2/4、server pipeline分片age/overflow、raw receive/ACK worker/TCP后台、per-stage drop，若本次没有drop，则不能从“另一runner没发生”得因果结论。ON数据**不能用于CPU获益或与OFF的166.97/265.02 CPU-s直接比**；不得挑runner。运行前/后保护门或工作流原始FAIL均留存，结果尚未取得为NOT_RUN/PENDING。

另按用户请求审查外部量化站点的出处及可比性，并在`docs/PERFORMANCE_EFFICIENCY_PLAN.md`追加**方法附录**，非另建handoff。SPEC官方CPU2017/PassMark/Geekbench/RunsOn公开的CPU基准可作为资源架构描述；PassMark裸物理7763 单线程2517，多线程84492(61样本)，9V74 2888/117606(仅2样本/High error)；未找到同口径可靠9V45聚合值。不可把这些当成4vCPU虚拟机相对处理速率来线性归一，尤其不同loss条件。正式比较固定每有效GiB CPU-s、全部业务质量硬门和资源分层；未来探索来宾同负载专用S系数必须单独Action采、不得搭在正式300s样本上，不能在没有跨资源重叠时硬写收益。资料/方法见[方案附录](../PERFORMANCE_EFFICIENCY_PLAN.md)。

下一步按当前diagnostic原始artifact回填evidence/devlog/STATUS，再决定是否存在具体可修的接收热路径瓶颈；Normal1/Game2保护未放行，CPU收益UNPROVEN，旧Game4 FAIL与纯TCP hash/大UDP边界/E7 80秒下行 OPEN_DEFERRED、E6/P6/physical NOT_RUN不变。
