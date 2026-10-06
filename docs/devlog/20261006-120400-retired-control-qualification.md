# 退役控制包修复的独立资格验收

## 本轮目标和阶段

开始SOURCE60f6544aa09259c8589c39eac08019ae284116c4，分支next/tlslike-dataplane。产品逻辑来自81939eb，仅fixture增加真实业务交付发布barrier。固定qualification/60f6544-retired-control-20261006；本轮无新产品修改。

## 修改与原因

更新唯一STATUS、AGENTS与ROADMAP顶部，消除仍写D04正在跑/helper pending的旧入口。历史失败与日志不改。准备源码严格固定的独立性能dispatch/有界artifact collector，不下载大pcap、不继承921结果。

## 复用来源

使用既有strict单样本harness/配对p95与p99分析；不提取old。

## Actions证据

SOURCE60f：[predelivery37411565811](https://github.com/lly8666/wobuzhidao/actions/runs/37411565811)四job PASS，含60次failed-Wake/SYN/retired race和30次replacement barrier重复；[lifecycle37411565849](https://github.com/lly8666/wobuzhidao/actions/runs/37411565849)、[steady37411565840](https://github.com/lly8666/wobuzhidao/actions/runs/37411565840)、[padding37411565856](https://github.com/lly8666/wobuzhidao/actions/runs/37411565856)、[GUI37411565897](https://github.com/lly8666/wobuzhidao/actions/runs/37411565897)PASS。基础37411565900、Linuxserver37411565981、fullstack37411565847及手动预检37411859529继续验收；当前性能/P6未跑。最终结果追加本日志，不提前声明。

## 问题、排查与风险

旧42 failed-Wake独一根因没有callstack证据，不能因本专项重复成功宣称已唯一定位。819新增fixture失败是真实发布barrier缺失，已保留原始失败。新closed-source marker只隔离已退役association，不吞当前活跃错误/EPERM/EBADF；成功热路径无新锁、计数或日志，仍需真实性能门。

D04完整300s功能结果和34历史完整native/13工况/30NOT_RUN保持。MTU异常仍需可观察Windows边界，不通过改MTU、缓冲、FEC/4096掩盖。DNS部分少量上行业务missing及输入send-lag p99未观测不关闭质量门。

## 下一项原子任务

同源码基础门、37生命周期与预检完成后，Normal10/lossless+5205分别独立Action，合格再Game4/3M/lossless+5205+5305分别独立Action，逐阶段同seed同harness配对p95/p99及原始pacing检查，之后同源码P6再部署。若有失败先按raw证据修复，不把runner解释当PASS；产品后续MTU诊断独立任务。

## 追加：正确性门收口

60f九项scoped门全部PASS。完整lifecycle37411565847下载aggregate source核验、37jobs全success、36samples failed/errors为空。独立Normal lossless37412573311、520537412577103已启动，各自一run一条；P637412581223并行准备同源码包，不代表性能通过或允许提前部署。raw重复日志保留retired-control-gates/37411565811.log，明确-count60成功。
