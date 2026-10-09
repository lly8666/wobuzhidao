# CPU 与真实业务效率优化执行方案

本文件是本优化分支的执行方案，进度仍只有 STATUS.json；不是第二套进度系统。用户2026-10-08授权：先优化CPU/真实交付效率，可适当增加有界内存；MTU沿用其他agent已开发的自动预算，一起压力验收；约80秒下行中断保留OPEN，优化结束后再处理。不得因此宣布全产品通过。

## 1. 目标、基线与不改的边界

- 主旨：真实业务首次到达 > 低延迟/p99/无跨业务HOL/突发稳定 > 较低系统与带宽开销 > TCP/TLS外观 > 额外安全强化。完整性、账户/租约隔离、同Seq相同密文、generation、MTU和有界资源仍为硬门。
- 主线文档基点82c3c614a824b9b033901077aff26127bb6044cc；既往物理产品b4ea061178a6e09b7e7c8587d72b4b8535492567；MTU代码来源68cd1a45fe4d87e48da70eda7a3a19d0016d847b，配套分层测试分支c480ccef30962358b33da936d9ba19def6061f70。本分支合入这些已有改动，没有在方案阶段新增性能产品实现。
- MTU分支是从b4分出的，缺少后来主线实机日志；合并保留两边历史。不可用旧STATUS覆盖五条最新物理样本。
- 本分支集成后的精确SOURCE必须重新冻结并验证，旧b4的实机成绩、c480的分层功能PASS均不能继承为新源码全链路性能PASS。
- b4原生Normal双向各10M：client约275.95 CPU-s/300s、ARM server154.07 CPU-s/300s，p99约74ms、raw drop0但上行缺10包。Game4双向各3M轮换：CPU338.70/166.73，p99约122ms、业务零缺失。不同机器架构、时间、模式、MTU不作因果CPU比较。
- 不重写握手/密码/FEC wire，不恢复DTLS，不做旧架构算法赛。不把外层做成严格可靠TCP。fresh不等ACK、4096、FEC块或其他lane；IP/LINK分片只等待自己的数据报。
- 不以扩大内核socket、队列或强行关闭repair代替优化。已经有FEC有效计算、长度分组、owned密文、增量ACK/SACK/淘汰、Linux就绪批量收发、Npcap就绪batch、Windows异步ACK、TX/RX方向锁；先审实际效果，不能重复发明或撤回。

## 2. 顺序与每步退出条件

一次只实施一类主要热路径改动。父版本、本次产品SOURCE与helper SHA必须精确固定；父资格只用于同条件比较，不给候选继承PASS。先core/race，再普通profile-off性能保护，再下一步。诊断或微基准不能替代正式进程成绩。发现无收益的候选撤销该候选，保留失败证据，不硬凑优化数目。

|步骤|开发内容|完成标志|
|---|---|---|
|E0|审源码、接入新MTU、资格化真实负载助手、建立CPU/包数账本|真实进程无损Normal/Game能正确测量；两个方向路径、rate、SOURCE、有效MTU与资源分类可核验；大包功能/覆盖局限明确|
|E1|按到期唤醒FEC/ACK/repair维护，减少粗粒度延迟和空扫描|稀疏partial parity按实际期限执行，无忙轮询/每包timer；systematic立即发送；下述四个保护场景过门|
|E2|按所有权减少重复分配/复制、复用有界工作区|无ownership/别名/race回归；payload保存到真实最后使用者；alloc/业务MiB及CPU可解释下降，p99不恶化|
|E3|提高已就绪IO批量利用率与收发公平性|真实平均batch、单包回退、syscall/有效MiB改善；小包不等凑包；部分发送/关闭/generation硬门通过|
|E4|降低ACK、shadow索引、修复选择与淘汰固定成本|fresh不门控、缓存满仍前进；无重复全量扫描；同Seq同wire；旧备份放弃不让接收方等洞|
|E5|优化剩余FEC/LINK/record热点和多lane固定成本|仅对E0及更新profile证明的热点工作；不换codec、不改变档位wire；实际partial/复制/包数开销下降|
|E6|冻结组合版本，真实混合弱网、容量、配置、生命周期及长测|同源完整结果矩阵、开放失败、P6 manifest/hash与能力边界齐全；明确下一项是延后的下行中断|
|E7|优化后回到80秒下行断点及剩余迟到，之后物理复验|单独定位并修复，独立资格；未关闭前不标PHYSICAL_PASS/RELEASE_QUALIFIED|

