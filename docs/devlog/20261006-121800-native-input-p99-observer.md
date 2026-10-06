# 原生负载助手补有界发送延迟p99

## 本轮目标和阶段

始于SOURCE60f6544，产品资格冻结在该SHA。其九项scoped正确性门及37生命周期已PASS，独立Normal37412573311/37412577103和P637412581223正在跑。本提交只改助手/门/状态文档，没有产品Go变化。

## 修改与原因

tools/physical_udp_client.cs与physical_udp_server.py的原MaxSendLagMs复用每批起点elapsed，因此未包括批内执行延迟，且原本没有p99。增加实际每次业务send前的monotonic clock，按原byte-budget包尾deadline计算lag，成功send后记录。原32包预算/1ms sleep/大小组合/byte-budget/dedupe/socket buffer/probe与stop/drain不变，没有为了数字改成新负载算法。

C#/Python各固定102桶，0.1ms向上舍入，>10ms归overflow；p99进入overflow时报告实际最大值作为保守上界，绝不伪造精确p99。记录计数必须等于成功业务TxPackets，0样本不能通过输入资格。输出Samples/Overflow/Resolution/P99UpperMs/PacingMode；不保存逐包timeline/body/pcap。固定计数器内存与测试时长/PPS无关；每包增加一次读clock/计数，实际helper CPU需继续报告，未宣称零成本。

test_physical_send_lag.ps1/Python固定向量涵盖边界、1%/2%异常值、p99与max区别、nonfinite拒绝、空样本、100000次下固定空间。next-predelivery-tools编译C#后执行PS向量，Python向量也是普通unit，无网络负载/driver access。所有测试在Actions；本地仅编辑。

## 复用来源

复用现有native正式助手时钟和byte-budget，无old提取/新产品参数。strict性能助手realpath_udp_duplex.py完全未改，60f性能/source/helper配对仍固定60f。

## Actions证据

60f实际九门与37生命周期见evidence/retired-control-60f6544-20261006.json及120400日志追加。新增助手提交后的compile/固定向量当前NOT_RUN，取得PASS后才能上传远端。候选P6可并行准备包，但性能和raw pacing未过不得部署。

## 问题、排查与风险

旧D04/MTU等证据输入p99仍NOT_EVALUATED，不能从旧max倒算或借新助手宣称历史质量PASS。新0.1ms上界仅衡量输入节奏；不替代业务loss、RTT p99、actual注入/host capacity/no-HOL。超过10ms的overflow上界可能偏保守，保持明确报告，不降原门。Nativebyte-budget-batch32不是strict absolute-deadline-sleep-v1。

M03尚需Windows写TUN前的受控逐片证据；Pktmon空事件UNSUPPORTED仍保留。当前只改助手，没有引入TUN观察器、改MTU或扩缓存，34份完整native/13工况/30NOT_RUN计数不变。

## 下一项原子任务

收60f Normal两条原始性能/pacing和同源RTT配对；通过后Game4 lossless/5205/5305三个独立run。同源P6hash/receipt与性能全部过门再配套部署。新增助手先compile/vector PASS再新实机独立样本，不补写旧成绩。
