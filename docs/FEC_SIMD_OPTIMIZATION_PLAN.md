# FEC跨平台SIMD优化开发方案

状态：PLANNED_NOT_RUN。实时状态只看STATUS.json；本文是实现与验收契约，不是已实现声明。

## 1. 本轮决定、来源与优先级

工作分支：`next/fec-simd-20261010`。从`543ac2cd2920e9f0fb59fdb38ee6aa3d9a65e56f`创建，保留最新串行测试夹具和36工况证据。旧版产品比较基线A固定为`a2db258b436a41fdee98c6c53abec9bab6ce600f`；父分支相对A的internal/cmd差异只有测试文件，不是新的产品优化。新候选B必须另外冻结精确SOURCE。文档HEAD、helper HEAD和产品SOURCE分别登记，不能互相冒充。

2026-10-10用户补充：**一切以性能第一为主旨，自己的FEC可以大改**。具体目标是持续真实业务首次到达效率、低p99/无跨业务HOL、低CPU/带宽、突发稳定；允许适当增加有界内存。内部结构、生成/求逆实现、缓存、循环布局、类型边界都可以重构；旧实现不是永久限制。基本完整性、认证/地址隔离、固定密文重传、generation、MTU和有界状态仍不能退化。额外链路安全不增加复杂度。

选定`github.com/klauspost/reedsolomon v1.12.6`，MIT，tag对象`4916c9cd17081aa4f43f36639502e86f5787b40e`。该版本go.mod为Go1.23，匹配当前Actions Go1.23.12；较新的v1.13.3/v1.14.2要求Go1.24，本轮不混入工具链升级。版本通过go.mod/go.sum固定，不用latest。x/sys按Go模块MVS处理，不能擅自降级现有版本。

选择理由：Go直接集成、x86与ARM64加速和非加速回退均有可核源码；无需为小包编码增加C/Rust FFI、cgo部署和额外线程。Intel ISA-L是优秀C实现，但本项目跨平台接入边界更重；reed-solomon-simd使用不同域/算法布局，不能作为当前GF256逐字节的直接替换。这是本项目的适配判断，不是它们速度更差的结论。