### E0：先把测量做对

复用strict realpath、正式client/server CLI、真实netns/veth/TUN/AF_PACKET、netem与资源sampler。不得仅调用LINK/FEC、mock endpoint或普通内核TCP承载外层稳态。TCP/UDP目标为真实socket，受控私网明确route-mode all，核对目标实际源地址与接口计数。用户业务HTTP/HTTPS正常验证证书与内容；不增加回环转发产品进程。

先只读审核agent/large-mtu-mixed-20261008已经产生的helper、12条样本及失败摘要；能安全复用则按精确文件来源提取，不能重跑同一个样本冒充新进展。其旧MTU和原SOURCE资格不继承到本分支。无法核验的助手自行修正并在Actions资格化。

MTU父日志注明既往12条longmix为FAIL、部分TCP offered不足，且platformflow UDP frame存在8936B能力边界。E0核实原始证据，区分Windows/Linux TUN运输IP片与OpenWrt平台UDP代理；65507B socket兼容或synthetic LINK通过不能替代完整产品，8936/8937两侧也要检查。发现截断属于完整性硬失败，不能放宽；明确不支持的路径应正确拒绝并单列，不能把一UDP消息拆成多个应用消息蒙混通过。未验证的独立截断防护候选不随本计划自动合入。

正式进程使用真实默认调度。已有部分测量测试tick=2/10ms，而正式入口默认100ms；两者是不同配置，不能由前者宣称正式8ms parity发包上限。记录每条样本实际tick/配置和实际到期到发出分布。E1修改前先建立正式入口普通基线。

用独立诊断Action获取CPU分布、alloc/GC、copy/所有权、record/source/parity/repair/PPS、batch与驱动等待账本。profile/逐阶段计时默认off；同源普通off样本必须另跑。不能将on/off差异宣称优化收益。微基准仅解释热点，不能作为容量结论。

### E1：deadline与执行时间对齐

当前partial encoder首源8ms是到期条件，正式100ms周期检查并非实际8ms发出保证。稀疏小尾片和不同尺寸组特别敏感。沿用systematic立即发送、满组即时parity，用每lane或既有owner共享的最近deadline调度；停流仍到期执行，generation退休取消旧任务。

不得为修8ms把整个生命周期循环改成1ms扫描，不为每包建goroutine/timer。有工作才唤醒，同次只做有界到期工作，fresh/receive优先；不能拿定时精度以牺牲CPU换取漂亮延迟。覆盖稀疏/密集/多lane/停流/关闭/过期任务。启动RTO1s、可信稳态最小200ms、3s horizon和保活语义先保留；如果实际deadline测量揭示其他粗粒度问题，只修执行机制，不在这一步混改恢复政策。

### E2：内存可以换CPU，队列不能换延迟

先画包的所有权路径，确认每份buffer谁最后使用，包括AF_PACKET/Npcap批量、encoder借用输出、跨goroutine发送、immutable shadow、重组和TUN。借用buffer必须在下一次复用之前复制/转移；密文一经用于某Seq不可改写。

允许预分配索引/描述符、小型slab和有界buffer pool。不要扩大业务等待队列或无限保留大buffer。新增常驻内存给出每进程、每lane、最多10个incarnation的字节上限；初始软审计预算为baseline RSS再加64MiB/进程，超出时必须给出必要性、真实上限和收益，并在STATUS记录，不能偷偷无限增长。停流/退役后live对象有界，区分pool保留容量与泄漏。

