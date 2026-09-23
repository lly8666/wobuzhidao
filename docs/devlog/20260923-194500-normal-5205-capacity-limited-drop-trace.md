# 20260923-194500 Normal 5205：业务性能通过，环境门控CAPACITY_LIMITED

## 样本

SOURCE_SHA `ef7a30bcfe53dcad767e5bffd09c2be8980f2cab`，`next-strict-weaknet` run `35846947558` / job `107135275382`，输入 Normal / 10Mbps / 5→20→5 / seed601 / 1lane / FEC20:20 / 300ms。

分类：

- CORRECTNESS PASS
- INPUT_VALIDITY PASS
- CAPTURE PASS
- ENVIRONMENT **FAIL**
- PERFORMANCE **CAPACITY_LIMITED**

唯一environment error：

`socket skmem drop max=164 endpoints={'server/ss_packet': 164}`

规范明确本机overflow不能解释成预期netem丢失，所以不能把本样本记PASS。

## 业务与恢复事实

这条样本没有业务性能错误：

- performance_errors为空；
- qdisc实际loss：c2s 4.990% → 19.970% → 5.004%，s2c 5.055% → 20.054% → 5.011%；
- stress 20%阶段两方向最终packet/byte loss均0，eventual goodput约10.000Mbps；
- delay-aligned wall stress：c2s 10.00116Mbps，s2c 10.00033Mbps；
- post5在offset=1s即出现连续3个资格窗口；
- probe timeout=0，10s drain后late bytes=0；
- link drop全0。

恢复账本符合loss-tolerant方向：

- client最终 abandoned=106814、forgiven gaps=81935、repair segments=186；
- server最终 abandoned=107258、forgiven gaps=81755、repair segments=167；
- repair outer仅c2s 51188B、s2c 41243B；
- FEC承担主要恢复；repair没有形成带宽风暴。

资源：

- client CPU 92.28s，server 95.88s / 120s采样；
- server AF_PACKET max rmem=828672 / rb=1048576（约0.790），但累计drops=164；
- full artifact `10744205038`，723324771B，digest `sha256:ab9027b595b2d22f6a6f152315c63cf47aa91e1e217195274948872af57647ea`；
- summary artifact `10743164327`，digest `sha256:4678e8b24e087cf2a29abea7c4aeb118be3c8c47aa398c3dca77a40f338e5298`。

同SHA foundation run `35846907092` 与 targeted run `35846907137` 全部适用jobs PASS，历史measurement jobs继续SKIPPED。

## 下一步：零采样drop timeline

不启动5305，不降低环境门槛。

扩展 `next-performance-artifact-reader`，只读取既有full artifact：

- 逐秒解析resources.jsonl中的server/client AF_PACKET skmem；
- 记录drop累计值发生增量的精确样本、stage、elapsed time、r/rb、CPU增量、run queue；
- 对每个drop事件对齐最近client/server runtimeowner transport计数；
- 继续输出整体bounded-recovery计数。

若drop是产品接收停顿可复现证据，则做最小路径修复；若是单次runner瞬时异常，则保留本run CAPACITY_LIMITED并用同配置独立新run复核，不能追溯改绿。
