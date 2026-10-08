# 固定 b4 原生五条收口：raw 实效通过，MTU/下行故障保留

产品/助手 SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`，冻结 ref `qualification/raw-scoped-buffer-20261008` 不动。配套包仍为 P6 run37713269147；此前16条 exact-source Actions/core/race/平台/生命周期/六独立性能及manifest/每文件hash核验均通过。三a/c853历史成绩不继承到b4。证据 `docs/evidence/raw-scoped-buffer-b4ea061-physical-20261008.json` 更新为五条完整原生样本、原回执hash、实际rb、独立清理读回和1554故障计数。

## 用户确认的无 HOL 验收

2026-10-08用户明确：有Actions并发无HOL证据即可认定该项通过，不强制物理机重复。固定b4 foundation37713152863运行 Windows/Linux `go test ./... -count=1`、Linux `go test -race ./... -count=1`，均通过；相应源测试无skip：

- `runtimeowner.TestRuntimeNormalNoHOLRepairReplacementAndGenerationFence`：先丢第一条记录，后续独立记录先交付，原密文修复后再交付第一条。
- `runtimeowner.TestACKFeedbackBlockedWriteKeepsNoHOLAndOnlyLatestACK`：实际goroutine/ACK worker被阻塞时接收和业务交付继续。
- `linkdata.TestCrossInnerMTUTinyTailFECRecoveryDoesNotHOLSmallDatagram`：8996B与25B尾IP fragment分别进入真实LINK/FEC；尾碎片丢失时，同FEC块的独立96B包在4ms先交付，101ms再恢复尾碎片。
- `tlsrecord.TestDecoderNoHOLAndMultiRecord`：独立记录乱序解密交付。

据此 `nohol_qualification=PASS_ACTIONS`，不把缺少重复物理并发测试继续列作阻塞项。现有原生MTU助手逐个发送并等待回应，只提供自身MTU/及时恢复证据；这项范围说明不否定已接受的Actions无HOL资格，也不让无HOL通过抵消失活、丢包或迟到。

## 五条原生结果

每条300s顺序执行；Normal1每方向10Mbps，Game4每方向逻辑3Mbps，FEC20:20、profile off；内层9000/外层1400，受控私网目标显式all。没有改变FEC/4096/窗口/期限/MTU/权限或全局rmem/wmem。普通负载为96/256/512/1000/1372字节混合；8973/65507最大包目前为稀疏功能测试，不冒充10Mbps连续超内层MTU混合资格。

|工况/seed|实际 raw budget|交付/延迟|raw drop|原分析状态|
|---|---|---|---|---|
|Normal1551，request0|212992B|双向9.999953M，0/0缺包，2980/2980，p9973.271ms|61|BUSINESS_PASS_TRANSPORT_PRESSURE|
|Normal1552，request524288|1048576B|9.999874/9.999953M，上行缺10/下行0；2980/2980，p9974.007ms|0|FAIL/business_loss；上行字节loss0.0010592%|
|M03/1553|1048576B|8973B171/171及时；65507B170/170及时；小包1362全及时；最大UDP p99394.450/max624.501ms|0|PASS_SCOPED_NATIVE_MTU|
|M03/1554|1048576B|接受1557包，回到客户端1479；78缺失；另1个65507B迟到1225.578ms|0|FAIL/五项及时交付门|
|Game4轮换1555|1048576B|双向2.999990M，0/0缺包；2980/2980，p99121.934ms；4个逻辑lane均换代，lease不变|0|PASS_SCOPED_NATIVE_CAPACITY；换代采样PASS|

Normal1552进程CPU约client275.95/server154.07 CPU-s/300s，与同源继承样本274.77/153.33接近；p99也接近。支持本条未观察到明显新增成本，不构成受控同源native RTT或跨版本CPU优化结论。Game CPU338.70/166.73 CPU-s另列；不同时间/模式/网络不能直接用p99差值归因程序。普通三条默认DNS60/60，指定NIC明文53观察、Npcap/user overflow、输入覆盖、完整性与owned退出检查通过。各条live snapshot写入失败计数4/2/3保留，不从离散快照宣称完整连续wire或亚秒A+B->B顺序。

## 1554两种故障必须分开

1. **开头约80s下行中断**。目标逐个收到并echo全部1557个接受请求，echo_send_errors为空；缺失的78包都在起始Sequence0..101，最后缺失发送于80.227s，第一条成功发送于81.349s。双方lease相同、source/ref匹配、pipeline overflow0、raw接收drop0。客户端generation1的Received/AuthenticatedRecords长期保持12，Rx FEC recovery deadline为0，说明没有进入客户FEC的后续记录可供恢复。服务器FreshSent和raw send_calls/messages持续增加，但所选接口/当前peer的S2C/40000有效TCP metadata停在14个payload包/3180B；双方header observer均同样观察。generation2/40001后恢复。
   这将断点收窄到server emit/marshal tuple与内核OUTPUT/路由/conntrack/qdisc到指定NIC匹配观察之间。观察器仅统计选定peer、可解析header，不能据此断言全部接口没有任何包，更不能直接宣布WAN或某条内核规则是根因。该段不是凭名义FEC恢复概率可解释的随机小损失，也不是raw receive budget不足证据。
2. **恢复之后的独立65507B迟到**。Sequence676于144.676s发送，145.902s收到，逐字节正确、RTT1225.578ms。此前约143/144s server RTO采样400/800ms、后续恢复200ms，显示退避仍会存在；尚无逐record因果映射，不可认定初始1s floor修复失效或这次一定由FEC不足造成。保留一秒及时门FAIL，不缩期限/丢迟到包美化p99，不任意增加缓存/重传强度。

1554返回小包p99115.419ms、最大UDP返回项p99230.302ms都不能掩盖缺失78包。原FAIL保留。原3a M03 late/missing、c853与b4低档r12旧~2.18s/缺失probe继续OPEN。完整k20/r12独立随机p30理想源残余5.7724%只作条件参考，不能替代实际k/r/partial/分片/相关性账本。

## 环境收尾与下一项

独立收尾确认：server配置与测试前逐字节相同，raw-recv-buffer键原本省略，故新程序默认请求524288已自动读回1048576，forced=true/limited=false；全局rmem/wmem四值与之前相同，服务原cap_net_admin/cap_net_raw不变。两端version均b4，ARM服务active，Windows owned客户端/任务0，测试地址/配置backup/owned物理进程0，raw pcap剩余0。完整有秘密配置没有上传；原配置hash比对留受保护本机，GitHub仅存结论及无秘密回执hash。没有安装新Npcap或修改系统设置。

下一项优先对固定b4复现初期下行断点：先核每条raw emit的src/dst/port/seq/ack/长度、普通发送与batch计数、OUTPUT/路由/conntrack/qdisc及实际NIC元数据，采用默认off、有界12s/固定数量诊断；不能先把责任丢给FEC或扩大buffer。若新增产品/助手观察代码，先Actions资格再物理，不编译/测试Go于本机。用户询问10M连续大小包：当前普通混合已测；若加入8973/65507，先资格化按总字节10Mbps的混合生成器，禁止同包数放大速率。每性能Action仍只一条，profile不能替代普通性能。修复必须有对应证据，原账号隔离/完整性/同seq同密文/generation/MTU/无HOL硬门保持。

五条目标测试已结束但不是全产品PASS：两条原分析FAIL保留，P7/项目仍IN_PROGRESS，full70/final18/1800s及其他原生配置资格未因此变为PASS。不要重新部署旧3a/c853或移动冻结ref，也不要重复已验真的并发无HOL项。
