# 20260923-224500 最终18份暂停：定位AF_PACKET短突发接收停顿并实施最小解耦

## 固定SHA矩阵已取得的证据

固定SOURCE_SHA `b0f5386cd468903507c68fb4b7986fff318d70de` 的最终18份资格已经完成前5个身份：

- Game4 / 5205 / seed101：run 35859280700，五类PASS；
- Game4 / 5305 / seed101：run 35859842587，CORRECTNESS/INPUT/CAPTURE PASS，ENVIRONMENT FAIL，PERFORMANCE CAPACITY_LIMITED；
- Normal1 / lossless / seed101：run 35863722306，五类PASS；
- Game4 / lossless / seed101：run 35864318697，五类PASS；
- Normal1 / 5205 / seed101：run 35864958803，CORRECTNESS/INPUT/CAPTURE PASS，ENVIRONMENT FAIL，PERFORMANCE CAPACITY_LIMITED。

规范明确CAPACITY_LIMITED不是PASS，因此停止继续dispatch剩余矩阵，不择优重跑、不覆盖旧证据。新产品修复后最终18份必须在新的同一SOURCE_SHA重新开始。

## Game5305：client packet socket单秒drop

run 35859842587 / job 107177085087 / full artifact 10749696995：

- client AF_PACKET累计1070 drops，全部集中在stress的同一个1秒采样间隔（elapsed约70.047s）；
- server packet socket、link、qdisc均0 drop；
- client packet socket全程采样到的rmem峰值101568/1048576；drop采样时已回落为0，符合短突发/短停顿后恢复而非长期buffer常满；
- drop附近CPU pressure some avg10约29-31%，无cgroup throttling、无memory pressure；
- 四lane transport均FreshBlocked=0、FreshEmitFailures=0、FreshWindowBypass=0、RepairEvictionMaxScan=1，不是此前repair全窗扫描/HOL复发。

client正式Linux入口为RawIPv4Endpoint -> SegmentMux。SegmentMux每route已有256槽有界channel，但此前没有route peak/bytes/age/full-wait诊断，因此本轮只补观测，不先扩大client容量。

只读诊断（均在main运行，不是性能样本）：
- run 35863328967：clientdrop timeline；
- run 35863510197：drop上下文。

## Normal5205：server容量1 handoff与drop时间点对齐

run 35864958803 / job 107194116879 / full artifact 10752362377：

- server AF_PACKET累计58 drops，全部集中在post的同一个1秒采样间隔（elapsed约93.044s）；
- client packet socket、link、qdisc均0 drop；
- performance errors为空；stress C2S最终packet loss约0.0054%，S2C为0，post5通过；
- drop前一个资源采样server rmem已经达到725184/1048576（约69%）。

LifecycleServer.Run当前路径为AF_PACKET reader goroutine -> 容量1 readCh -> 同步handleSegment/downstream。现有qualification-only pipeline timing把drop与短停顿直接对齐：

- drop前：handler max <9.6ms；
- drop点：handler max升到11.596202ms；
- 同一点handoff_block max升到11.647959ms；
- queue_age max升到11.756511ms；
- downstream max约6.009ms且无>10ms；
- ready queue没有长期堆积，说明是短时stall而不是持续业务堵死。

只读诊断：
- run 35865556593 / 35865556552：packet-drop timeline/context；
- run 35868717069 / job 107206856773 / artifact 10753736469：pipeline timing。

因此证据支持：当kernel packet socket已有较高积压时，容量1的userspace handoff让reader在约11.6ms同步处理停顿期间停止drain，短突发可越过剩余buffer并触发AF_PACKET drop。

## 本次最小产品修改

1. Server与LifecycleServer共用固定256槽server receive handoff。
   - 不改kernel SO_RCVBUF；
   - 不改协议、FEC、repair、loss门槛；
   - channel满时仍阻塞reader，保持显式背压，不在userspace静默drop；
   - 现有ready_current/peak/bytes/queue_age继续给出有界队列证据。
2. SegmentMux route容量保持既有256，不先扩容。
   - 增加qualification-only diagnostics开关；
   - 每route记录capacity/current/peak、bytes/bytes_peak、full_waits、handoff_block、queue_age；
   - 正常未开启diagnostic时不做逐包time.Now/atomic队列统计；
   - Linux client diagnostic JSON在保持原owner/lanes顶层schema的同时增加segment_mux字段。
3. 增加unit测试：
   - server queue固定有界且释放slot后恢复，不接受超容量；
   - SegmentMux能报告route满队列背压、容量/峰值/bytes和queue timing。

256槽不是通过扩大内核buffer拖延overflow；它是userspace reader/handler之间的固定短突发解耦，按当前约万级pps量级覆盖几十毫秒而仍严格有界。client是否需要调整继续由新增SegmentMux证据决定，不从server结论外推。

## 验证顺序

1. repository contract + runtimeentry/core + Linux race + targeted/foundation；
2. 只跑一条Normal1 10Mbps 5205目标样本，要求server AF_PACKET drop=0且既有输入/正确性/capture/performance门全部满足；
3. 再跑一条Game4 3Mbps 5305样本，读取新增segment_mux peak/full_waits/queue_age：
   - 若route未满而base AF_PACKET仍drop，继续查raw reader调度；
   - 若route达到256且full_waits对齐drop，再做client侧第二个最小有界修复；
4. 两模式稳定后，在新最终SOURCE_SHA重新开始18份矩阵；`b0f5386cd468903507c68fb4b7986fff318d70de` 的部分矩阵永远保留为历史证据。
