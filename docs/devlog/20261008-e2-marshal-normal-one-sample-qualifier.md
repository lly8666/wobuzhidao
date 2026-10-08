# E2第一次真实业务样本：完整测试前置成功，冻结单缓冲MarshalPacket产品SOURCE（2026-10-08）

## 新产品候选已通过基础保护
本提交仅针对`next/performance-efficiency-20261008`，产品SOURCE冻结为E2候选精确commit `673a8ab0b2295d495e67cd7d4a42e23a70d38a7a`，不是旧instrumentation产品或`main`。此前`MarshalPacket`需一段Frame中间所有权内存并复制进IPv4帧，当前新实现直接把经同一`frameHeaderFields`验证的Frame写入单个最终所有权IPv4 buffer，`MarshalFrame` API与线协议未变；新增 `TestMarshalPacketSingleOwnedAllocationExactLegacyWire`、`TestMarshalPacketValidationPreserved`、`TestMarshalPacketOneAllocation`。该提交精确[foundation 37792796931](https://github.com/lly8666/wobuzhidao/actions/runs/37792796931) 全部Linux/Windows单位、race/fuzz、TUN/TPROXY/P2 privilege、契约SUCCESS；[lifecycle 37792797183](https://github.com/lly8666/wobuzhidao/actions/runs/37792797183) core SUCCESS。此结果证明基本兼容和测试，而**不是**真实吞吐、全量无损或CPU提升保证。

## 唯一新性能样本
在此新helper提交中，`.github/workflows/next-efficiency-e0-single.yml`的`PRODUCT_SOURCE`从仪表旧SHA更新到`673a8ab0b2295d495e67cd7d4a42e23a70d38a7a`，唯一配置`.github/efficiency-e0-sample.json`设为Normal1、mixed普通UDP包长+TCP四长流/300短流和HTTP/HTTPS真实验证、各向逻辑10Mbps总预算、0%外层loss、300ms单向、300s业务+3s真实drain、FEC20:20、padding0、outer1400/auto record cap0、profile**OFF**，seed1822、正式client TPROXY/encrypted raw/server共享TUN。新的helper完整SHA取此commit `GITHUB_SHA`，1 Action只含一个case无matrix与同runAB。要保留C2S/S2C offered/delivered/UDP+probe缺失、TCP hash与HTTPS body、实际TUN MTU、runner host/PSI/steal/cgroup quota、raw PPS/calls/batch/alloc/GC和normalized cost ledger，原Analyzer FAIL原样保留。不能用旧profile ON与新OFF直接比CPU，也不能拿宿主变化当收益。

## 退出门
Normal1 lossless仅第一条保护入口，不满足E2全部四门：仍需Normal1 5205、Game4 lossless、Game4 5205 **各自独立Actions**并检查真实业务，旧Game4[run37788802499](https://github.com/lly8666/wobuzhidao/actions/runs/37788802499)原始FAIL：C2S UDP2510缺失/双probe缺29与28/ready溢出317739/TCP due7231->emit18.363s，不能拿本Normal成功覆盖。CPU或alloc收益至少3条父/候选可比runner、同条件OFF分层，非同runAB，若无可比层写INCONCLUSIVE。先读本条原始run/job/artifact，不准预写PASS/E2完成。此前 TCP-only、8937/65507不支持FAIL及原80s下行 E7 OPEN、E6/P6/物理 NOT_RUN 都保留。所用证据：[本轮JSON](../evidence/performance-efficiency-e2-marshal-normal-lossless-candidate-20261008.json)。
