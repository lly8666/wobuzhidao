# 20260922-202000 strict generator 固定120秒窗口修复

## 失效证据与口径

首个strict workflow：
https://github.com/lly8666/wobuzhidao/actions/runs/35728934986

该run正确展开18个独立job，但多个样本在理论的120s injection + 10s drain之后仍长期停留在负载步骤。审计 `realpath_udp_duplex.py::run_sender` 确认存在harness缺陷：

- target时间由累计计划bytes计算；
- 若实际runner已经落后，旧代码仍逐slot补发，直到把所有计划bytes补齐；
- target peer未ready时旧代码只sleep+continue，不推进cumulative/seq，可无限停在同一slot；
- 因此容量不足时“120s”会被延长成更长的catch-up发送，drain/最终交付可掩盖运行时容量不足。

这直接违反 WEAKNET_QUALIFICATION 的固定120秒有效注入、p99<=10ms、skipped<=0.01%以及“不能用迟到数据掩盖堵塞”。所以run 35728934986从harness定义上作废，不等待它变绿/变红再决定资格。

## 最小修复

不改产品代码，只改发生器：

- 每个slot先计算target_ns；
- 若检查时或sleep唤醒后已比target晚 >10ms：该slot记 `skipped_slots/skipped_bytes`，推进计划cumulative/seq，不发送、不追赶；
- peer未ready：同时记send_failure和skipped并推进slot；
- 正常slot仍按原精确字节pacing发送；
- 新增per-second skipped slots/bytes，供阶段99%..101%和<=0.01%门独立复算。

这样无论runner多慢，注入时间窗都仍在120秒截止；容量不足只会留下INPUT_VALIDITY FAIL/CAPACITY_LIMITED证据，不会把测试时间拉长来“补成绩”。

## CI节奏

本修复提交同时把 next-strict-weaknet 的push触发临时移除，仅保留workflow_dispatch。原因是修改发生器本身原会立刻再启动18个样本；按用户要求，应先跑校准、基础和受影响fast回归。

fast验证后会用下一原子重新启用push trigger并bump `docs/qualification/STRICT_WEAKNET_TRIGGER`，只启动一轮新的18样本。

## 旧run处置

35728934986 的日志/artifact若最终产生，全部保留作为runner/harness容量诊断，但状态固定为 HARNESS_INVALID；不得与新run择优拼接，不得计入3 seed资格。
