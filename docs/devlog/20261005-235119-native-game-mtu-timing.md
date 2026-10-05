# 四路大包功能完成，补有界事件时间定位迟到

## 本轮目标和阶段

P7原生M02，开始HEAD760f04a，产品SOURCE7eeb配套二进制不变。完成seed1412证据并只改测试助手。

## 修改与原因

native_mtu_probe.cs记录成功发送的sequence/size/DF、单机Stopwatch RTT、SendCall和UTC，接收时间在内容校验前采集。native_mtu_boundary_target.py每次原有echo发送记录recv/echo UTC及SendCall单机monotonic，最多8192条。保留原1s timeout/100ms pacing/3s drain/完整性/发送错误门，未增加重试、cover、产品计数或逐包payload。test_native_mtu_target.py覆盖独立确定时间与发送错误传递，原真实UDP最大合法/非法边界保留。共享CPU开销只属于低速功能助手，不把其当产品benchmark。

## 复用来源

既有本分支MTU工具，无old模块提取。

## Actions证据

新助手NOT_RUN，提交后next-predelivery-tools验证Windows CSharp编译/Python功能fixture。产品仍固定7eeb的既有精确scoped Actions资格，不因helper更新重打包或继承未跑全量资格。

原生seed1412完整300.0969s，2747/2747 exact、1373 small全准时、所有坏payload/重复/异常send/recv0，exit0和owned清理0。各DF/1399..9000IP功能通过；9000IP/DFfalse98次中1次>1s后收到，Timely97、LateExact1、Timeout1，精确时刻/RTT缺失。状态PASS_WITH_LATE_RESPONSES，不宣称延迟通过。窗口外层最大C2S1290/S2C1340、checksum/碎片/同seq冲突/非法TLS头0，capture drop0，raw删除；仅窗口外观证据。见evidence/native-game-mtu-7eebdcf-20261005.json。

## 问题、排查与风险

此低速probe不证明目标吞吐或正式p99，跨机UTC未确定时钟误差不能推出单程。旧1412长尾与历史rawp99、maxUDP和wake残余loss未关闭。完整native计数28/12工况跨源码含FAIL/PARTIAL不等于28PASS，余31caseIDs未验。归并physical_current近期结果至唯一completed数组，避免并行状态入口。

## 下一项原子任务

Actions助手门通过后，原生同源M02 seed1413独立300s；逐sequence匹配target/client与同机诊断时间，报告RTT分布/late/最大值/SendCall，定位最早停顿边界后才改产品。然后M03最大UDP及剩余矩阵。每性能Action只一条，不扩大socket/FEC/4096或引入HOL。