先省确实重复的copy/alloc，不进行全项目零复制重构。利用此前ownership修复的负例测试，保留坏payload/header mismatch/tag错误为0的硬门。

### E3：已有batch要真实有效

审实际batch大小、syscall/message比、Npcap queue路径与单包回退。就绪包可批量，不等待未来包、不随机睡眠。接收批处理有包数或时间预算，ACK/维护/另一方向不能长期饿死；不能把接收线程挂在阻塞emit上。

明确batch部分成功、EAGAIN/中断、关闭和generation切换的语义：已发出的记录不能换密文或作为fresh整批重发；真实IO错误不能吞掉。平台不支持时保持可靠回退。优化Linux不能据此宣称Windows/Npcap CPU下降；Windows hosted能编译/测接口契约，实际驱动收益留物理资格。

### E4：有限外观修复不能抢走业务预算

4096是可放弃shadow备份，不是吞吐窗口。到期/容量淘汰/找不到备份时结束修复，不等、不过度扫描、不重新生产密文。接收端乱序首次交付不等已放弃洞；已有gap metadata retirement和FIN保护不能删。

现有repair新字节额度1/5、128KiB启动credit、100ms defer只作现状审计，不直接改大改小。先统计修复实际CPU/字节、是否修了已经由FEC恢复的业务和突发。不为知道FEC成功再发明跨层ACK协议。区分ACK轻量元数据与昂贵payload备份生命周期；可改索引/摊销回收，不把几万条完整在途payload缓存设为高速运行前提。

500M+高RTT时4096不可能保存全部flight；预期表现是有界放弃与fresh持续前进，不是扩大窗口直至全保存。高缓存压力必须测真实首达、queue年龄、CPU和回收，不能仅看Abandoned多便判吞吐故障。纯ACK/FIN/关闭/候选验证仍保持现有合法规则。Linux同步ACK只有在证明确实阻塞receive后才考虑借鉴现有Windowslatest-only worker，不能盲目开启或为每包建worker。

### E5：只做剩余有证据的热点

当前20:20已有最多三个长度组、RS实际k/r计算优化和owned加密传递。先查大中小混合的partial数量、padding来源、每业务MiB record/PPS及大小组内CPU。不能假定所有档位的实际冗余都是R/20；partial parity=min(k,R)。例如k=1时20:4也可能1源+1校验。

保留全部固定档位、wire兼容和实时source交付。不为了组满等待业务，不更换加密/codec，不进行新旧架构比赛。不会带来真实收益的额外并发不做；每包goroutine、跨laneFEC和全量hash重复校验都不是默认方向。必要并行仅按热点和多核容量证据选择有界批任务，并接受Normal低速保护测试。

## 3. Actions样本协议

每个性能workflow run只有一个源码/配置/seed/场景、一个测量job、一条sample claim；包括微基准、容量、校准、soak和A/B。不同样本分别dispatch；禁止同run matrix、串行A/B/B-A或独立负载同时争用runner。普通unit/race/功能测试可有多job；只读aggregate不生成流量。不得将现有功能矩阵伪称性能矩阵。

每条300s有效业务，建连准备独立记录，固定3s drain不进入goodput分母；原120s正式5205/5305可复用为快速保护，但E0/E6真实混合资格必须300s。一个场景的既定pre/stress/post阶段仍是一条，不属于多样本。

每个E1..E5候选先过相关unit/race和Linux/Windows编译，然后独立跑四保护门：Normal1每方向10M lossless、Normal1 5205、Game4每方向逻辑3M lossless、Game4 5205。沿用同seed、FEC20:20、300ms单向、padding/profile off。父版本同配置的近期有效样本可作为对照，条件不同则分别补。首次1份仅筛查，正式收益结论需至少3个独立、可比资源层的父/候选样本，禁止同runner双测违反用户要求。无证据热点可标SKIPPED_NO_BOTTLENECK并进入下一步。

