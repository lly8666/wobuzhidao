# 全新agent可直接使用的FEC SIMD开发提示词

你接手GitHub lly8666/wobuzhidao 的 `next/fec-simd-20261010` 分支，负责完成FEC指令集优化开发及GitHub Actions验收，不只是提建议。先核远端/分支/HEAD/工作区，不覆盖别人，不merge或push规范主线。所有编译、Go/unit/race/fuzz/微基准/功能性能测试只在Actions；本地只编辑、阅读、Git和结果处理。不要从old/或旧日志的“下一步”恢复任务。

先读AGENTS.md、PROJECT_CHARTER.md、docs/STATUS.json顶层active_work.fec_simd/next_task/latest_log、docs/FEC_SIMD_OPTIMIZATION_PLAN.md、docs/AGENT_CONTINUITY.md、docs/REALPATH_TEST_FIXTURE_GUIDE.md，再读MODULE_MAP/WIRE_SPEC/PARAMETERS及相关源码。最新用户已明确：性能第一，我们自己的FEC可以大改。真实业务首次到达、低p99、无跨业务HOL、低CPU/带宽与突发稳定优先，允许适当有界内存；额外安全最后，但完整性/账号地址隔离/固定密文重传/generation/MTU硬门不变。

方案已确定采用MIT的 github.com/klauspost/reedsolomon v1.12.6，Go1.23.12不升级。按方案S0→S5执行：S1先用零值LowLevel.GalMulSliceXor加速生产encoding/recovery共用的GF乘加并保留标量回退；注意该版本LowLevel.WithOptions没有保存options，不能依赖。LowLevel不自动提供GFNI/AVX512；ARM64需native执行证明NEON。S2开发整块多输出编码，以WBD自定义矩阵和单goroutine实例适配，保住partial active/inactive优化；高级内核/恢复替换/有界缓存只按真实热点与收益保留，禁止营销大文件跑分指导小包。如果融合比S1更慢就记录并不用它，用户允许大改不等于必须留下负优化。

保留source立即发送/交付、P全档与partial=min(N,P)、当前32ms调度/size-class、3s期限/compact retirement/迟到systematic一次交付、owned buffer、FEC off快路和现有MTU/大包兼容。内部类型/矩阵/缓存/循环可重构，不重新引入等待填满block或跨包/跨lane排序。注意fastblock_encoder对具体FastReedSolomon类型跳过inactive清理，替换后端必须用明确能力契约保住这项。4096是可放弃shadow，fresh绝不能门控，不改外层repair政策、buffer或默认配置混淆收益。

A固定产品SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f；起点helper/证据父提交543ac2cd2920e9f0fb59fdb38ee6aa3d9a65e56f。B和helper提交分别冻结，核buildinfo/二进制SHA256。复用现有五netns真实业务正式进程夹具；tools/fec_policy_batch.py写死旧branch与单SOURCE off/on，必须新建轻量SIMD AB runner/workflow，不改掉旧证据入口。

用户明确授权**本项同一个Actions单job串行新旧对比**，覆盖旧“一run一条”限制，仅本项例外：A→B→B→A，同工具链、seed、FEC、流量、delay和清理规则，无matrix或并行负载。先core/race与golden互通、fallback/nativeARM/Windows build，然后Q1 mixed Normal1 20:20 300ms单向lossless 10M pilot，再Q批Normal lossless/5205与Game2 5205，L批20:4/20:10低loss及off负对照。每leg120s+3s drain，最终关键300s确认，至少3台runner各自内部配对；按完整方案逐项执行，不自造严格门。同步workflow policy为本分支精确workflow/config push的窄串行例外，不全局放开。

报告client/server及总CPU-s/verified delivered GiB、RSS/HWM、实际注入/交付/按时效率、每尺寸损失/late、p99及未返probe、10ms交付空洞、TCP长短流hash/HTTPS、外层source/parity/repair/PPS和socket/raw drop。CPU型号/ISA、配额/cgroup、PSI/steal分层，未知写UNKNOWN；cross-build不是NEON/ARM性能PASS，capture坏样本不算有效wire。少发、丢更多、profile-on、host不同、统计幸存probe不能冒充优化。理论不足的高loss大包不要求全恢复，但普通业务不得被挡住。

每轮原子commit同时新增详细docs/devlog并更新唯一STATUS，证据进docs/evidence；准确保留FAIL/NOT_RUN/CAPACITY_LIMITED。旧Game4 socket压力、300ms TCP-off收尾、11项RTT历史FAIL、MTU/PMTU及约80秒S2C中断不能由SIMD好样本关闭；80秒按用户决定优化后处理。不要上物理机、不改监控自动化、不向别的聊天发消息。抓包有界，清理owned大raw，留摘要/hash/失败，不输出凭据/业务正文。做完更新精确SOURCE/后端/CPU收益/质量范围/P6 manifest和未完成项，physical保持NOT_RUN，交回原聊天物理复验。
