# 自动 FEC：真实工况、理论曲线与有余量验收

2026-10-12 用户修订。适用于 `next/adaptive-fec-aes-tun-20261010` 的 N2；与 `ADAPTIVE_NETWORK_PLAN.md` 合用。本文件覆盖旧文档只验“出现过 RAISE/LOWER”或持续延长干净阶段的 N2 性能收口方式。方案/阈值是待实现验收契约，不能冒充已通过结果；唯一实时进度仍在 STATUS。

## 1. 目标与不可改变的边界

真实业务首次到达效率第一，低 p99、无跨业务 HOL、突发稳定、低 CPU/带宽。自动档必须在保护与开销之间有效折中：可信证据充分时，用较低且足够的档位，不长期保留没有必要的 20:20。允许工程余量与合理弱网残余丢失，不要求追随理想曲线每秒精确匹配。

`off,20:4,20:8,20:10,20:12,20:16,20:20` 是离散保护档，不是线路丢包率。冗余 R/k、总外层字节放大、首次发送线路损失、FEC 后残余、重传后业务残余分别报告，不能互相替代。低档也可能有远高于线路 p 的合理冗余。

保留：独立加密 record、systematic 即时交付、partial r=min(k,R)、32ms partial deadline、3s bounded lifecycle、SIMD/长度分组、同 Seq 同 wire、认证/账号地址隔离/generation、最大封装 MTU、4096 仅 shadow 且 fresh 不门控。只对新 block 切档，旧组/裸 LINK 迟到仍正常处理。统计成熟可等待，业务绝不能等待统计。auto/aggressive 只 Normal，Game 固定；fixed/off 不暗中开启自动控制器。

本轮只改必要 N2 统计/控制/夹具，不重构 RS、不加额外稳态每包字段或主动质量业务探针，不另开算法比赛。N3–N6 保留既定范围，收口后继续。全部构建和测试只在 Actions，最终统一物理验收，不提前部署。

## 2. 已知事实与此次接手点

