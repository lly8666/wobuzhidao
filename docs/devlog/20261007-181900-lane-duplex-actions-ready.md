# 20261007-181900 Lane方向锁候选Actions验收完成并移交物理复验

## 本轮目标和阶段

接手固定SOURCE `3a594a34191159bd7224f35ba9117cdf6f239c69` / `qualification/lane-duplex-20261007`，完成方向锁候选从core/race到Normal、Game4和同源码P6的Actions验收。没有修改产品源码，没有移动qualification ref，没有部署物理机。所有编译、测试、race、网络与性能均在GitHub Actions；本机/容器只读分析已生成artifact。

## 修改与原因

产品修改仍只有候选既有的Lane方向锁：TX `mu` 管encoder/sealer/PN/outbound/padding，RX `rxMu` 管decoder/recovery/reassembly/Expire/InboundTransition；Stats/Close固定TX→RX，Health留在TX。wire、keys、FEC档位/期限、repair、shadow4096、MTU、generation和首次交付规则均未变。本轮只新增验收留痕；为补Game大artifact的10ms检查，另用隔离的controller分支做workflow dispatch和只读artifact scan，不修改产品ref。

## Actions证据

Foundation 37602221629的Windows/Linux unit/build、Linux race/fuzz、arm64 compile及privileged jobs实际PASS；历史extension job的skipped没有冒充PASS。fullstack 37602535872、default-network 37602539898、splitroute 37602544149、config 37602548438、linux-server 37602552100均PASS。

Normal r12 profile 37602556519、r12 lossless 37602559653、r12 5305 37602563270、正式20:20 lossless 37602567530、20:20 5205 37602571402均为独立单样本run。正式20:20 paired stress probe p99 605.904→613.274ms（+7.370ms），60/60；5205 c2s stress实际业务loss约0.481%、s2c0，按档位实际恢复能力记录而非强求零loss。raw/socket/link drops为0，RSS未显示扩大buffer换收益。

同seed r12诊断对比支持方向隔离的局部收益：Lane RX lock wait累计约7221.6ms→39.4ms、max2.584→1.051ms；route queue max18.302→6.533ms、over10ms 1360→0、peak358→51；父诊断业务10ms连续零交付约1310/1290ms，候选同seed为0。mutex profile中Lane.outbound累计延迟约5427.9ms→36.8ms。该结果没有复现历史约284ms停顿，因此根因仍OPEN。普通r12/5305候选stress p99约2168.137ms，父同seed约2177.749ms，说明另一类late probe仍存在，不能用profile好样本关闭。

Game4正式20:20每方向逻辑3Mbps：lossless/5205 seed1479为37605009121/37605011893，lossless/5305 seed1484为37605015130/37605017713。四run均SUCCESS且五分类PASS；5205/5305两方向stress均约3Mbps、app loss0、probes60/60、mismatch/stale/source-discard0、socket/link drop0。paired stress p99分别+1.808ms、+4.852ms。Actions只读scan 37605933377直接核验原始10ms业务桶：两个lossless及5305零空洞，5205只有零散单个10ms桶，最长10ms，无长HOL。

P6 run 37606109601三目标和aggregate均PASS，aggregate为HOSTED_PACKAGE_ONLY且physical/release_qualified仍NOT_RUN。linux/amd64 artifact11475186403 sha256 accd0ed8d38416405d16fab5b493f6c88f6c0e11dd9864ca9e08f891d0ffb5e7；linux/arm64 11474687727 sha256 796062547d99777864ab53117dbef695c94eab157a04b95822dafa073923c976；windows/amd64 11474354633 sha256 a4553ebf6412b354ec49f1792981a58c8d1fd184243ca58745685e8e917d8b90。三bundle的SOURCE、target、manifest hash和manifest内每个文件size/sha256再次只读复算，0 mismatch。arm64为hosted cross-build，不能写成ARM物理PASS。

完整结构化证据：`docs/evidence/lane-duplex-3a594a3-qualification-20261007.json`。

## 问题、排查与风险

方向锁候选保留，因为同seed诊断确认争用/排队显著降低，正式Normal/Game的完整性、paired p99、10ms连续性、drop、RSS与带宽门没有发现退化；但不宣称CPU收益，hosted runner CPU-time跨样本波动明显。r12普通5305长probe tail和历史284ms停顿根因都继续OPEN。备用FEC zero-tail计算优化没有开发，也没有重开FEC选型。

旧11项配对RTT FAIL、native S01/S16/M03、full70/final18/1800s及STATUS中的其他NOT_RUN全部保留。ACTIONS_READY_FOR_PHYSICAL不等于PHYSICAL_PASS或RELEASE_QUALIFIED。

## 下一项原子任务

原聊天读取本日志/STATUS/evidence后，使用同一SOURCE的上述P6包做Windows→Linux ARM物理复验，重点真实业务首次交付/p99/弱网、rotation A→A+B→B、payload idle与keepalive分离、DNS互备、split routing、IPv6和>MTU包，并核验真实驱动/NIC/TUN与ARM运行。必要抓包仅有界观测，结束清理owned原始大文件。除非物理证据触发新的开发交接，本聊天不再修改产品源码。