### 真实业务工作负载

所有速率为每方向逻辑业务总预算，不含Game重复。混合TCP5M+UDP5M合计10M，不是各10M。独立收包线程/流探针，不能send一包等待echo才发下一包。CPU产品/生成器/目标/观测分别计量。

|工作负载|内容|
|---|---|
|UDP-mixed|96/256/512/1000/1372/4068/8972/8973/65507B，以及派生inner MTU对应的payload-1/exact/+1；正常流和允许分片的大包混合，报告各档数量/字节占比|
|TCP-real|至少4独立内层TCP流，长上传/下载与短HTTP/HTTPS请求并存，写块256B/4KiB/64KiB/1MiB；真实TCP分段/MSS/重传/背压，字节hash和关闭尾巴|
|TCP-UDP-mixed|固定TCP/UDP配额，两方向主动源，加独立小UDP探针；某流丢片/重传不影响其它业务首次交付；TCP背压时不偷偷把配额补给UDP|
|sparse-tail|8973/65507等稀疏大包与独立96B，partial组、单小尾片/指定record丢失、无后续SACK；属于一条预定义故障场景，不顺序跑多个独立样本|

UDP按成功业务发送字节限速并记录计划/成功/失败/lag，不能插入65507还保持原PPS。大包占比用固定字节预算，低PPS可能使小尺寸样本不足，必须显示数量；不够就单独延长同场景或另run，不编造p99。

TCP应用write大小不是网络包大小；用实际IP/MSS分布判定是否发生LINK拆包。TCP正常流内有序等待不是外层跨业务HOL。off/FEC不同档位可能改变派生TUN，父候选都按配置读真实值，不偷偷固定旧9000。

### E0与E6矩阵

E0先资格化生成器，再各一条Normal UDP/TCP/mixed无损和Game保护，找基线瓶颈；不要一开始派发全部高带宽组合。

E6正式主矩阵：三个真实工作负载 x 0/5/20/30%外层丢包，300ms单向、300s，每项独立Action；0%和关键失败/修复关键通过至少独立重复一次。再补按仓库5205/5305波形的UDP/mixed及低延迟（15ms单向）sparse-tail，延迟/损伤明确定义、双向实际netem校验。低RTT检查100ms调度/稀疏repair，不能被600ms基础RTT掩盖。

FEC20:20是普通主资格；off和20:12做混合0/5/30%针对性检查，其它固定档位至少真实配置功能有效，不把20:20性能继承。低档恢复量按实际k/r/partial/分片/相关性分析，不能统一零丢包；普通尺寸仍按WEAKNET_QUALIFICATION硬门。用户允许高丢包大UDP无法完整恢复，但不能损坏/截断、拖住其它业务或无限排队。

功能边界单独覆盖派生MTU、DF开/关、65507/65508、非法/oversize TUN输入、拒绝后正常包继续、TCP PMTU/ICMP反馈与屏蔽ICMP表现、不同peer MSS/record cap双向不对称、outer1300/1400/1500及最低有效预算。API预期拒绝不放进正常丢包分母。现有IPv4 floor576可能仍需LINK分片，不承诺所有配置一包一record；IPv6依旧捕获丢弃，不新增IPv6代理。

## 4. CPU收益、虚拟机差异与容量上限

每条记录runner image/架构、CPU型号/逻辑核数/频率可得值、内存、内核、Go版本、cgroup cpu.max/cpuset、虚拟化、steal/PSI/softirq、host busy和产品/助手CPU时间。CPU-s/有效交付GiB和core占用都报；通过丢更多业务省CPU不算收益。低速可另报CPU-s/注入GiB，必须并列交付比例。

