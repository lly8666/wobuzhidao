# 20260923-163000 lossless Normal10 的 record-BDP 与 pending-payload clone A/B

## 先回答 4096 的含义

当前定向性能样本是 Normal 单 lane、FEC20:20、padding off、业务每方向 10Mbps，scenario=lossless；但网络仍固定 300ms 单向时延，所以 RTT 约 600ms。

`MaxOutstandingRecords=4096` 不是“只有丢包时才使用的重传缓存”。每个 freshly-sent steady record 在收到累计ACK/SACK并退休前都占一个 pending record slot。即使网络完全不丢包，只要RTT非零，就会存在大约：

`record_rate_per_s × RTT_s`

个尚未ACK的records。

父SHA `5f711af1171a3b6ceb01ea891d5e04f9cc866325` 的 seed631 PASS capacity样本：
- client record rate ≈6526.11 records/s；
- final SRTT ≈600.47ms；
- record-BDP ≈3918.74；
- client PeakOutstanding=4029；
- server record-BDP≈3920.03，PeakOutstanding=4027；
- 0 Abandoned / 0 RepairEvicted / 0 retransmit / 0 socket drop。

也就是健康无损时已经天然使用约95.7%的4096，理论headroom只有约176 records；按6526 records/s换算约27ms。只要runner处理停顿、ACK处理延迟、调度抖动让effective RTT多几十毫秒，就会撞4096。

所以“无丢包性能应该高”是对的；但“无丢包4096应该很空”在当前设计和600ms RTT下不成立。若想让4096占用显著下降，必须降低records/s（减少分片/每业务字节record数量）、降低RTT，或改变ACK-retirement模型；单纯优化CPU只能让系统更稳定地维持理论BDP，不会把BDP本身变小。

## 当前head与Actions

当前head `1eeb9e254696724cf4c16787fe006a3eec43beb9`：删除steady `outboundSegmentFlags` 对transport-owned immutable `pendingRecord.payload` 的重复整record clone。pending repair缓存和raw serializer最终copy仍保留，因此same-Seq/same-ciphertext和raw packet ownership不变。

### seed601 recovery
run 35827109727 / normal10-lossless job107072197444：
- 五类classification全部PASS；
- scenario=lossless；
- 双向三阶段约10Mbps；
- socket/AF_PACKET drops全部0；
- client/server CPU≈49.85/49.57 CPU-s / 120s；
- PeakOutstanding=4017；
- core-race job107071865834 PASS。

### seed631 capacity
run 35827109667 / job107071262594：
- CAPTURE/CORRECTNESS/INPUT_VALIDITY PASS；
- ENVIRONMENT FAIL，PERFORMANCE CAPACITY_LIMITED；
- 四个CPU busy≈99.1%，steal=0、cgroup throttle=0；
- CPU PSI some avg10峰52.8；
- client/server CPU≈119.03/115.35s；
- server/client PeakOutstanding=4096；
- first >=4000约1–2s，first Abandoned约4s；
- server/client AF_PACKET max drops≈579450/77801，并有UDP local drop；
- server handler累计104.3s/438245 reads，约238us/read，明显处于慢runner容量层。

该失败不能包装成产品PASS，也不能简单归咎“VM慢”；它证明当前实现离在最差hosted runner上稳定承载该目标仍有余量不足。

## 生命周期race验收

父提交 `5f711af1171a3b6ceb01ea891d5e04f9cc866325`：
- next-lifecycle PASS；
- next-foundation PASS；
- next-p4-steady-targeted PASS；
- next-lifecycle-fullstack run35826779865 共36个sample + aggregate，37/37 jobs PASS。

因此qualification publication race已闭环，不再是当前阻塞点。

## 下一步

将 `next-performance-ab-game` 改为：
- BASE=`5f711af1171a3b6ceb01ea891d5e04f9cc866325`
- FIX=`1eeb9e254696724cf4c16787fe006a3eec43beb9`

AB和BA各自在同一runner顺序执行before/after，另保留Game4 logical3。这样只比较本次payload clone删除本身，排除跨runner容量层级差异。

若same-runner显示仅2–5% CPU收益但慢runner仍CAPACITY_LIMITED，下一优化方向不再围绕4096本身，而是：
1. 降低每个业务datagram形成的steady record数量；
2. 减少每record FEC/LINK/owner/handler固定成本；
3. 检查ACK产生/处理是否有可合并但不增加等待的固定开销。

禁止扩大4096、扩大socket buffer、降低10Mbps/FEC20:20/Game副本或改变600ms RTT来冒充通过。
