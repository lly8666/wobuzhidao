# 唯一开发路线图

当前工作：PERFORMANCE_EFFICIENCY，实际分支next/performance-efficiency-20261008；实时步骤只看STATUS.json。本轮执行[E0..E7方案](PERFORMANCE_EFFICIENCY_PLAN.md)，不恢复历史提示词里的任务。

|顺序|工作|退出条件|
|---|---|---|
|E0|核MTU集成、资格化真实进程助手、普通CPU/包数基线|可核验路径/注入/资源，热点账本及局限齐全|
|E1|最近deadline唤醒、partial FEC/维护调度|稀疏及时、无忙轮询，core/race及四独立保护样本|
|E2|有界预分配与所有权/分配复制|无ownership/race损坏、内存上限、CPU/alloc收益|
|E3|已就绪批量收发与公平性|实际batch/syscall收益、部分失败/关闭语义及延迟保护|
|E4|有限ACK/shadow索引与淘汰成本|fresh持续，容量满/放弃洞无HOL，同wire修复|
|E5|剩余FEC/LINK/record热点|证据驱动，保持档位和wire，收益可复核|
|E6|组合冻结、真实业务矩阵/容量/配置/长测/P6|SOURCE精确、全部失败/能力边界透明|
|E7|优化后查80秒下行及迟到，交原聊天物理复验|独立根因/修复资格；未完成前不标交付|

P0..P7正式阶段仍用于产品资格：P0仓库，P1记录，P2握手，P3数据面，P4平台/生命周期，P5端到端弱网性能，P6同源包，P7物理。历史曾关闭某门不代表新SOURCE继承；本轮处于P5资源优化与P6/P7待验。

不得扩大任务做协议/加密选型、新旧DTLS性能比赛、在线安装升级工具或第二套交接系统。用户已允许有界内存换CPU，但未允许队列膨胀、牺牲p99/完整性或把fresh恢复成ACK窗口门控。

本轮之后：80秒下行断点仍OPEN_DEFERRED；真实物理MTU压力、剩余配置/PMTU/低档尾延迟及当前SOURCE完整资格按STATUS保留。优化期间出现失活不删FAIL，不以健康样本关闭。

历程见[AGENT_CONTINUITY](AGENT_CONTINUITY.md)，整理前路线图在[历史快照](history/20261008-before-efficiency-roadmap.md)。历史快照不可作为当前任务来源。
