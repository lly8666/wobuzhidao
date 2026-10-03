# 20261004-014100 双模式换代短窗通过，冻结新源码完整资格

## 本轮目标和阶段

P5换代返工验证。真正测试及后续打包SOURCE_SHA固定2b2bd9eb106d7c6fa83096cd88a59a0d0bfae8f8；本提交只更新证据/状态/文档，不把文档HEAD当测试二进制来源。

## 修改与原因

无新产品代码。记录有限retiring接收、每包全扫描改为一次FIN启动/常数维护、未发布candidate握手拒绝局部隔离的Actions结果。原失败均保留。同步章程默认值文字与真实CLI/PARAMETERS：idle-dormant默认0、rotate-min/max默认0/0；历史15分钟休眠/30～60分钟轮换是可配置策略，不能当当前程序默认，也不能让新agent按旧描述改回代码。

## 复用来源

无；当前正式实现、不可变Actions artifact只读核验。

## Actions证据

精确源码基础37140620422（Linux/Windows unit/build、Linux race、真实kernel fallback、共享TUN iptables/nft、OpenWrt TPROXY）PASS；tools/30定向race37140620452、生命周期core37140620391、padding37140620399、steady-target37140620388 PASS。36功能生命周期push37140620398当时排队/运行，不能提前PASS。

独立Normal180s [37140842423](https://github.com/lly8666/wobuzhidao/actions/runs/37140842423) PASS原门、新增原发送1s loss门、内部queue0门。每方向目标10Mbps/FEC20:20/300ms单向/三轮5→20→5/50s轮换。最低阶段goodput9.994948267Mbps、最大阶段packetloss0.027022645%、最差1s0.405022276%；probe阶段p95最高615.462931ms、p99最高619.035521ms。两端内部queue/kernel/socket/capture drops0、完整性0、generation1→5、FEC/LINK停流排空门PASS。server replacement_checks65，queue peak272/4096、age max21.346722ms、handler max8.943031ms。client/server CPU120.97/122.61 CPU-s/180s，heap峰26.68/25.65MiB；跨VM不作固定CPU改善百分比。summary artifact11280466930 ZIPsha25633f24cf112443c128b4d0a6424b9521332d3f516a159010cfbc48eb687e8e96d；diagnostics11280427064 ZIPsha256a2baac66495e92cca2ed3d10acc00bdd1262da4a70d6ccfbb2ad36683e80105b。

独立Game180s [37140844185](https://github.com/lly8666/wobuzhidao/actions/runs/37140844185) PASS相同门；每方向3Mbps/4lane/FEC20:20，其它网络/阶段不变。最低阶段2.997930667Mbps、全部阶段/每秒packetloss0；probe阶段p95最高607.864532ms、p99最高608.977429ms。两端内部queue/kernel/socket/capture drops0、完整性0，4lane各generation从1/2/3/4→5/6/7/8，自动换代和排空门PASS。server replacement_checks71、queue peak532/4096、age max33.223697ms、handler max8.136317ms；client/server CPU163.83/154.24 CPU-s/180s，heap峰54.90/52.85MiB。summary11279159761 ZIPsha2561ddc984fe7b481db380a8975b5ad92c3f64a2f32f55010a0ac6d5d84b6aeaed1；diagnostics11279644303 ZIPsha256b8910392c3b0c638127a7621f8bd19c20412bbfbc9257b63ca0a719decfa3606。四个ZIP均按GitHub metadata digest本地只读验证后解析。

主线冻结perf-fixed/2b2bd9e全40位及-r2/-r3/-r4。诊断controller37140675052；严格18独立原始样本controller [37140877004](https://github.com/lly8666/wobuzhidao/actions/runs/37140877004)进行中。完整78回执controller [37140925122](https://github.com/lly8666/wobuzhidao/actions/runs/37140925122)已以两条原始短测+foundation/tools为硬前置门，复用同ref原始短测，启动缺失配置/生命周期/黑洞/打包/两条1800s，不重挑VM或最佳结果。Normal1800 [37141231250](https://github.com/lly8666/wobuzhidao/actions/runs/37141231250)、Game1800 [37141232736](https://github.com/lly8666/wobuzhidao/actions/runs/37141232736)进行中。全部SOURCE同2b2，每性能Action一条；两个controller本身只读/调度，不承载性能负载。

## 问题、排查与风险

短测能证明原换代断流在此样本改善、candidate拒绝不再全server退出；不能替代正式30分钟或高丢包三seed矩阵。新短窗门仍允许人工link loss+2pp抽样余量，原阶段loss/RTT/input/资源门不变。旧b1长测阶段平均PASS但最差秒61.264%及内部queue20012drop永久保留；a86 Game原始server退出FAIL保留。无buffer/4096/FEC/3s期限扩张，不恢复HOL。物理P7、Windows真实驱动、ARM原生、真实路径PMTU自动适配仍NOT_RUN/未资格化。

## 下一项原子任务

读取完整78及严格18原始回执，审计两1800s逐秒损失、内部queue、尾延迟、heap平台期及停流排空；全通过后同SOURCE P6三目标包/manifest/hash/独立receipt核验，才可标hosted完成/可交物理。任一真失败保留原样，窄修后新SOURCE重新资格，不凭本日志提前关闭P5/P6。唯一交接入口STATUS；不要继续微优化或改用户负载。

## 01:44追加：严格18同SOURCE收口

Controller37140877004 SUCCESS。aggregate source2b2全40位、sample_count18、revision2/result PASS、errors空；campaign18回执、collection_errors空。artifact11279519959 ZIPsha2564187ff4e26c2b5779e009cdc26d1463d015b31fed5b6a87f756ae1186dc43c7c已只读核验。18份原始run receipt与按阶段配对RTT汇总进入evidence/rotation-handoff-2b2bd9e.json。此为同源码120s弱网资格，不替代正在进行的1800s换代长测；完整78/配置/P6与P7状态仍未关闭。
