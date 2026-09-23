# 20260923-145000 4096压力时间线与发送后冗余packet clone修复

## Actions验收

SOURCE_SHA `65735cb28be9eada7250133fe29d65ed2b613563`。

- `next-performance-capacity-diagnostic` run **35821083282** / job **107052861476** SUCCESS，compact artifact **10732998342**，digest `sha256:616c2f365a3f289a7a2d382f7317da57ecdf0fc62d9be175111906094acd8c31`。
- 样本固定产品a924、Normal1 lane、FEC20:20、lossless、seed631、每方向10Mbps；分类为 CORRECTNESS/CAPTURE PASS，ENVIRONMENT FAIL、INPUT_VALIDITY FAIL、PERFORMANCE CAPACITY_LIMITED。C2S eventual goodput pre/stress/post约7.38/2.79/2.55Mbps；S2C约8.73/6.43/6.39Mbps。
- server AF_PACKET max drops **515669**，first drop **19.051s**；client max drops **73957**，first drop **20.052s**。

## 因果先后

服务端：
- PeakOutstanding>=4000：**2.0015s**；
- first Abandoned/RepairEvicted：**13.0015s**；
- first AF_PACKET drop：**19.0514s**；
- final Abandoned/RepairEvicted：**387634**，Outstanding=4096；
- final record-BDP估算约 **4082.1**，距4096仅约14 records。

客户端：
- PeakOutstanding>=4000：**2.0350s**；
- first Abandoned：**20.0350s**；
- first AF_PACKET drop：**20.0519s**；
- final Abandoned/RepairEvicted：**326504**，Outstanding=4096；
- final record-BDP估算约 **4141.0**，已高于4096约45 records。

因此本轮证据明确否定“AF_PACKET drop先发生再把pending推满”：4096 pressure早约17秒出现，服务端实际丢弃repair-owned fresh record也早约6秒出现。

## runner/CPU证据

- cpu0..cpu3 busy约 **97.6–97.8%**；
- softirq约 **3.9%**，steal均 **0**；
- client/server process CPU约 **115.19/112.04 CPU-s / 120s**；
- cgroup `nr_throttled=0`、`throttled_usec=0`；
- host CPU PSI `some avg10`峰 **51.75%**，full=0；
- server handler最终599468 samples / 98.14s，handoff 107.23s，最差1秒区间handler均值约377us、handoff约429us；
- server TotalAlloc约 **7.57GB** / 862 GC；client约 **8.81GB** / 1104 GC。

这说明不是steal或cgroup限额；目标路径自身在4核runner上形成明显CPU调度竞争。仍不能简单写“机器不行”，因为存在可审计的每包实现开销。

## 本轮最小修复

正式Linux client/server的SegmentIO.Emit都执行 `_, err := raw.WriteSegment(seg)`，即返回的raw packet bytes直接丢弃。当前 `RawIPv4Endpoint.WriteSegment` 中 `MarshalSegment` 已创建新的独占owned packet slice，同步 `Sendto` 完成后又 `append([]byte(nil), pkt...)` 完整复制一次再返回。

产品提交 **75b5cc9a82458a7a46d382373339193276d6c51e** 删除这个第二次整包clone，直接返回已经owned的 `pkt`。API所有权、checksum、IP ID、MTU、nonce/ciphertext、repair缓存都不变。

本轮暂不同时删除runtimeowner构造Segment时的payload clone，保持一次一个原因。

## 验证基础设施

提交 **6c061d3034aae85d05323d64376b1807101ed109** 把capacity workflow改为：
- `internal/faketcp/**`、`internal/runtimeowner/**`、`internal/runtimeentry/**` 性能提交自动触发；
- `FIX_SHA=${{ github.sha }}`，避免继续固定a924而误测旧产品。

当前应读取：
- 产品SHA 75b5cc9a 的 `next-performance-recovery`：相关core/race + seed601 Normal10；
- 验证SHA 6c061d30 的 `next-performance-capacity-diagnostic`：包含同一产品树的seed631 compact timeline。

若seed631仍快速撞4096，则继续下一份确定的发送复制/分配审计；不扩大4096、不降低FEC/速率/Game、不扩大buffer、不引入攒包等待。
