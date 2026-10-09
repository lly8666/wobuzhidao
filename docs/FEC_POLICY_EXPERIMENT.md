# FEC关闭与有限TCP-like修复：同runner串行实验

用户2026-10-09最新授权：**本项实验允许在一个Actions run、一个测量job里，先后跑不同业务/FEC配置**。它是对旧“一run一条”规则的明确窄例外，用于同VM配置内配对比较。禁止同时运行两条负载、禁止跨runner矩阵冒充同机比较。其它优化保护/E6/容量/长测继续原单样本规则。

本文件是方案，不是已实现功能或第二套STATUS。计划状态PLANNED_NOT_RUN。先读[夹具使用说明](REALPATH_TEST_FIXTURE_GUIDE.md)，不得重复开发观测平台。

## 1. 要回答什么

在低丢包下，FEC-off+已有有限SACK/RACK/shadow repair，是否在真实业务交付/p99/无跨业务HOL前提下明显降低CPU与线上字节？高RTT和稀疏尾包是否使恢复延迟不可接受？必要时20:4是否是合理折中？不直接开发自动切换、不提高repair预算、不扩大4096/队列/socket、不恢复严格ARQ、不改codec/wire。

冻结一份共同产品SOURCE；当前候选a2db258b436a41fdee98c6c53abec9bab6ce600f（统一32ms）。开始时确认是否有更新，再选一个经过core/race的共同源；整批不能中途换SOURCE。构建一次两端binary，所有段复用它们，hash一致；每段重启进程/隧道状态。

## 2. 分三批，先做一批，不一次铺满

每段120s有效业务、3s drain，Normal1双向逻辑各10Mbps，profile/timing OFF、padding及tls-startup-padding OFF，outerMTU1400、record cap自动、原repair策略不变。业务独立probe不停止后续发送。120s是探索，不继承300s正式资格。

|批次|损伤/延迟|业务与FEC|段数和用途|
|---|---|---|---|
|A，首先运行|15ms单向，固定0%与1%双向loss|udp/tcp/mixed各比较off、20:20|12段，同一Action串行；纯成本与低RTT低损恢复|
|B，A夹具/数据有效后|300ms单向，固定0%与1%|同上|12段另一个Action串行；观察反馈往返代价|
|C，确有必要再跑|15ms和300ms单向，固定5%|同上|12段另一个Action串行；观察损失与资源边界|

一批约24分钟有效业务，另有首次构建/准备及每段重建开销，通常30..50分钟；timeout给90分钟有界上限，启动时说明段数/预计时长，单段额外timeout与全局deadline。不得中途延长drain或减业务量来凑通过。

A内每个loss/workload一对相邻段，相同seed/尺寸计划/速率；交替off→on、on→off，记录顺序与开始/结束宿主状态。同批是6对、12段，不是12个独立VM证据。首批只筛查；要宣布策略收益，在另两个独立run重复关键mixed配对、反转顺序，形成至少3批可比较的同机配对。宿主配额/压力变化仍要检查，不是同VM就绝对公平。20:4只在两端结果确实需要折中时追加小批，不先铺全部档位。

当前FIXTURE不支持这些参数，先实现最小适配和Actions功能门，再dispatch A。具体参数名字由agent按现有CLI定；不得把上述表伪装成当前已可执行命令。

## 3. 每段隔离与资源重置

只有一个串行runner。编译/安装仅首段前完成；每段独立case目录、case_id、source/helper/config/seed/sequence/order、时间与claim receipt；整个批次另有batch manifest及唯一授权范围。旧perf_sample_guard保持原规则，新batch guard固定可允许的case plan，防止偷偷加段/并行/改源。

- 使用原owned netns/PID/iptables/nft/tc清理逻辑；每段先退出client/server、业务/HTTP/捕获/采样/损伤进程，再清理该段资源。只操作当前实验owned路径和规则，不影响其它任务。
- 重建fresh进程、netns/TUN/路由、seeded netem与会话，清空上段FEC、shadow、RTO、repair credit、flow、lease和计数。不可热切FEC后沿用旧状态，不清全机cache/sysctl来制造结果。
- 同一配对保持内层MTU自动策略一致、外层1400；FEC切换可能改变实际派生TUN/record预算，逐段记录。这是完整策略成本对比，不冒充仅RS算法微基准；辅报真实分片/包长分布解释差异。
- 准备完成后才按固定规则开始计时；每段开始/结束记录CPU型号、核数、Go/kernel/runner image、配额可见性、cpuset/PSI/steal/softirq、socket实际SO_RCVBUF、MTU/MSS。UNKNOWN不是0，也不能把hosted祖先配额看不见写unlimited。
- stop/wait/清理、netns不存在与残余owned进程检查、队列排空和冷却有界验证成功后，才进入下一段。某段业务FAIL保留并可继续采集后续；完整性错误或无法隔离清理则停批，后面标NOT_RUN，不把缺段改通过。
- 默认不打开内部逐record诊断、BPF或100ms额外探针。已有低开销资源采样逐段独立。CPU-s只计正式进程有效业务窗口，助手另列，不计建连/drain/上段。

