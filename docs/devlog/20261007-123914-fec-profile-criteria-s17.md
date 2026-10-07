# 20261007-123914 按FEC能力定义检测标准及S17

## 本轮目标和阶段

用户新增要求按不同FEC与实际丢包能力制定合理检测门，优先性能、延迟、no-HOL。产品sourced6cb6cee4c241aac8dd2f542a876edc57bf3d7db，文档HEAD4b6850a。S17独立300s运行中，不提前标PASS。

## 修改与原因

在同一WEAKNET_QUALIFICATION第10.6节写实际partial min(k,R)、20:20分大小组、有限块理想源loss解析表、codec硬正确性/性能质量/恢复效率三栏。解析表不是测试结果或生产保证，不追溯降低正式20:20门/历史FAIL。其他档位workflow输入与traceoracle尚未实现，STATUS明确NOT_IMPLEMENTED/NOT_RUN。本轮无产品代码/参数变化。

外部物理controller仅添加S17配置4lane/3M/FEC20/60s；原14助手及DNSguardhelper字节冻结，默认stageoff/profileoff/workeron。此运行不与另一个物理负载并行，server配置/owned网络在finally恢复，原始有界抓包审计后删除。

## 复用来源

无old复用。读取fec.go/fastblock_encoder.go/block.go/size_class_encoder.go/linkdata fec_path；RS erasure参考RFC5510，但wire是项目私有既有格式。数学解析在本机只读计算，不属于编译/unit/性能验收。

## Actions证据

当前source14scoped/24RTT/P6/lifecycle36+aggregate PASS见上一日志/evidence，lower-profile新弱网专项NOT_RUN。每性能Action仍一条。S17为用户授权的原生阶段例外，控制器receipt docs/evidence/native-s17-controller-20261007.json；原helpers不变。

## 问题、排查与风险

完整20:20 p30理想源loss期望0.130108%，partial k=r=1时9%；不能把平均p、源码profile名或source残余直接当业务多片/Game硬门。相关损伤不能乘q^4。有限恢复槽/3s/parity时效、本机drop与真实WAN分开报告；高损下不强制恢复无限状态，已到systematic/后续完整包仍必须立即交付。p99包括返回覆盖解释；线上字节与CPU成本保留。

## 下一项原子任务

Collect S17 native300s Game4/3M/20:20/rotate60s seed1485, analyze per-ID generations/latest-ACK/nozeroactive/probes and ownedcleanup; preserve failures. Then implement bounded test-only profile-aware weaknet input/identity and oracle underWEAKNET_QUALIFICATION10.6; no productFEC/recovery changes without defect evidence; every performanceActionone sample. ARMraw208KiB/read-service/M03/full70/18/1800s remain open.

## S17完成回执与用户新增批量筛查任务

SOURCE d6cb6cee4c241aac8dd2f542a876edc57bf3d7db，300.000050s，Game4双向goodput2.9999877/2.9999903Mbps，业务loss0，探针2980/2980，p95/p99=118.8424/135.4208ms；三个阶段最高p99169.3538ms。原门BUSINESS_PASS_TRANSPORT_PRESSURE，Windowsdriver/interface/useroverflow0，server raw686，4个live snapshot写错误均保留。CPUclient343.765625/server170.43秒，helpers11.578125/17.694597秒；跨时间不宣称优化固定收益。默认DNS60/60，选定NIC53plaintext0，owned规则/NRPT/network journal清理0，配置恢复/原14helpers不变，pcap审计即删。

四logicalID分别generation1→5→9、2→6、3→7、4→8，logicalpeak4/physicalpeak5/retiringpeak1，lease稳定，未观察active0。9个transportref实际worker启用/timingoff/error0；queued1151808/coalesced542561（47.105%）/attempts608109/sent608108，最后旧ref的1次inflight差不等于丢ACK或线程泄漏，仍非最终join证明。逐lane至少换代一次；1秒快照不证明亚秒wire/精确FIN顺序，附近探针窗口可能重叠不能重复计数。证据docs/evidence/native-ack-d6cb6ce-s17-seed1485-20261007.json及receipt压缩。

当前53普通300s/24唯一工况/19NOT_RUN另2诊断，不等于53PASS，S01/S16/M03历史失败保留。用户要求其他FEC快速批量筛查；下一工作转为测试框架显式profile/identity与独立Actions，不改产品FEC参数/恢复强度，每run只一条。按10.6理论和性能/noHOL优先收口，差异不能简单用完整块q判程序故障。
