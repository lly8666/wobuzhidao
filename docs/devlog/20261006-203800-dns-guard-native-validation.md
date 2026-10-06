# 20261006-203800 DNS guard原生验证与质量边界

## 本轮目标和阶段

开始HEAD a280566b3a1642d43afe95f29a70ab4daf4e8759，P7 Windows/Npcap/Wintun→ARM原生。产品保持a280固定，不再改运行代码。本轮收口DNSguard真实provider及开关，独立测量D01与下一D06。

## 修改与原因

仅证据/当前状态。D01 seed1464完整300.000243s双向近10M、DNS60/60；物理出口DNS0、observerdrop0/元数据窗口覆盖完整。客户端未启动时强制interface6UDP+TCP53 validreply；启动后UDPtimeout10060/TCP拒绝10013、均无validreply。真实NetSecurity filter是以太网单NIC、Outbound/Block/Anyprofile、UDP/TCP远端53；没有Wintun规则。正常退出ownedDNS0/process0，旧ownedNRPT/IPv6/journal0。故单NIC普通DNSguard功能PASS；DoH/DoT/其它NIC、TCP truncated fallback未验，不夸大。

## 复用来源

无。十四原生助手b393字节保持不变，新有界DNSprobe来自a280并通过Actions。新D06 coordinator仅JSONdns-hijack=false静态配置，reuse同helpers；新的分析器只接受D06且flagfalse，物理DNS预期可见，DNSon的zero-leak gate不改。

## Actions证据

a28013定向run、四独立strict样本/12配对p95+p99检查PASS，最大p95/p99增量10.180534/10.781512ms，P6三目标hash/manifest一致。见windows-dns-guard-a280566-actions-20261006及receiptgz；b675旧wrapper失败保留。完整70/18/1800s仍只属于024，a280NOT_RUN。

## 问题、排查与风险

整条D01仍FAIL，不能以DNSPASS抵消：上行54missing/.0111318%字节loss，下行0；2979探针中2978收到(1timeout)，幸存p95=199.5742/p99=285.8157ms，中段p99=362.5176ms。serverrawdrop70，Win driver/interface/useroverflow0；产品clientCPU306.515625s/server160.01s，助手21.734375/35.7231s单列。INPUT两端lagp99上界2.1/1.3ms，发送窗有效，无lossless保证的真实WAN不能唯一归因协议/VM。rawdrop与业务loss不按一比一归因。

024 S11同机器/同FEC20基线p99=267.5946ms，候选观测增加18.2211ms，clientCPU增加.578125s，server减少.35s。两段相隔约2.5小时/WAN未知，只能观测，不能声明严格零性能下降或DNSguard导致这18ms。下一D06off更近时间的同配置样本，仍不抹掉timeout/loss。

实际FECsource/paritycount1/1；freshblocked/bypass/abandoned/repairEvicted0、FECpressure0、record/patherrors0；峰值shadow4096没有卡fresh。仍有serverIncompleteExpiry3/MissingSources30，不能凭聚合计数唯一定位54业务丢包。M03已有missing/late不关闭，不重复盲测/延长期限/扩大窗口。证据docs/evidence/native-d01-dns-guard-a280566-seed1464-20261006.json。历史完整样本47，unique22/43，剩余21NOT_RUN，含失败/跨SOURCE，不是PASS数量。

## 下一项原子任务

D06 seed1468现运行单300s，静态dns-hijack=false：实际NRPT0/DNSguard0，系统DNS正常，显式physicalUDP/TCP可达，物理普通DNS是预期而非泄漏；检查existing系统DNS/foreign state保留与退出clean。完成后按本套计划补IP/IPv6/自定义DNS等实效，性能诊断用bounded/defaultoff/不等待业务方法，禁止无证据重构或降低p99/吞吐/完整性门。
