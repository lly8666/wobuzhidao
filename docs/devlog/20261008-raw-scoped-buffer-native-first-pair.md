# raw 单 socket 补偿：两条完整原生五分钟对照

固定产品与助手 SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`，冻结 ref `qualification/raw-scoped-buffer-20261008`。P6 run37713269147 的配套 Windows/ARM 包已核 manifest/size/hash 后部署；未改系统 rmem/wmem 默认、服务权限、Npcap、FEC、4096、MTU 或稳态收发路径。Actions16条 exact-source 资格见原有 actions evidence；本轮物理证据为 `docs/evidence/raw-scoped-buffer-b4ea061-physical-20261008.json`。

原生测试严格单条顺序执行，每条300s、Normal1、FEC20:20、双向10Mbps、profile off、内层MTU9000/外层1400，受控私网目标显式 route-mode=all；不冒充默认中国/LAN分流资格。每次退出恢复自己的路由/NRPT/firewall/测试配置，短窗口raw抓包分析后删除。

| seed | 请求/实际 raw budget | C2S / S2C goodput | 业务缺包 C2S/S2C | probe / p99 | raw socket drop |
| --- | --- | --- | --- | --- | --- |
|1551|0继承 / 212992 B|9.999953 / 9.999953 Mbps|0 / 0|2980/2980，73.2713ms|61|
|1552|524288 / 1048576 B|9.999874 / 9.999953 Mbps|10 / 0，上行0.0010592%字节|2980/2980，74.0073ms|0|

1551原分析器为 BUSINESS_PASS_TRANSPORT_PRESSURE；1552原分析器仍 FAIL/business_loss，不能改写成全链路无损PASS。两条全阶段注入/吞吐门、输入计数、probe coverage、完整性、DNS60/60、所选物理NIC明文53观测和owned退出清理均通过。Npcap driver/interface drop、用户receive overflow为0。live snapshot写入失败分别4/2次单独保留，不用离散快照证明全时段无HOL。

1552启动读回 `forced=true,limited=false,effective_bytes=1048576`，同时逐秒 owned wbd-server socket rb为1048576；全局rmem_default/max仍212992。初始化journal包含历史重启，证据只提取带新force字段且匹配本条请求的日志，测量实际rb从本条socket快照独立验证，不把旧c853的416KiB条目归到新SOURCE。程序只在普通请求受限时使用该socket的现有CAP_NET_ADMIN，0继承不尝试force，权限不足/不支持保留普通实际值。没有新增稳态调用或权限。

同源码附近时段的结果支持补偿可以消除这条样本的raw drop，未见明显尾延迟/CPU代价；但网络时段/seed不同，没有同源受控native RTT基线，不能宣布固有性能改善或所有队列丢包根因已关闭。业务10个残余包在raw drop0时仍存在，不能把raw drop与业务丢失一一对应，也不能无证据归因FEC或WAN。

下一顺序仍由原聊天root独占物理环境：M03/1553、M03/1554各300s，再Game4轮换/1555。大包一秒及时门、穿插小包延迟、outer wire、实际rb与cleanup继续原口径；候选好样本不覆盖旧S01/S16/M03 FAIL。低档r12/5305当前55/60与旧~2.18s尾部继续OPEN：完整k20/r12独立p30的理想源残余5.7724%只是条件参考，分片/partial/相关性和缺失探针不能忽略。不增加FEC/窗口/期限，不以丢迟到包美化p99。

本次只有两条新SOURCE完整原生样本，P7/项目仍IN_PROGRESS，full70/final18/1800s及其他配置/生命周期/线路资格未因此变成PASS。原3a/c853证据和冻结ref保持独立。
