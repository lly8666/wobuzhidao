# p99异常先定位客户端共同停顿，增加有界时间证据

## 本轮目标和阶段

分支next/tlslike-dataplane，开始HEAD16ecc97。产品部署仍SOURCE660b370；原Game5305/seed1382 p99 FAIL不变，用户要求p99和吞吐不可退化。仅添加失败归因所需观测，尚无修复结论。

## 修改与原因

原小诊断进一步显示客户端四条SegmentMux队列同时出现约1298–1303ms最大排队、峰值2377–2418段，client ingress峰值966；业务源在该秒正常注入、上行target82秒接收0后83秒集中861840B。比笼统的FEC/VM推断更早的边界是客户端共同停顿。客户端transport详细timing当时未开启；不能把未观测的0解释为耗时0。

tools/realpath_udp_duplex.py只在现有每秒probe收到时保存seq/sent/received/reply_sent单调钟，最多8192项、超限明示计数；目标回包字节/大小/发送节奏不变，普通业务路径无新时钟。区分上行、目标echo、下行延迟；只保存数字，不保存payload/凭据。

internal/qualificationdiag/jsonl.go在显式诊断1Hz路径保存真实observed UTC与memory/product采集耗时，原ticker UnixNS兼容保留。用于判断ReadMemStats或snapshot收集是否导致/隐藏全局暂停；这只是待验证假设，不能先宣布诊断造成p99。正常不开diagnostic的程序完全不调用这些新时钟，未变更FEC/repair/socket/4096/Game竞速。

## 复用来源

复用现有UDP probe和JSONL诊断入口，无新协议或用户参数。

## Actions证据

新增probe测试覆盖同host上下行拆分、echo错误可见、8192界限且业务统计不受影响，接入performance-analysis-unit。新增Go observer延迟测试真实注入20ms snapshot延迟，验证原scheduled timestamp与新采集耗时。开发机未跑Go/Python产品测试；本提交foundation/core/race/analysis及独立性能Action待执行，不写PASS。

## 问题、排查与风险

原run37255274295总CPU未持续打满、GC累计暂停变化几ms，不排除调度/共享锁/阻塞syscall或诊断STW；目前证据不足修算法。新的probe每秒数次操作，JSONL每秒三个clock只在观测路径；仍须Actions验证无吞吐/p99回归，不能凭代码简短宣称零影响。nativeS18功能已PASS但无probe，p99仍NOT_EVALUATED。

## 下一项原子任务

先Actions正确性和race，再单run单样本同seed Game5305及lossless，Normal/Game5205；不覆盖原失败，不降低500ms配对p99门。读新增逐probe分段时间与client observer耗时，确定阻塞点后再窄修。helper16正确门通过后继续原生S19/S20及最大UDP序号诊断，部署SOURCE660不变。每性能Action只一条。
