# 20261006-212800 Npcap owned返回复制Actions收口

## 本轮目标和阶段

HEAD/source be456cee72a663a35d7e62fbedf17cc74bcbe410，冻结qualification/be456ce-owned-return-20261006，next/tlslike-dataplane/P7候选。产品运行改动只移除Npcap序列化返回clone；本轮仅记录精确候选定向资格与同源包进度。

## 修改与原因

证据/状态更新。五功能run：foundation37467445072、steady37467445034、GUI37467445206、preflight37467445354、lifecycle37467445046全部SUCCESS，含真实unit/build/Linuxrace、Windows序列化所有权四边界与原native批量partialprefix/fallback/generation回归、GUI和生命周期。所有run attempt1/head exact，未经本机测试。

## 复用来源

无。读现有严格分析器identity/probe_valid与p95增量200ms/p99增量500ms常量，未降低门槛，汇总只读四独立产物。

## Actions证据

Normal5205/seed1470 run37467498831、Game5205/seed1471 run37467504952；同seed lossless分别37467631403/37467636231。每run只有一条120s/一种模式配置，各自五分类CAPTURE/CORRECTNESS/ENVIRONMENT/INPUT_VALIDITY/PERFORMANCE全PASS，socket/linkdrop0。Normal1双向10M/Game4双向3M/FEC20:20/单向300ms/5→20→5保持。四份strict summary sourceexact，三阶段12个p95/p99配对门全PASS，最大p95增量10.976894ms/p99增量13.062689ms；基线/有损探针30/60/30全回。

九run证据windows-npcap-owned-return-be456ce-actions-20261006.json/receiptgz。GitHub读API两次timeout，按原run只读重试，未重跑测试/更换seed/隐藏失败。P6run37468358428已触发，三目标hash/manifest结果仍待收集。full70/full18/1800s/nativeNOT_RUN，不继承a280/024。

## 问题、排查与风险

这是Windows发送路径微优化；严格性能环境为Linux，主要证明其余数据路径未退化，不证明Windows驱动调用CPU或p99改善。Normal5205CPU102.65/103.87s、Game56.29/53.47s，不同runner不拿总CPU直接A/B判收益。理论每encode去掉一个packet长度clone；实际分配/CPU/尾延迟必须profileoff原生确认。a280CPU诊断溢出3233/p99663ms/148missingFAIL、无profileon/offDNS样本p99286/332ms与M03missing/late继续保留，不假称修复。

## 下一项原子任务

P6 exactsource hash/manifest/Actionsreceipt三个平台全部PASS才同源部署；保留a280可回滚包/服务器binary、配置与installationID。不重装Npcap/不改系统foreign网络。一次native Normal1/FEC20/10M/300s seed1472，关闭CPUprofile、原diagnostic1s单列成本，检查业务完整性、全部探针、p99、processCPU、allocatedbytes/allocations、nativecalls/send-lock/recvqueue及正常cleanup。WAN不受控不夸大CPU因果收益。失败则继续窄诊断，不继续下一项算法优化。