## 4. 业务负载与观测

UDP复用ordinary混合96/256/512/1000/1372/4068B；独立小probe按明确周期。TCP复用至少4长流+定时短流+HTTP(S)，完整length/hash/证书验证。mixed固定TCP/UDP总预算各占约一半，短HTTP(S)包含在既有预算内；TCP背压时不偷偷补给UDP。120s适配后报告实际流数/请求数，不能机械填300s的304条流。

首批不把65507B支持争议混进主结论。要测大包，单列已知受支持尺寸与拒绝路径；少数4068B失败必须按尺寸单列。稀疏UDP、独立后到96B和受控尾包损失作为下一项小功能保护，不用常规随机loss声称已经测过精确丢片。

每段保留：
1. 真实计划/成功注入/收包字节、阶段goodput、各尺寸UDP sent/delivered/missing/corrupt、TCP完成/hash/背压、HTTP(S)成功。
2. 全部probe请求/返回/超时、returned-only RTT p50/p95/p99、1s/3s期限交付率及未回比例。缺失多时不能拿幸存p99代表所有请求。
3. client/server CPU-s、CPU-s/注入GiB与CPU-s/交付GiB、RSS/HWM、产品和助手资源分开；profile OFF没有alloc/repair/lane计数就写NOT_COLLECTED，不虚构零。
4. 外层尝试/通过/丢失的packet及IP byte两方向账本、实际业务与线上字节放大；固定loss实测和seeded tc版本；不要把qdisc PPS当物理NIC PPS。
5. 原始AF_PACKET/raw/UDP socket和网卡drop差分、资源采样错误、实际MTU/MSS/SO_RCVBUF。若无法同窗口观测wire bytes，则写NOT_COLLECTED，不靠名义FEC比例填数。
6. 10ms持续交付桶、最长空洞、跨业务延迟保护。单个空桶不能证明HOL；重传晚到与阻塞其它业务分开。

当前repair不是可靠交付层：fresh credit约1/5、重复修复信用成本增长、有限shadow/horizon、SACK/RACK及周期条件都在。不得先改这些参数让off变好；当低损off恢复不够，先说明原因/可用范围，不追所有UDP零丢。

## 5. 判定与比较

每段独立原结果：VALID_OBSERVATION/PASS_SCOPE、BUSINESS_FAIL、INPUT_INVALID、CAPACITY_LIMITED_EVIDENCED、UNSUPPORTED、INFRA_INVALID、NOT_RUN；不能沿用20:20/5205阈值套off/1%。无损合法业务仍要求无损坏、无无故丢失；有损UDP允许有限未到，但必须报告期限效率与比例。socket额外drop继续单列资源失败，业务被FEC救回不抹去它。容量证据不足就原因OPEN，不把PSI非零自动当VM免责。

配对比较同SOURCE、同VM、同条件、同投入并同时给交付：CPU收益=1-(off的CPU-s/有效GiB)/(on的CPU-s/有效GiB)，并辅列CPU-s/注入GiB、交付比例和有效吞吐。不同交付量、TCP注入不足、严重压力漂移不能给纯CPU节省结论。每业务类型单列，不平均UDP/TCP/mixed掩盖失败。

线上开销、returned-only p99、全部请求期限交付、残余loss和队列/drop分栏。不能因CPU减少而忽略迟到；不能为低损追零丢升级为严格可靠外层。相同VM也有顺序/时间漂移，首批只报观察与配对差值，不直接升级生产默认。

合理输出：off适用的RTT/损伤/业务范围、20:4候选折中（若测）、20:20收益与成本、哪些结果未确定。自动按loss切换、动态协商、GUI新策略、repair改强或大缓存均不属于本任务。主旨仍真实首次交付/低p99/无HOL/低系统开销优先，基本隔离/完整性/generation/同Seq同wire不变。

## 6. GitHub留痕与结束

在独立 `experiment/fec-policy-sequential-20261009` 分支从最新优化分支开发夹具适配，避免与正在做E1/E4的agent共享源码；先git状态/远端/HEAD核对，不覆盖别人。新workflow仅该明确实验分支、一个job、顺序case；不要改canonical主线或触发原单样本测量。可以复用已有foundation/lifecycle功能门，增加分支触发仅限明确实验分支，不改旧验收内容。

每次修改同提交新增devlog+更新唯一STATUS，将STATUS.working_branch注明实验分支，保留规范branch与原产品失败、E1/E7进度，在active_work.fec_policy_comparison记录本项。每段小artifact与hash，aggregate只读全部段、保留失败/未跑；不上传凭据/密钥/正文。pcap有界并在每段分析后清理，最后also emergency cleanup。校验case数量、SOURCE/helper、实际两端FEC模式、profile OFF和序列性；不要以12段全workflow绿色代替每段真实性。

任务先完成adapter、其Actions功能验收、批次A和完整数字报告。数据有效且确有价值再B/C；不可无限抽好VM或扩展观测平台。本计划PLANNED_NOT_RUN，产品策略/默认未修改。向优化分支交回时只提交审查过的夹具/证据/文档，不把实验资格变成全产品PASS。
