# 有事件时间的原生MTU独立结果

## 本轮目标和阶段

P7。产品仍配套SOURCE7eeb，helper固定f965在上传前逐文件校验，未改产品。完成本case并继续下项。

## 修改与原因

只记录真实证据/进度。本case numeric sequence/size/DF逐条对账独立target，单机Stopwatch RTT与send调用有界，跨机UTC只提供相对观察、不声明绝对单程延迟。

## 复用来源

既有MTU助手，无old提取。

## Actions证据

helperf965 predelivery37336237449四job、foundation37336237523、targeted37336237483、GUI37336237453 PASS；docHEADb9cb foundation37336820613/targeted37336820509/GUI37336820421 PASS，未重新测相同Go性能。

原生独立300s，配置actual inner9000/outer1400，程序存活/正常退出/ownedcleanup0；完整性和既定DF边界见 evidence。窗口外层<=1400、checksum/非法TLS头/同seq冲突/外层碎片0，raw删除且capture drop0；仅窗口外观scope。结果：

{"case": "m03-current-300s-seed1414", "status": "PASS_WITH_LATE_RESPONSES", "timing": {"sent": 1954, "received": 1954, "missing": 0, "timeout": 4, "rtt_ms": {"p50": 110.48800000003212, "p95": 139.177200000006, "p99": 204.59259999999801, "max": 3016.277400000007}, "send_call_max_ms": 1.5925999999999996}, "echo_send_call_max_ms": 5.197666, "cpu": {"product": 10.078125, "helper": 0.71875}}

## 问题、排查与风险

稀疏功能RTT不等于10Mbps/3Mbps严格性能p99。健康重复不证明旧1412 >1s根因修好，旧最大UDP缺包和rawp99/Wake残余loss仍开放；跨源码完整样本数不等于全部PASS或最新源码全量资格。

## 下一项原子任务

完成server-ingress-port-filter-actions并定位任何late/missing最早边界，再D04与其余矩阵。每性能Action一条，原始大pcap审计后删除，不盲扩缓存/改FEC/4096/HOL。