不同CPU型号/配额/runner家族分层统计，预定义容量判据并保留所有样本。不能挑最快候选对最慢父版，也不能因无drop便宣称机器没有容量限制。满足输入、捕获、socket/queue和host资源要求后才做可比普通收益。不同宿主无法同机A/B是限制，采用同配置seed、多独立run、资源分层与中位数/离散度；未形成重叠资源层只能写趋势或INCONCLUSIVE。

生成器饱和/明显send lag、socket/AF_PACKET drop、PSI/steal或接收排队说明最早边界，不自动把FAIL改PASS。INPUT_INVALID、CAPACITY_LIMITED、PRODUCT_FAIL、UNSUPPORTED、NOT_RUN与普通PASS区分；容量样本仍保留业务失败，不证明纯环境原因。诊断profile不用于普通CPU收益。

每步默认优化目标：可比交付下CPU/有效GiB降低10%以上，或有可解释的显著p99/突发恢复收益且CPU不升高；这是工程目标，不是承诺。小于噪声的几个百分点不得宣称成功。重复可比普通小探针p99回退超过max(5ms,父中位数10%)先作为优化回归排查；同时满足现有配对RTT门。更多迟到包因恢复改善重新进入统计时，需按全部请求/期限交付率解释，不能仅用returned-only p99机械否决或美化。

缓存/队列年龄、持续10ms零交付桶、1s/3s deadline交付率、全部未回probe与完整性为保护项。p99在样本数量不足或未回比例使其不可确定时标不可确定并给超时数量/下界，不能填0。无损受控场景要求合法业务零损坏/零无故丢失；真实WAN旧残余loss不反向污染Actions无损门。

容量探索最后按25→50→100→200→500Mbps每方向逐级，先lossless再5%；每个速率/配置独立run。上一档明显容量不足即停止更高档，报告本runner可验证上限，不把500M设为hosted必过门。FEC20:20/repair使外层可能超过业务两倍，netem/qdisc带宽需容纳实际放大，不能把限速器制造的瓶颈归CPU。单lane主测，Game按逻辑速率另测。任何独立bypass校准也是另Action，不在产品样本夹带第二次性能测量，更不能用其它宿主校准证明本宿主余量。

## 5. 最终冻结与移交

E6组合SOURCE冻结后：core/race、相关privileged Linux网络与Windows契约、真实配置70、生命周期36+aggregate、现有独立strict18及配对RTT、Normal/Game各1800s target soak、必要共享黑洞、三平台P6按既有门补齐。不得把文档CI绿或12条新混合测试当全产品交付。若容量不足明确范围与尚未验证项，不硬跑无限重复。

每样本保留小summary、精确source/helper/config/seed、实际MTU与buffer、input/probe/hash/完整性、资源和失败原因；必要抓包限时限条数/空间，分析后清理owned大raw，只留摘要/hash/失败。不上传凭据、session secret、业务正文或完整私密配置。

每步一份新devlog，同时更新STATUS、evidence和新增参数目录（如有CLI/JSON变动）。原agent已验证的NoHOL为b4 PASS_ACTIONS，本分支改过热路径仍需相关unit/race回归，不强制重复物理并发NoHOL来阻塞交付。

约80秒单向中断按用户要求为DEFERRED_UNTIL_AFTER_E6，原证据不可删。优化测试若碰到同类失活，保存独立失败、界定该样本无法做普通收益比较，不用优化门掩盖它；必要时在E6声明受其阻塞的资格，转E7。E7查emit tuple/header、raw syscall返回、OUTPUT/路由/conntrack/qdisc到NIC，未确认不归因互联网/FEC。之后才同SOURCE物理复验。保活15s/dead-after90s和初始/稳态RTO不在本计划盲目调小，旧late/root原因仍OPEN。

本方案不承诺CPU提升倍数，也不承诺hosted500M通过；目标是在每一步保住真实业务质量后，获得可复核的更低CPU和有界资源成本。


## 附录：不同虚拟CPU的可比较效率量化规则（2026-10-09）

