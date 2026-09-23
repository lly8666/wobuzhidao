# 20260923-160000 steady pending payload 的 Segment 重复clone

## 为什么继续改这一处

ordered A/B run 35825736119 已证明上一处 post-Sendto packet clone 清理不是慢runner上的决定性差异：

- 快runner AB：a924 before 与75b5 after都全PASS、约10Mbps，CPU约59s/process，handler约19us/read；
- 慢runner BA：before/after都CAPACITY_LIMITED，CPU约115–119s/process，handler约200us/read，after server/client AF_PACKET drops约543065/77916。

因此不能再把跨runner CPU减半归因给75b5。目标仍FAIL稳定性门，strict 18矩阵继续不跑。

## 确定的下一份实现浪费

`laneTransport.send` 在进入pending repair owner时已经执行：

`payload: append([]byte(nil), record.Wire...)`

所以 `pendingRecord.payload` 是transport-owned的最终TLS-like wire/ciphertext副本，必须保留到ACK/SACK/repair retirement以保证同Seq同密文。

但每次fresh/repair构造FakeTCP Segment时，`outboundSegmentFlags` 又执行：

`Payload: append([]byte(nil), payload...)`

随后正式Linux `RawIPv4Endpoint.WriteSegment` 的 `MarshalSegment` 会再把 `seg.Payload` copy进最终IPv4/TCP packet。第二层Segment clone不提供repair所有权，也不改变wire。

## 安全边界

本提交只把 `Segment.Payload` 改为借用传入的pending immutable slice：

- pending本身仍持有唯一repair-owned密文副本；
- fresh Emit为同步调用，正式raw serializer在返回前已经把payload写进最终packet；
- repair同样从current pending构造并同步Emit；
- ACK/SACK retirement只把 `p.payload=nil` / 删除map引用，不修改backing bytes；
- Segment值若被测试emitter保留，其slice自身仍持有backing array，不会因owner丢引用而失效；
- 不允许修改payload内容，既有实现从未修改pending payload；
- `TestSteadySelectiveACKRetiresPayloadAndFastRepairsHole` 会在SACK retirement后仍比较首次fresh与fast repair payload完全相同，继续覆盖same-Seq/same-ciphertext关键性质。

不改变raw recv ownership、raw packet ownership、4096、FEC档位、Game副本、MTU、nonce、ACK/SACK/repair算法、注入速率、队列或生命周期。

## 并行验证状态

父提交 `5f711af1171a3b6ceb01ea891d5e04f9cc866325` 修复LifecycleServer qualification publication race：
- `next-lifecycle` run 35826779814 core已PASS；
- 36份 `next-lifecycle-fullstack` 正在执行；
- strict 18矩阵已显式门控，没有因该生命周期提交提前启动。

## 本提交验证

push后自动：
- next-performance-recovery：targeted core/race + Normal10 seed601；
- next-performance-capacity-diagnostic：Normal10 seed631 compact CPU/4096/AF_PACKET timeline；
- foundation/P4 core/race。

如果两个定向Normal显示改善或PASS，再把ordered A/B+B/A改成 parent race-fix-only SHA 对本payload-clone修复SHA。任一慢runner仍CAPACITY_LIMITED时继续保留FAIL并从compact per-core/PSI/handler证据选下一单因，不扩大4096/buffer、不降FEC/速率/Game。