一手资料（已读源码，不使用营销跑分代替WBD测量）：
- [选定项目](https://github.com/klauspost/reedsolomon/tree/v1.12.6)、[Go版本与依赖](https://github.com/klauspost/reedsolomon/blob/v1.12.6/go.mod)、[MIT许可](https://github.com/klauspost/reedsolomon/blob/v1.12.6/LICENSE)。
- [LowLevel接口](https://github.com/klauspost/reedsolomon/blob/v1.12.6/mulslice.go)、[amd64乘法路径](https://github.com/klauspost/reedsolomon/blob/v1.12.6/galois_amd64.go)、[ARM64路径](https://github.com/klauspost/reedsolomon/blob/v1.12.6/galois_arm64.go)、[标量回退](https://github.com/klauspost/reedsolomon/blob/v1.12.6/galois_noasm.go)。
- [整块编码、恢复和融合循环](https://github.com/klauspost/reedsolomon/blob/v1.12.6/reedsolomon.go)、[自定义矩阵与线程/缓存选项](https://github.com/klauspost/reedsolomon/blob/v1.12.6/options.go)。
- [ISA-L](https://github.com/intel/isa-l)、[Rust reed-solomon-simd](https://github.com/AndersTrier/reed-solomon-simd)。

## 2. 保留的是有效行为，不是旧内部架构

|继续保持|可以重写|
|---|---|
|原始systematic独立、立即发送/首次交付；不等填满20包、repair或其它lane|GF运算后端、整块编码/求逆实现、行列循环和共享不可变矩阵|
|off/20:4/8/10/12/16/20；当前partial有效parity=min(N,配置P)|active编码接口、用能力契约替代具体类型判断、描述符与缓存布局|
|当前32ms partial调度/size-class政策、3s绝对恢复期限及bounded compact retirement|有界预计算、工作区复用、只恢复缺失systematic的实现方式|
|ownership、迟到systematic仍首次交付、去重/完整性、MTU、大包兼容|codec内部对象划分；在收益清楚时移除旧热路径实现|
|fresh不受4096 shadow门控、同Seq同wire、Game首次竞速/generation|不能以本轮为由改变外层repair政策、堆socket buffer或凑包等待|

本轮默认保留FEC v1 header/索引/生成矩阵/输出字节，原因是隔离CPU优化变量和保留互通，**不是用户禁止FEC重构**。不要无收益地同时改wire、档位比例、调度和工具链。若以后确有改变wire的性能理由，单列版本化方案/迁移和精确证据，不隐式破坏互通。

现状需读：`fastcodec.go`为64KiB查表字节循环；`EncodeActive`已跳过无效source/多余parity；Reconstruct只恢复缺失数据、不重算无用parity。`fastblock_encoder.go`以具体`*FastReedSolomon20x20`判断无需清理inactive source；换类型会悄悄恢复20-N清零开销，必须同步改成明确能力契约并有负例测试。`size_class_encoder.go`/`linkdata/fec_path.go`管组、期限、off和档位，不用库的大文件Split/Join替代它们。

## 3. 确定实现路线：逐片SIMD，再整块融合

### S1：完整编码与恢复共用的SIMD乘加

先在生产xorMul对应整段操作接入零值`reedsolomon.LowLevel.GalMulSliceXor`，两条编码/恢复路径同时受益。仍有原WBD标量函数作为reference/回退，参考`ReedSolomon20x20`不改。低层语义是out XOR (coef乘in)，不能错误替换成覆盖out。

CPU能力在初始化时判定并只读；短span初始以32B为分界，按当前32/16B内核结构选择，微基准若证明不合适再在同一优化内调整。x86有AVX2/SSSE3或ARM64具有所需向量能力才选SIMD，否则走现有64KiB标量。尾部、未对齐buffer由内核正确处理，不为对齐增加线上padding或每片copy。coef0/1保留正确语义。平台文件/build tags正确处理`noasm/appengine/gccgo/nopshufb`；加`wbd_fec_scalar`构建标签作回退和归因，不增加用户GUI/运行配置。

两个源码审计陷阱：
1. v1.12.6的LowLevel.WithOptions未把局部options保存回对象，本阶段不要调用它或假装已关闭某条ISA。使用不可变零值，不改全局cpuid。测试强制标量由WBD自己的dispatcher/build tag完成。
2. LowLevel单span实际是AVX2/SSSE3与ARM64 NEON，**不能宣传调用这个API就获得AVX512/GFNI**。库的普通非SIMD回退可能按系数懒建较大的16-bit表；无SIMD/特殊build必须回原WBD标量，不让“兼容回退”无意增加数十MiB查表。

热路径不新建goroutine/timer，不按字节做CPU检测；不新增input/output重叠或借用寿命。普通测量不开per-record timing/pprof。先通过S1单测/race和同runner小包微基准，留下原子日志。

### S2：开发多输出整块编码，减少重复读取和调用

性能优先授权下，不把S1当终点。针对密集20包完整块，适配库的整块编码引擎，利用一次输入处理多个parity的生成内核。完整块以`New(20,P)`配置精确P行的`WithCustomMatrix`，用WBD现有generator第20..20+P行；严格只传P行，不能传20行给P<20实例。库默认矩阵不是当前WBD矩阵，直接默认New会改变parity。

实例初始化时共享不可变矩阵、`WithMaxGoroutines(1)`、初始`WithInversionCache(false)`。小片编码不拆并行任务。库的整块路径可按CPU/尺寸选择不同生成内核，包括符合条件的GFNI/AVX512；能力可用不等于实际执行，只有定向profile/路径证据可以宣称用到该ISA。每块动态plan/矩阵生成存在额外alloc，必须量化，不能因“更高级指令”就默认更省总CPU。

Partial N<20继续用S1只读N/只写min(N,P)的active路径作为首版，保住已有inactive优化。允许之后在同一codec适配层把partial转为N列的shortened自定义矩阵和有界预生成实例，但必须先证明与20-N authoritative zero数学等价及逐字节一致；不使用库默认N矩阵。不要热路径按N反复New。N=1的有效parity权重也不默认等于简单XOR，要按WBD系数算。

恢复先使用S1保留只补缺失数据；只有恢复热点仍显著时，接入同矩阵`ReconstructData/ReconstructSome`，复用缺片底层存储，不让库重新分配大shard或补已退役parity。库根据nil/零len判missing，WBD根据present；适配时保留原buffer/capacity，不能把仍在使用的slice丢掉。已全到齐直接返回，只有方程足够才恢复，稀疏known-zero语义必须等价。原始systematic快路不经过重建。

暂不启用无限求逆树缓存。若求逆确成热点，可用lane-local固定64项缓存：完整selected-row bitmap/矩阵形状为key，只存有界小矩阵、固定替换、不锁全局热路径。写清每lane/10 incarnation/进程最坏内存，分别测高命中与随机失配；若无实际收益不保留缓存，不用缓存扩大block生命周期。

S2不要求保留慢旧代码形态；允许把整块/active/scalar重构成简洁明确后端。最终自动选择按真实WBD片长/完整与partial的已测结果确定，不加用户手调ISA开关。若融合路径CPU/p99差于S1，就不把它设默认，记录REJECTED_NO_GAIN及原因，S1仍可交付。不能为了“大改”硬留负收益架构。

## 4. 动手顺序与退出条件

|步骤|本次agent做什么|继续条件|
|---|---|---|
|S0|核分支/HEAD、读本方案/STATUS/原始热点；冻结A、helper，登记依赖许可/版本；补正确性oracle|不是恢复旧E1下一任务；无本地编译；不搬old架构|
|S1|跨平台SIMD span及原标量回退、encoding/recovery共用，完成unit/race|字节与oracle完全一致，尾部/别名/回退无回归|
|S2|整块融合编码适配；必要的能力接口/有界工作区；恢复更换以热点为门|inactive清零/小partial/系统包立即到达不退化；分配/CPU收益证据明确|
|S3|新独立paired fixture/workflow，真实业务pilot，严格receipt|两个产品SOURCE/构建hash、实际配置、netem/发送/CPU可核验|
|S4|新旧同job ABBA正式screen、跨runner重复、ARM native|下文质量/CPU门同时满足；仅受限场景通过不能写全局PASS|
|S5|关键300s确认、配置/生命周期/打包；文档收口给原聊天物理复验|精确候选SOURCE、保留失败、P6 hash齐；physical保持NOT_RUN|

每步原子commit同时更新STATUS并新增详细devlog，带开发原因、源/依赖、改动、真实Actions/run/artifact、FAIL/NOT_RUN/限制、下一任务。产品SOURCE变化后不得继承之前PASS。

## 5. 正确性与跨平台执行门

全部Go开发编译、unit/race/fuzz/benchmark在Actions，本地只编辑/阅读/Git/数据处理。

- GF所有256x256组合与旧乘法逐项对照；coef0/1/255；长度1/15/16/17/31/32/33/63/64/65/96/127/128/129/256/512/1000及当前真实source cap；非对齐offset/sentinel边界。测auto和wbd_fec_scalar/noasm。不能用大块存储尺寸覆盖网络小片测试。
- 所有P=4/8/10/12/16/20、N=1..20，混合尺寸与20→1→20复用。active parity字节全等，未使用行不被写，known-zero权威、不读脏inactive slot，S2正常全块与partial相同wire。
- 旧编码→新恢复、新编码→旧恢复；全到齐、1/多片丢失、恰在恢复能力内/外、重复乱序/parity先到/metadata变化、迟到source、到期compact、状态上限与关闭/generation。只恢复缺失数据，parity无用时不做重建。
- 现有active_parity/e2 inactive clear/deadline/profiles/live pressure/size class/cross_mtu_sparse_recovery以及core/race全过；系统包与独立小包不等更早丢包。保留ownership负例。
- Linux amd64和Windows amd64实际unit/build；Linux ARM64要用native ARM runner执行SIMD测试。跨编译仅证明可构建，不能标NEON执行/ARM性能PASS；不拿QEMU跑分当实机。ARM32及其它受支持平台编译/标量回退正确；runner/权限不可用记NOT_RUN/UNSUPPORTED并突出限制。
- 功能门可多job；性能测量仍必须单job无并行负载。后端CPU检测不得造成illegal instruction，release继续携SOURCE/target/manifest/hash和依赖许可。

## 6. 复用可靠夹具，但不污染旧实验

使用本分支包含的543ac2cd helper快照，功能说明见REALPATH_TEST_FIXTURE_GUIDE。本轮新建`tools/fec_simd_ab.py`、`.github/fec-simd-ab.json`与`.github/workflows/next-fec-simd-ab.yml`，复用正式netns拓扑/业务/分析/resource与owned清理。现有`tools/fec_policy_batch.py`写死产品A和旧branch、只做off/on，**不能直接冒充两SOURCE新旧比较**。旧batch配置/入口原样保留。

`prepare_large_mtu_harness.py --fec-experiment`已有120s及15/300ms入口；完整业务、HTTP计划、采样、manifest、CPU分母和drain都要一致。5205分阶段若现adapter只支持固定loss，显式接stage helper 30/60/30并审计实际qdisc；不只改标签。复用五netns真实socket→TPROXY→正式client/raw→netem→正式server/shared TUN→目标socket，不以直接调用codec代替端到端。

原batch B的300ms TCP-off收尾未通过，剩7段NOT_RUN；旧Game4 socket压力与80秒S2C问题未关。新mixed测量若TCP完整性/实际注入不达标，保留FAIL并定位夹具/真实背压，不静默去掉TCP、放宽收尾或只统计收到的字节。

新增workflow策略只在`next/fec-simd-20261010`、精确配置路径push、单job/无matrix允许serial ABBA；同步`check_performance_workflow_policy.py`与其单测，不能全局放宽。新feature分支workflow_dispatch不保证可发现，沿用现有精确config push触发；不修改default/main线来注册workflow。设置批次claim/互斥，禁止两个测量进程叠加。功能CI并行允许，但不要同测量host额外启动分析/编译/pprof负载。

## 7. 同一个Actions的公平新旧对照

用户明确授权本轮**一个Actions单job顺序跑新旧版**，是针对FEC SIMD的窄例外；旧“一run一条”不能阻断此任务，也不能因此放开其它性能跑法。

A=a2db固定旧产品，B=本轮精确候选，helper=冻结测试提交。两版同job用Go1.23.12、相同编译flags/GOAMD64基础设置和业务环境，一次各build后核buildinfo与二进制SHA256。每context固定同seed和逻辑输入，顺序**A→B→B→A**，每leg新产品进程/netns/租约/随机状态，清理owned资源并等待队列排空。建连不计steady CPU，启动成本另外报告。CPU与流量按120s真实区间计，+3s drain独列；不能把排空时长算吞吐或给某版本多等。

### 首批Q：基础与Game保护（120s screen）

|context|业务/模式|FEC|单向delay/注入|每方向逻辑速率|
|---|---|---|---|---|
|Q1|mixed / Normal1|20:20|300ms / 0%|10Mbps|
|Q2|mixed / Normal1|20:20|300ms / 5205=5→20→5%|10Mbps|
|Q3|mixed / Game2|20:20|300ms / 5205|3Mbps总逻辑|

每context ABBA=4legs，12legs约24分钟净负载，计构建/隔离预计30–50分钟，一个job timeout90分钟。先Q1 A/B pilot通过再整批；预检失败未开始的leg记NOT_RUN，timeout保留部分产物，不反复启动几十分钟盲跑。Game4另作严格功能/诊断保护，因旧容量FAIL不能假定天然通过。

### 第二批L：轻档与负对照

L1 Normal1 mixed、20:4、15ms单向/1%；L2同条件20:10；L3 Normal1 mixed、off、15ms/0%。每方向10Mbps，均ABBA120s+3s，共12legs。off是负对照：FEC内核不应参与，若同样大幅变快，先查host漂移/测量或其它代码变动；不把它算FEC收益。其它P通过完整正确性门后可按热点补paired case，不宣布没测过的档位CPU收益。

Native ARM批次至少Q1/Q2 ABBA，8legs；same runner两源码，不能拿x86旧结果作ARM新baseline。先核native ISA、TUN/AF_PACKET/netns/netem权限；不能运行则明确ARM_PERF_NOT_RUN。Windows hosted验证核心与构建，Npcap实机性能留给物理，不宣称托管host已经测了驱动。

### 微基准与最终确认

微基准只解释kernel收益：编码P全档、N=1/5/10/20、真实短/长片；恢复缺1/3/能力边界；ns/op、B/op、allocs/op，分scalar/auto/融合路径，同host顺序测，多次有界重复。禁止只展示1MiB储存块GB/s。profile诊断单列，不能当普通off CPU。

Q/L分别在至少3台独立runner重复，**每台内部仍ABBA**；按CPU型号、ISA、核心/CPU quota/cgroup、PSI/steal分层。若收益低于同台波动，写INCONCLUSIVE而不挑好机器。最终有收益候选对Q1/Q2/必要Game场景做300s同job配对确认，原300s/长测/功能门不被120s screen替代。

## 8. 真实成本与验收方法

每leg记录两端与合计CPU-s/平均核、峰值RSS/HWM、业务submitted/delivered/on-time字节与每尺寸missing/late、UDP首次到达、TCP长短流/hash/HTTP证书和正文核验、probe应发/应返/返/不返、p50/p95/p99、最长10ms零交付段、source/parity/repair与外层data/ACK PPS/字节、AF_PACKET/UDP/TUN/socket drop、FEC heavy/retire/内存状态。每lane configured/actual证据不足写UNKNOWN，不把副本当真实业务。

主要CPU指标：`cost=(clientCPU-s+serverCPU-s)/verified_delivered_GiB`，`gain=1-costB/costA`，同时提供submitted_GiB口径和真实交付/按时效率。注入不足/丢得更多不能靠少做工作冒充优化。固定输入负载与质量后比较双方CPU、RSS、延迟，不用host总busy替代产品CPU。

ISA capability与kernel实际执行分开。启动可输出一次低成本backend/capability说明；定向profile证实SIMD符号后普通测量关闭诊断。没有可靠GC/alloc统计写NOT_COLLECTED，不硬填0。库高级指令若有初始化或降频/调度成本也算真实收益，不能只减算术时间。

门槛：完整性/隔离/同密文/generation/MTU与跨业务无HOL硬门全过；受控无损不得新增缺包/资源drop；弱网按真实线损与FEC理论能力判恢复，不强求超过能力仍零loss；同时不得相比A系统性降低首次到达/按时交付效率或新增长空洞。p99包含缺失与超时比例，不能只看幸存probe。重复配对出现p99增量>=10ms或>=5%作为需复核警报，不能偷偷将它放宽为自动PASS；沿用原场景正式硬门。

CPU目标是**可重复的实际产品降低**，不预先承诺倍数。仅microbench快/整机更忙更闲不算完成；融合若不如S1则回退该融合路径，保留S1与失败证据。内存增长逐项有界、停流/退役可回收；沿用已有baseline+64MiB/进程审计线，超线须写清必要性和最坏上限，不拿更多排队换吞吐。

最坏host有raw/socket drop、PSI很高或cgroup未知需突出CAPACITY_LIMITED/UNKNOWN；这些样本保留，但不用跨CPU裸CPU-s排版本名次。新旧同台都故障仍须报告产品容量/现存缺陷，不能一概归罪runner。mtu普通包与合法跨MTU大包混合必须持续有流；高loss jumbo的收不齐不等于HOL，也不能挡住普通包。

## 9. 收口与历史保护

本轮交付应有代码、依赖许可、完整正确性/跨平台路径结果、同job新旧CSV+JSON/配对报告、SOURCE/helper/hash、普通性能与p99、内存上限、P6配套包和manifest。GitHub devlog/evidence/STATUS是唯一留痕系统。参数不变则不造GUI选项；新增内部build tag在本文和交接提示词登记，真正新增用户参数必须同步catalog/GUI。

S2舍弃也可以有依据完成优化，但需明确实际交付后端及未获得的能力。历史11项RTT FAIL、Game4压力、TCP收尾、MTU/PMTU与80秒S2C故障不被一次SIMD好样本关闭。80秒问题保持OPEN_DEFERRED，优化验收后回原E7单独定位；physical仍NOT_RUN，新代码不得继承旧物理PASS。