该段是**测量规则**，不是第2套状态系统。用户要求量化7763/9V45/9V74。公网数据库主要测**整颗物理CPU**，不能直接代表4vCPU hosted Actions来宾配额、虚拟化争用与CPU-s。原始业务源质量始终先于效率指数。

- 外部参照来源：SPEC CPU2017 官方结果（https://www.spec.org/osg/cpu2017/；按SPECspeed单任务与SPECrate吞吐分开）、PassMark CPU Benchmark（https://www.cpubenchmark.net/）、Geekbench Browser（https://browser.geekbench.com/）、公开runner**来宾实测**RunsOn CPU Benchmarks（https://runs-on.com/benchmarks/github-actions-cpu-performance/；第三方提供商自测、存在商业利益，独立核对原始harness）。SPEC整机交叉验证有配置/编译器差异，公网CPU型号不等同来宾可得性能。
- 2026-10-09只读来源核对：PassMark 7763整颗CPU的单线程2517、CPU Mark84492、样本61；EPYC9V74单线程2888、CPU Mark117606、**样本仅2、官方标高误差**；9V45**没有核实到同口径、统计量足够的PassMark型号级样本**，标为NOT_AVAILABLE，绝不用近似9V74/9B45/9R45数字替代。二者整机CPU Mark之比不可用作4vCPU修正因子。SPEC公开7763的裸机报告、RunsOn部分9V74的4/8vCPU基准也不是本次runner虚拟机独立测得值。
- 第一层官方比较量：`Q = Σ(product client+server CPU-s) / Σ(有效按期限到达且校验正确的逻辑GiB)`；必须并列报告全部请求的交付/期限率、最坏持续空桶、p99（缺失时给下界/不可确定）、外层TX/RX bytes/PPS、丢包、有效TUN MTU、FEC/padding以及RSS/PSI/steal/cgroup quota。LOSS、GOODPUT、错误与drop不同则**不作优化因果比较**。不能用总注入或重复Game包作为GiB分母。每源码/同配置/同seed至少3条独立OFF，CPU型号、核数/配额、host busy/PSI/steal/softirq分层并报告分布/异常，不选最佳样本；跨层无重叠 -> `INCONCLUSIVE_CROSS_HOST`。
- 第二层**探索**指标，不是PASS门：若未来独立Action为资源层收集相同版本、可复现的来宾workload-specific速度`S_i`，包含有界加密seal/open、FEC20:20、marshal/解析、收发系统调用吞吐与稀疏单包延迟（分别单线程和固定4 vCPU），并证明每资源层可重复/稳定且无宿主限额干扰，可以给`reference_equivalent_CPU_s_per_GiB = Q × (S_i / S_reference)`。这个比率只对**能代表本产品瓶颈**的速度维度有意义；多维异构无共同单值时应输出向量/区间，而不以任意权重压成高精度总分。速度锚/校准必须为**独立Actions run，一次一场景一测量job**；不得在产品300s样本内夹带基准、改变定时/profile OFF、或假定不同host同一时段可校准。验证不足则该探索指标`NOT_CALIBRATED`，不得把外部PassMark乘在产品CPU-s上宣布收益。
- 任何一个跨CPU归一化分数都**不能抵消**应用丢失、p99变坏、AF_PACKET本地drop、慢首达或跨业务HOL；失真时先修数据质量/可比性。9V45 lossless/166.97 CPU-s与9V74 staged5205/265.02 CPU-s不仅CPU不同，损伤场景也不同，按规则结论始终`INCOMPARABLE`。


### 首选：资源层内版本相对指数（有配对时才跨CPU汇总）

