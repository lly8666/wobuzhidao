# 20261007-173937 客户端双向锁候选收尾与全新聊天交接

## 本轮目标和阶段

用户要求简单收尾，将开发及Actions验收交给全新聊天，原聊天收到验好的同源包后负责物理机复验。开始产品诊断候选6e7638abf7b8bea1d6877c239bcaf3cea465ecae；本轮只提交小范围Lane方向锁候选，尚未验证，不部署。

## 修改与原因

- client_locks准备、root完成静态审查与格式化：Lane现有mu只管TX，新增rxMu管RX/Expire/InboundTransition；Stats/Close统一TX→RX双锁；closed唯一写持双锁，读持任意方向锁；Health保持TX以保证PN唯一。
- 新lane_direction_lock_test.go有三项实质并发测试：阻塞TX时接收/移交/过期仍立即完成（off/12/20）；阻塞RX时TX/flush/health PN正常；TX/RX/Expire/Stats/Close并发边界。测试未本地运行，必须Actions验证。未改变wire、keys、FEC参数/期限、repair、shadow4096、owner/generation/latefirstarrival。
- 补充章程明确用户优先级：真实业务首次到达效率最高、p99/noHOL/低开销、TCP/TLS外观尽力，安全强化最后；不取消原有完整性/认证/隔离等硬门。

## 复用来源

仅现有新分支Lane状态按独立方向保护；未提取old。三路审计分别覆盖raw写、client锁、FEC/owner；方向锁实际实现与字段/锁序审查分离。client_locks在收尾时会话额度耗尽，root已接管完成格式化、阅读全部新增测试和静态审查，未将其测试准备冒充已运行。

## Actions证据

6e诊断补丁16/16 workflow SUCCESS，包含12基础/生命周期/网络/GUI等工作流，及profile+三条独立普通样本；元数据docs/evidence/client-stage-6e7638a-actions-20261007.json。fullstack与core/race实际job要由接手者继续复核，不把workflow数当样本数。

6e专项profile37580826640实际pipeline/raw/lane/transport/feedback开关全true，docs/evidence/client-stage-6e7638a-20261007.json。client handler最大3.206ms、state lockwait0.990ms、lane inboundlockwait2.584ms、decode1.769ms、raw lockwait1.889ms/syscall1.888ms；此次只见route age最大18.302ms，未复现旧284.219ms停顿。不能把此前累计Lane争用5.2s或这次好样本写成根因关闭。

普通父样本：r12 lossless37581075051(seed1500)、r12 5305 37581075128(seed1508)、r20 5205 37581074824(seed1521)，每Action一条。均workflow SUCCESS，原始吞吐/p95/p99/coverage/resource尚未逐项收齐，不冒充整体性能PASS。旧d6 profile37579975921及旧pause37574340886均保留。

方向锁本轮SOURCE是本日志所在代码提交，冻结ref随后写入新聊天提示词。候选core/race/ordinary/performance全NOT_RUN，预期基于共享锁争用改善双向隔离，不承诺消除284ms。

## 问题、排查与风险

FEC实现恢复策略冻结；允许必要有证据的等价计算优化但一次只做一项、5205过关再下一项，不重开档位选型。跳过source零尾部乘法是备用建议，尚未实现。不能扩大buffer/FEC/4096、更改无损/吞吐/p99门或用系统drop抵扣线路loss。11旧配对RTT FAIL仍OPEN，原生S01/S16/M03/full70/final18/1800s/19NOT_RUN按STATUS保留。

## 下一项原子任务（新聊天负责）

1. 先读AGENTS→章程→STATUS→ROADMAP/PLAN→WIRE/MODULE_MAP→ACCEPTANCE及本日志/PARAMETERS，核验分支/remote/工作树。主项目D:/codex/wbd，实际repoD:/codex/wbd/audit-next-p2，detached HEAD可用，禁止reset覆盖或读old指令。
2. 收本候选自动core/build/race/datapath并发测试、生命周期/GUI/网络所需门；失败最小修复。父6e本轮结果不能继承给新SOURCE。
3. 单独profile r12/5305 seed1508验证锁等待是否改善，普通off r12 lossless/5305+正式r20 Normal5205，同源配对lossless RTT baseline；然后Game4每方向3M/FEC20:20的5205，必要5305。每性能Action一条，不同run可并行，不做同run A/B/matrix；所有编译测试Actions，本地仅编辑格式化和只读分析。
4. 重点比较真实输入和goodput、p95/p99覆盖/late/missing、10ms交付空洞、post5、wire/PPS/CPU/RSS、raw/socketdrop/queue。profile是诊断，不能替代默认off资格；只在证据支持且效率/尾延迟无退化时保留候选。原284ms未复现可据实PARTIAL，不虚构根因。
5. 若方向锁无效/退化，用证据最小调整或回退这个优化；必要时再做已知zero-tail等价计算优化，每项分别过5205。不盲拆并行线程、不破坏所有权，不追求零loss/HOL。
6. 每轮写详细devlog/更新唯一STATUS/PLAN/MODULE_MAP/ACCEPTANCE，精确SOURCE/HARNESS/run/artifact/hash/失败都留痕，新增参数同步catalog。最后同源码P6三目标打包，清楚写ACTIONS_READY_FOR_PHYSICAL或未完成，给原聊天固定SHA、包/manifest/hash、测试矩阵和实机重点。不自行部署物理机。

## 原聊天负责的后续物理复验

等新聊天完成并核验同源Actions证据和包后，原聊天再按现有PHYSICAL_5MIN_ACCEPTANCE、STATUS实机未完项操作，Windows→LinuxARM五分钟Normal/Game/低档FEC、rotation/idle+keepalive/DNS互备/分流/IPv6/大于MTU包，重点p99/真实业务持续交付。既有用户授权机器沿用会话/受保护本机材料，口令不写仓库/提示词。先记录线路质量与host压力，不能把超FEC能力线路loss当程序bug或把本机drop藏在线路里。无需持久大pcap，必要有界头部观测，测试后清理owned原始大抓包，保留摘要/hash与失败。只移交开发权，原聊天不再同时编辑共享源码。
