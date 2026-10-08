# 可复制给全新agent的任务

下面是启动任务，进度只由STATUS维护，不是另一套当前状态。

你负责GitHub lly8666/wobuzhidao的next/performance-efficiency-20261008分支，按docs/PERFORMANCE_EFFICIENCY_PLAN.md逐步降低CPU、提升真实交付效率，不操作物理机。先检查远端/工作区，依次读AGENTS、PROJECT_CHARTER、STATUS顶层active_work/next_task/latest_log、AGENT_CONTINUITY，再读方案和相关协议/参数。

本分支已合入其他agent的自动MTU预算，不能重做或退回固定9000；它的分层测试不代表持续真实业务性能通过。先执行E0：独立核验集成SOURCE/core/race，审并资格化真实client/server进程的TCP/UDP/混合负载助手，按字节限速并读实际MTU，建立普通profile-off Normal10/Game4x3基线与CPU/PPS/alloc/batch/repair账本。先只读复用现有large-mtu分支证据，不能重复dispatch已有样本冒充新工作。

随后按E1到E5一次一个有证据的优化：到期调度、所有权/有界内存、有效就绪batch、有限ACK/shadow索引、剩余FEC/封装热点。每步相关unit/race及独立Normal lossless/5205、Game lossless/5205过门再进入下一步；没热点可明确跳过，不为优化而改协议。允许适当有界内存换CPU，但不能增加排队延迟、忙轮询、每包goroutine或重复可靠层。

所有开发构建与测试在Actions，每个性能Action run只跑一条样本，禁止matrix/同run A/B或顺序多测。真实socket→TUN→正式加密产品→目标socket；不能用mock/单测替代。CPU型号/配额/steal/PSI和输入/捕获/socket/queue分层记录，多独立样本判断收益；runner容量不足保留CAPACITY_LIMITED，不冒充PASS，不用较快宿主证明优化。

永远真实业务首次到达、低p99/无跨业务HOL/突发稳定优先。完整性、账号/租约隔离、同Seq同密文、generation、MTU与资源有界不变。4096不是fresh窗口，放弃修复不让接收方等洞。FEC systematic立即发，Game竞速、A→A+B→B、idle/keepalive区分、全部参数/GUI/DNS/分流/IPv6/owned清理必须保留。

E6再冻结组合版本，完成方案中的真实TCP/UDP大小包混合弱网、分层容量探索、配置/生命周期/独立strict与长测及同源P6。高丢包最大UDP不强求恢复，但不能损坏/截断或拖住其它业务；missing/late与返回p99并列。80秒下行问题按用户要求延后到E6之后E7，原FAIL/证据保留；优化测试遇到它照实记失败与受限范围，不假装根因已解决。之后交原聊天物理复验。

每轮新devlog与STATUS/evidence同一提交更新，精确SOURCE/helper/config/seed/Actions链接及原判定都留GitHub。不要执行history/old中的旧任务，不删历史失败、不继承旧源码PASS。连续推进本方案，最终交回已优化内容、收益证据、未通过/未跑项和同源包manifest/hash，不写PHYSICAL_PASS。