- 模型曾把窗口边缘 cropped parity 当成未保护业务 source，`e854b93c` 已修正：source/parity 分开，未知真实 source 仍保守，不能把它们全部当满组或删除。
- 精确 SOURCE `3f5c7880679a1b982e29dd07333680fac2cfde03` 的 clean300 run [38160516630](https://github.com/lly8666/wobuzhidao/actions/runs/38160516630)：300ms 每方向、Normal 各10Mbps、100s，两方向实际20→16→12→10→8→4，246711/246711 首次业务交付，probe100/100，RTT p99 600.706428ms。仅该干净场景 scoped PASS。
- SOURCE `90925f81cfb16e8435c238b060c46492eef017ea` 的 off-start recover540 run [38161148715](https://github.com/lly8666/wobuzhidao/actions/runs/38161148715)：0/1/5/20/5各60s，最后0%240s，300ms 每方向、各10Mbps；双向0→4→20。C2S 在240s干净业务阶段仍20，结束后10s drain才降16；S2C 在干净阶段逐步降至off。产物分类 `SCOPED_OFF_START_BOTH_DIRECTION_RAISE_OBSERVED_UNSEEDED_REVIEW_PERFORMANCE`，`not_full_n2_pass=true`，不是完整自适应有效性 PASS。
- 该540s样本各发1332237包，C2S缺389、S2C缺272，全部在初始1%适应段；其余阶段无缺包、无损坏/重复、无整秒零交付，probe539/540、返回者RTT p99 621ms；分析器无capacity flags。CPU 308.65/312.62s，诊断ON，不作跨CPU或诊断OFF优化证据。
- 旧420s/360s不降档、off约2.73s尾部及约80s S2C历史问题保留。N3 AES、N4 Windows TUN、N5 GUI、N6包未开始，物理NOT_RUN。

不要再只把干净阶段240s改480s来宣称成功。先把本次原始记录纳入 evidence，指出源窗口接受效率、M/R上下界、历史年龄、真实k分布和降档阻塞；排空后的切档不冒充负载内及时收敛。

## 3. 理论参考必须独立、分三层

### A. 网络/编码理想参考，不是产品输入

测试侧知道预声明损伤、实际 qdisc drop，以及发送前 source/block/尺寸布局。它可以计算“如果知道当前真实 p 和编码组状况，哪个档位足够”。生产控制器不得读取 netem、夹具 stage、seed、墙钟相位或 oracle 结果来选档。

系统式 MDS/RS、独立同分布擦除下，对一个 source：

`q(k,R,p) = p × P[Binomial(k + min(k,R) - 1,p) >= min(k,R)]`

R=0 时 q=p。q 是 FEC 单独的最终 source 残余参考，不包含外层 repair、迟到期限、系统drop、完整业务包重组与内层TCP重传；尤其不是 p99 延迟保证。

对真实组的 source 加权：`q_mix = sum(k × group_count(k,size) × q(k,R,p)) / sum(k × group_count(k,size))`。同时输出业务字节加权版本，fragmented UDP按实际片数及其相关性单列。source即时到达与parity的实际发出时刻用于恢复时延参考；不能只算最终能解码而忽略32ms/排队/3s期限。

理论普通档 R* 是满足 q_mix<=0.001 的最低允许档；降档裕量参考使用0.0005另列。0.1%是控制目标，不是所有弱网的业务PASS门。若所有档都达不到，则写 `TARGET_UNATTAINABLE`，选档饱和与业务正常继续分别验收。

以下是纯完整k=20、IID、足量样本的示例；不能直接用于稀疏/突发/截断窗口。

| 首发丢包p | 允许off时理论最低档 | 默认min4时最低档 |
|---|---|---|
|0%|off|20:4|
|0.1%|off（恰在目标边界，工程上可保留4）|20:4|
|0.5% / 1% / 3%|20:4|20:4|
|5% / 10%|20:8|20:8|
|15%|20:12|20:12|
|20%|20:16|20:16|
|30%|20:20也未达到0.1%模型目标|同左|

完整k=20、q<=0.001的p边界约为：off0.100%、4档4.709%、8档11.438%、10档14.804%、12档18.049%、16档24.070%、20档29.430%。这些是理想模型的交叉核对，不是产品硬编码线路阈值。

因此默认min4的0→1→5→20→5→0密集IID波形，A层理想目标为 **4→4→8→16→8→4**，普通稳态工程带例为 **[4,8]→[4,8]→[8,10]→[16,20]→[8,10]→[4,8]**，实际从initial20渐降；aggressive围绕普通建议+1，再评价有限余量。允许min0且显式off起步时A层目标为 **0→4→8→16→8→0**，实际会有检测/逐档降档滞后，不要求网络0%瞬间归off。初始和每个变化后的过渡时间带不套稳态档门；真实k/不确定性不同必须在跑前重算。

稀疏k=1且R>=4时实际r=1，各开启档都是q=p²。p=5%时模型残余0.25%，加到20也不增加该组保护。应报告“组形状使目标不可达”，普通自动档不因这个不可达目标无限升档；不能为了省parity使source等待凑组。突发擦除采用预声明的 Gilbert-Elliott/时间黑洞 trace，在测试侧按真实组和发出时刻计算或独立Monte Carlo参考，报告分布/种子/样本量，不把平均p代入IID公式假称突发保障。

### B. 可实现的检测/控制参考曲线

在跑负载前，为每条场景生成一个小型、独立的参考程序/表：输入损伤波形、业务节奏和k分布、报告1.5–2s/变化触发、统计成熟、RTT、有效窗口采样率、512最小样本、fast/slow2s/15s、当前30s稳定/入off60s、10s降档间隔、配置floor/ceiling。推导 earliest/expected/latest 选档时间带和稳态允许档范围。

仅知道 p 变化时刻并不能立即决策。检测延迟预算应分解：形成数据源窗口→报告抵达→统计成熟→回声返回→独立足量样本→EWMA/置信历史退出→稳定/冷却→下一新block提交。并发流水线用事件时间合成，不能把每个时间项机械重复相加。给出理想足量输入下的下界，以及报告丢失/抖动/重排条件下的上界。

例：理想20→4有5次下降，首降稳定30s、其余4次至少10s，因此仅状态机下界约70s，另加必要观测延迟；到off多一次下降且有60s条件，必须按实际稳定时钟求解，不能拍一个统一分钟数。若当前512只靠每报告最多256个可信 PN，则使用有效接受报告速率，不能用业务几千PPS伪装512已瞬间满足。

**独立参考不得直接调用产品 Step/复制实现分支，也不得事后用实测缓慢的接受率重写预声明上界。** 可用已有条件设计 reference；若条件本身造成长时间不肯省资源，需要同时评价设计是否符合目标，而不是把实现的每个HOLD都解释成合理。

### C. 实际产品曲线与输入解释

同一图/表并列：外部已注入p、A层理想档、B层允许时间带、实际普通建议档、实际已提交TX档、该档真正发出的source/parity/字节、M/R首次损失范围、输入新鲜度/有效样本、HOLD理由。C2S/S2C分开。

用只读有界输入trace重放生产控制器，可以检查live接线和重放一致，但不是独立的选档质量证明。oracle只评价产品，不帮助产品；UNKNOWN不能变0，成熟缺失不是精确物理首发loss。观察不到真实首发时明确估计区间，必要反馈有意义，不能用裸ACK缺口/重传比例替代。

## 4. 有余量但不能无限宽的验收

每run之前冻结 SOURCE/config/helper/工况/参考模型版本/理论输出与容差，存入manifest；先校验注入/业务负载/容量，再评估产品。结果分为 correctness、measurement validity、capacity、adaptive effectiveness、resource/performance，workflow绿只说明工作流成功。

### 4.1 稳态保护与开销

对非边界、足量可信样本、容量正常的普通auto：默认允许实际稳态落在 `[R*, next_profile(R*)]` 的区间，按配置floor/ceiling裁剪；置信区间跨多个档时单列输入质量问题，不把整个[4,20]当合格区间。模型边界预声明独立容差。aggressive以普通建议加一档为中心，不在已加一的结果上继续加；其工程余量也不得无限累计。

以预声明最后60s观察段至少90%的真实业务活跃时间位于允许范围作为初始工程screen，同时看source/byte权重，短暂升档和最后drain不改变分母。不是要求每秒一致；若输入不足则 `QUALITY_LIMITED/INCONCLUSIVE`，真实业务若仍良好可单列功能PASS，但“自动省资源”仍OPEN。

统计过度保护时间：`rank(actual)-rank(allowed_upper)` 的正部积分（档位·秒）、连续超范围时长、超出模型需用的parity字节及CPU/GiB。under-protection、切档频率、恢复延迟分别记录。FEC冗余比例分source/parity records与字节，控制填充、TCP头、repair另列；不把总体wire倍率全部归于FEC。

普通独立随机1%完整组长期20:20需要原因和修复，不能只因业务零loss就PASS；5%完整组理论8档，短暂20档允许，稳定后长期20档也不能视为自动控制有效。稀疏各档同样r时，不以选最高档假装达到目标。

### 4.2 曲线时间容差与诊断触发

为每个事件先计算 B层上界 `T_ref`；默认初始screen使用 `max(1.5*T_ref, T_ref+max(10s,4*RTT))`。这只是有根据的工程范围，必须随声明的loss/抖动/反馈可靠性分层，不能对全黑洞或真实样本不足硬套。

密集10Mbps、300ms每方向且没有local drop/反馈故障时，额外设置**设计效率screen**：恢复后可信新干净反馈建立，再观察；约90s仍没有首降、约180s仍远超适合档、允许off但约240s仍不能合理接近off，要给出原因并定位。不是到点必须丢保护，不根据测试真p直接强降；参考模型若证明某档需要更久，必须在跑之前写出原因，若原因是采样/窗口设计过慢，则效率问题OPEN。上述screen可凭跨工况证据修订一次，不追着每个seed改单。

已有90925f81的C2S240s不降、drain才降，记为“数据功能可用、恢复效率未证明/需定位”，不能凭更长最终0段或drain切档直接结案。总墙钟、从真实网络恢复、从首份可信恢复输入三个时钟同时报告，以免把慢检测从性能账中删除。

### 4.3 真实业务、p99与无HOL

无损健康容量的已发送合法业务仍需完整/正确交付，不能降低原完整性门。弱网业务loss不强制等于模型q、不一律0.1%或probe全回；起步off适应损失分事件窗口和总量，不通过长干净尾部稀释。探测missing与返回者p99/p95/p50一起报，单向首交付与往返probe不是同指标。

TCP流hash/完成时间、UDP首次到达goodput和丢包分开，TCP业务自身有序不算隧道HOL。人工永久丢一个旧record/大UDP片段时，独立小业务必须继续；10ms交付桶/最长空洞只用于有持续输入的阶段，不能把稀疏/休眠本来没发包算HOL，也不能凭整秒零交付=0声明毫秒级HOL已排除。

p99用同SOURCE同夹具无损基线和预声明模型恢复/排队预算评价，包含最大空洞与tail事件；<=5ms的绝对噪声带可用于低RTTscreen，持续多秒尾部不当噪声。过载、不足测量/未跑单列，完整性、fresh门控、清理、认证/MTU/generation等硬门不放宽。

## 5. 工况清单：先冻结，再分批单条 Actions

不做30分钟单run大matrix。每个性能Action一个SOURCE/配置/seed/场景、一measurement job；单配置预声明波形是合法一条场景。纯模型/核心功能unit/race可同workflow多job，但不叠多个测量负载。同一场景不同RTT/initial/floor/cipher是不同Action，不在一个run中途改策略。

优先顺序与预声明建议如下，精确持续时长由 B层推导；若连允许上界+60s稳态都放不下，先解释设计效率，不随意无限延长。普通默认initial20/min4、ChaCha、外层MTU1400、两端record-cap默认auto，Normal业务每方向10Mbps，除表中明确变更外配置不变。业务使用夹具合法大小包混合，不能只测64B探针。

|优先级/ID|场景|要验证的现象|
|---|---|---|
|P0 M0|独立理论表、参考曲线生成与controller功能/race|公式边界、k=1/partial、从off pilot、aggressive+1、状态/年龄、模型不可达、same-wire新组切档|
|P1 C01/C02|干净0%，15ms或300ms每方向，分别独立run|默认20起步双向降至低档，理论时间带和CPU/字节基线；已测旧SOURCE只作来源|
|P1 C03|300ms/方向，0→1→5→20→5→0波形，initial20/min4|沿理论阶梯保护，弱网恢复有效降档；不是只找一次RAISE/LOWER|
|P1 C04|同波形但显式initial0/min0，独立run|off探测→保护、起步损失/升档时延、恢复到off；默认配置不隐式改|
|P1 C05|300ms/方向，C2S1%/S2C10%稳定，其他同默认|上行低档/下行较高档，不串方向；预留收敛+末60s稳态|
|P1 C06|C2S10%/S2C1%，独立run|反向重复验证方向归属，不依赖客户端标签|
|P2 C07|15ms每方向的同一损伤波形，独立run|不能只有高RTT生效；RTT影响检测延迟而非随意提高稳态FEC|
|P2 C08|普通steady 1%完整/实际partial大小包，独立run|长时间20:20算开销问题，参考p+组形状判合理档|
|P2 C09|普通steady 5%，独立run|足量后接近理论档，稳定冗余与剩余损失|
|P2 C10|相同平均5%的相关丢包trace/GE，独立run|与C09区分，burst参考分布/恢复时延，不能套IID零loss|
|P2 C11|预声明20/100/300ms全包突发黑洞，间隔>=20s，独立run|短暂丢失不卡死，不能靠高档救全黑洞或永久留下高档|
|P2 C12|有界重排/延迟抖动，0或低真实loss，独立run|晚到不大量误判丢包，业务立即交付，升档不被ACK重排驱动|
|P2 C13|质量反馈subtype单独丢失/延迟，数据损伤固定，独立run|UNKNOWN/HOLD及恢复；不得把所有反向数据一起丢当“仅反馈损伤”|
|P2 C14|0.1/0.5/3/10/15/20%平稳loss选档表|模型敏感边界在Actions核心独立表全覆盖；性能只选1/5已测+10或20一条有价值代表，不必每率都300s|
|P2 C15|稀疏64..256B 1..10包/s和交互短流，独立run|真实k小、样本不足/不可达不盲满档、不为凑512阻塞、不加主动业务探针|
|P2 C16|预声明idle→新业务、rotation和反馈代次功能|关闭业务RTTprobe用于idle段；quality/keepalive不刷新payload时钟，不误关/误唤醒，旧报告不能改新generation|
|P2 C17|auto-aggressive的对应波形，独立run|与普通建议+1且clamp，同输入trace功能证明；不同runner性能不作为严格差一档证据|
|P3 C18|UDP大小包+多条短/长TCP+HTTPS同跑，outer1280及合法超inner MTU，独立run|按总双向业务预算计速，真实分片/重组/MTU边界、独立小流不被缺片大UDP阻塞；30%大包不强求全恢复|
|P3 C19|Game4 fixed20:20，每方向逻辑3Mbps|自动优化不得回退固定Game、竞速去重和资源；至少所改SOURCE的功能+代表弱网保护|
|P3 C20|10→20Mbps或有界CPU/链路服务降速容量诊断，独立run|识别host/queue/drop，确认不是盲加parity正反馈；容量不足不冒充auto协议FAIL或PASS|

建议波形初稿（尚未dispatch，M0计算后冻结）：C03的0/1/5/20/5各120s、最后0%180s，总780s+10s drain，RTT/业务不变；C04以同前五段加最后0%240s，总840s+10s drain。这样变化后可有过渡及末60s稳态观察，而不是60s一变、控制器尚未收敛又换损伤。若M0证明更短已充分可缩短，但上界必须预先明确，不能测完再加尾段。已有540s先只读定位，理论/功能不过关不先dispatch上述长样本。

相关损伤初稿也先冻结可解释参数：C10用时间驱动GE，good损失0%、bad损失50%、mean-good450ms/mean-bad50ms，长期bad占10%、期望平均loss5%，同时核实际loss/突发时长；用1ms时间步或等价状态时刻trace，不能偷换成未说明的每包转移。C11在固定背景0%时每20s按预声明循环注入20/100/300ms双向全包黑洞。C12高RTT300ms基础叠加±50ms有界抖动、5%包额外滞后100ms及1%重复，真实loss0%，注入方式/实际作用逐项确认，不把重排算loss。C13只drop质量控制subtype（例如30s期间30%随机丢、后20s100%丢、恢复），保留业务数据；若只能在正式Emit处做有界acceptance fault，明确test-only范围且独立核验，无可分离能力时写UNSUPPORTED，不能用反向整条netem黑洞代替。以上都是一条固定策略的预声明损伤工况。

只对核心关键变化和异常场景做2次独立重复；容量正常却方向不一致不能用多seed挑好结果。每次新源码先core/race，再最小对应真实场景，基本通过后扩大；两次同失败无新证据就缩小边界。旧C03/C04只延长尾段的现有样本可以用于定位，但不替代新预声明曲线资格。

## 6. 模拟真实环境与机器差异

复用现有五netns正式二进制真实路径、`tools/adaptive_n2_wave300.py`、`adaptive_n2_offstart_wave300.py`、`adaptive_n2_offstart_recover540.py`、stage helper和`realpath_udp_duplex.py`，以及`REALPATH_TEST_FIXTURE_GUIDE.md`。优先增强共同统计/参考manifest，保持旧夹具和原始判断；新schema/参考版本明确，不静默重判历史FAIL。

netem必须逐方向逐阶段核实际drop、delay/backlog和stage helper退出码。内核不支持seed则保留 `UNSEEDED`，业务seed≠网络随机seed；不假称确定性同trace。不以qdisc全局drop证明某个source集合精确p，也不拿抽样低流量的瞬间p当长稳态值。专用回放trace则记录trace/hash/失包时刻并检查生效。delay队列限额须按实际外层PPS×延迟加抖动余量预设，区别目标loss与队列overflow，不能靠无限socket buffer隐藏服务瓶颈。

记录CPU型号/ISA、core、cgroup quota/cpuset、GOMAXPROCS、host busy、PSI/steal可得范围、产品/助手CPU、峰值RSS/GC、源注入/发送滞后、AF_PACKET/Npcap/socket/内部queue drop。记录raw缓冲实际有效值而非仅配置值；系统/驱动不支持时如实写。有容量上限时单列 `CAPACITY_LIMITED`、已得到的功能证据和待复验，不能把良好业务/坏host证据都抹掉。

CPU按各端真实TX/RX首次unique字节明确归一化，双向分母与单向分母不能混用；损失大导致CPU/GiB飙升也要列CPU-s、输入/交付、wire/PPS。不同CPU/诊断ON-OFF不直接宣布优化；CPU相同也需配额/竞争/输入规模可比。每run只一测量，不为对比违反单条规定；普通诊断OFF性能独立一run，低频有界诊断ON曲线定位不当普通CPU资格。

## 7. 修复路线：证据决定小改，禁止负向优化

1. 先生成理论曲线/允许带和当前540s对照，解析last-qualified历史而不只last HOLD；报告新鲜可信source接受比例、oldest/newest source时间、k/r/边缘source/parity、M/R、Wilson/slow、输入成熟和实际提交时间。核查“1%却长期20”以及“240s clean仍20”。
2. 若问题是256-PN窗口不对齐真实组/采样效率低、旧历史按样本数滞留过长、因不必要修复扩大M+R上界，则分别最小修复。允许增加有界统计内存/时间戳/组证据，优先复用已有发送账本；不追求无法辨认的精确首发物理p，不拆same-wire语义，不让统计锁等待发送。
3. **最新用户允许以跨工况证据修订统计/控制软参数。** 30s/60s/512/32窗口/置信裕量是起点而不是永恒硬门；若事实证明它们组合产生无必要高档或几分钟迟钝，预声明改动/原因/风险后可调整一项。认证、完整性、generation、fresh、MTU、资源有界等硬门不动。没有可信输入不能强降或伪造零loss，也不能永远停高档后宣称自动有效。
4. 针对不可达目标/partial饱和，判断更高档是否真有额外保护，不把 `requiredProfile` 找不到档简单当必须max20；burst margin有触发/上限/退出，不遗留永久保护。只在现有算法上做必要的小改，不引入后台探针、持久线路学习、全局状态或大量每包概率计算。
5. 每个修复Actions core/race和该反例单条实际波形→合理保护/开销/p99→固定/off/Game保护，再继续下一修复。同一理论有效规则跨多个场景，不对seed或上/下行加专用例外；不得仅为了曲线好看关外层repair、丢fresh、增大socket/可靠窗口。
6. 已足够证明数据正确、曲线合理、开销受控且余下限制明确时收口N2，继续N3–N6。不要无限逼近理论或用额外安全性堆复杂度。

## 8. 留痕与完成条件

每轮原子提交devlog+唯一STATUS+evidence。每条run记录 SOURCE/helper/config/model版本/seed或UNSEEDED/run/job/artifact/hash，业务首次交付/p99/missing/最长连续空洞、双向实际档位曲线及允许带、冗余/wire/PPS、CPU/内存/队列和限制。小型CSV/JSON/SVG可存，原始大pcap按owned范围清理保留摘要/hash，密钥/凭据/正文不上GitHub。

结论建议拆为 `SCOPED_FUNCTIONAL_PASS`、`SCOPED_ADAPTIVE_EFFECTIVENESS_PASS`、`MEASURED_NOT_QUALIFIED`、`QUALITY_LIMITED`、`CAPACITY_LIMITED`、`INVALID`、`FAIL`、`NOT_RUN`；不要把所有新标签塞入既有只允许固定enum的product_state，那里仍IN_PROGRESS直到既有资格满足。表现接近模型可结案，偶发合理偏差需留原数值；持续高档、持续多秒tail和系统中断仍是实质问题。

N2完成至少要有：双向真实升降/稳态开销、普通和激进、不对称、稀疏/反馈/代次/idle、mixed/MTU和固定/off/Game保护；模型/功能可合批Actions，性能仍每run单条。Actions无法做真实Windows Wintun/ARM硬件功能则NOT_RUN，不假称Linux/mock通过。N0–N6全部功能和同源三目标P6证据齐备后才ACTIONS_READY_FOR_PHYSICAL，physical仍NOT_RUN，由原聊天最终统一复验；不移动qualification ref/合并正式主线/自动部署。

## 9. 参考边界

- 概率式由系统式MDS恢复条件直接推导，需在Actions独立参考中校核p=0/1、R=0、k=1与小组全枚举/二项和，不复用产品公式作为唯一oracle。
- [RFC 9002 §6.1](https://www.rfc-editor.org/rfc/rfc9002.html#section-6.1)：丢包检测要容忍重排，错误判丢会导致不必要重传；不照搬QUIC有序/拥塞语义。
- [tc-netem](https://man7.org/linux/man-pages/man8/tc-netem.8.html)：独立/相关loss、GE、delay/reorder/seed能力按runner实际支持验证；不把模型配置当真实生效。

这些来源用于解释边界，不授权替换本项目运输架构。
