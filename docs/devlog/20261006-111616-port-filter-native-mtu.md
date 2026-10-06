# 过滤入口候选原生最大UDP结果

## 本轮目标和阶段

P7，固定配套SOURCE/helper9211b24，不更改产品或既定原生助手的负载/timeout/pacing。300s Normal1、FEC20:20、outer1400、Windows inner9000。

## 修改与原因

只留原始计数/时序/版本/hash与当前进度。额外IP统计在load前后读取，非逐包工作；稀疏功能RTT不当10M/3M正式性能资格。

## 复用来源

现有 qualified MTU/计时助手；无old提取。

## Actions证据

固定9211b24 core/race/37生命周期/五独立性能18RTT对/P6及kernel/native门已通过，见server-port-filter-9211b24-20261006。当前原生结果：
{"status": "FAIL", "total": {"sent": 2028, "received": 2027, "missing": 1, "timeout": 2, "rtt_ms": {"p50": 98.3071999999936, "p95": 125.66950000000077, "p99": 175.74149999998667, "max": 1308.1963999999857}, "send_call_max_ms": 1.6306999999999987}, "maximum_udp": {"sent": 169, "received": 168, "missing": 1, "timeout": 2, "rtt_ms": {"p50": 123.56269999997949, "p95": 183.09219999999726, "p99": 848.6079999999987, "max": 1308.1963999999857}, "send_call_max_ms": 0.2800000000036107}, "raw_io": {"enabled": true, "receive_port": 443, "kernel_port_filter": true, "receive_calls": 36551, "receive_messages": 67854, "receive_multi": 13416, "receive_fallbacks": 0, "send_calls": 34297, "send_messages": 64904, "send_multi": 14028, "send_fallbacks": 0}, "socket_drop": {"observations": 380, "max": 0}, "arm_ip_delta": {"FragCreates": 11661, "FragOKs": 676, "FragFails": 0, "ReasmReqds": 1690, "ReasmOKs": 338, "ReasmFails": 0}, "expected_echo_fragments": 11613, "cpu": {"product": 13.75, "helper": 0.625}, "server_cpu_delta_380s": 6.31}

全部窗口<=1400、checksum/外层fragment/同seq冲突/非法TLS头0、capture drop0，raw已删除。正常退出owned network/NRPT/firewall0；合法/拒绝DF与65507/65508边界保持原门，完整清单见evidence。

## 问题、排查与风险

只证明本sample；不把单次健康等同旧2..3秒长尾根因全部关闭。kernel_port_filter443实际true；whole-host分片/重组计数与fixture数量对账，server TUN1400/Windows9000仍不一致且echo产生更多IP碎片，但没有把它直接当延迟原因或改MTU。IP.OutDiscards等全机指标不能自动算作业务drop。CPU不同WAN时刻不推固定提升百分比。历史失败和最新源码全70/strict18/1800s/P7整体未关闭。

## 下一项原子任务

继续M03-reverse-fragment-diagnostic-and-DNS-helper-qualification，再D04及剩余矩阵，每性能Action一条，不为全收齐扩大库存或恢复HOL。