比较软件版本而非机器型号时，不应先根据PassMark把CPU-s乘以整颗CPU分数。固定同一场景／seed／FEC／MTU／profile OFF／业务质量硬门，在每一个重叠资源层`j`（同CPU型号、vCPU/配额、runner与Go版本、host busy/PSI/steal等）分别取得父版与候选**各至少3条独立Action样本**。计算该层每有效GiB CPU-s的中位数`Q_parent,j`、`Q_candidate,j`，其单位无关效率比`R_j = Q_parent,j / Q_candidate,j`。固定预先声明的资源层等权汇总`EfficiencyIndex = 100 × exp(mean_j(log R_j))`；100持平，110表示每单位CPU可做的合格业务约提高10%（相当于同GiB的CPU-s约减少9.09%），90表示变差。正式结果必须附每层样本数、原始值、中位数、离散度与固定层分层bootstrap可信区间，并列全体业务p99/超时、socket drop/吞吐/内存；*只对所有硬门相同且通过的配对层计算*，不得删掉不合格样本求出漂亮指数。不存在共享有效资源层或质量门不齐时指数`INCONCLUSIVE`。外部实体CPU网站只是辅助解释、不是这套比率的分母。不要混合Game与Normal、0loss与5205、profile ON与OFF，或不同运行时间/业务组合。


### E4 外部 recv 系统调用观测的接入门槛（2026-10-09，尚未获正式性能资格）

- Linux `recvmmsg` 入口事件可按目标TGID和 `args.fd` 过滤；退出tracepoint本身不含原始fd，要只对同一OS线程里已捕获目标FD入口的调用配对。不同OS线程之间的`exit→enter`不能当作同一个goroutine停顿；Go goroutine可以迁移OS线程，慢系统调用也可能是正常等待首包。外部 `/proc/PID/net/packet` inode与 `/proc/PID/fd`必须匹配**唯一** AF_PACKET SOCK_RAW接收FD，并二次核验PID出生时间/FD绑定；重用、歧义、权限缺失均禁止继续。
- 非性能资格已包含：真实云runner2s的tracepoint附加、5s局部socketpair两次`recvmmsg`记录，以及在独立netns里正确解析唯一AF_PACKET接收FD；各证据单列，不能合并成产品QUALIFIED。双FD（目标和干扰同PID）功能门固定目标2次/干扰2次，进入/退出计数各2、合法>20ms间隔恰好1。附加和解析失败不能写零事件。
- 下一无业务功能门是目标FD256次、同PID非目标FD256次，在一个5秒小fixture里只核对BPF内部聚合 `enters=exits=256` 且无错配。通过仅证明该固定短时本地输入下无漏记，**不**证明高PPS长时或丢包波形下的漏记率/接收调度影响，也不能通过它计算CPU收益。
- **性能扰动独立门（未执行）**：必须使用独立Actions、一条run一个CPU负载场景/测量job，分开观测器OFF、ON样本，不在同一run顺序A/B；固定fixture、编译器、CPU/vCPU/配额、seed、syscall速率、线程数及网络基础质量，按匹配宿主资源层比较并记录host busy、PSI、steal。预先定义预算和可判定统计置信度，提供syscall速率、合格事件比例、观测器及fixture各自CPU-s、RSS、p99/tails、BPF丢事件指标、BPF map峰值以及stderr/退出码；未匹配资源层或量不足标 `INCONCLUSIVE`，不可拿公网PassMark当同host校准。大于预定扰动预算或存在未知漏事件必须保持`NOT_VALIDATED`，不要进入正式300s产品诊断。
- 若未来获得非扰动支持，再在冻结产品SOURCE外部开启严格有界的`PID+唯一AF_PACKET接收FD`观测，与同源100ms socket skmem.d、逐秒raw_io/read-gap、cgroup及OS schedstat联证；`causal_root`默认仍`NOT_ESTABLISHED`，只有能直接区分阻塞recv/进程停读与到达包突发的证据才考虑最小产品候选。不因偶然干净runner解除原9V45 profileOFF socket drop FAIL，不通过扩大 SO_RCVBUF、mux、shard、ready队列掩盖问题。
