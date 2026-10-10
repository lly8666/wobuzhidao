# 当前Actions验收契约

当前ADAPTIVE_NETWORK_PLAN第9节定义N0..N6功能/真实负载/合理性能门，STATUS唯一记录进度。所有构建/unit/race/fuzz/测试在Actions，每性能run一条sample，一个SOURCE/配置/seed/场景、一测量job。历史SIMD/策略实验的串行例外不适用。

## 永久硬门

真实业务完整性/hash、认证/账户地址隔离、同Seq同wire、generation、统一MTU/MSS/checksum、资源有界、无跨业务HOL、有效配置以及精确owned清理。不能因为“安全最后/门不要苛刻”取消这些要求。

定向丢弃旧record/FECsource/大包片时，后续独立完整业务必须在旧包未恢复时交付。单TCP流自己的排序/重传或单UDP自身重组不是跨业务HOL。旧V2向量保留、新V3各算法有新向量；新字段一次协商，业务包无额外mode/cipher字节。

自动控制：足量成熟指标才决策，方向正确、快升慢降、无反馈HOLD、激进建议+1/ceiling、Game不创建auto；切档保留原block/期限和off裸包迟到语义，PN/BlockID/MTU不重置。new-source正确性/race不能继承父门。

Windows少量route数量不随中国CIDR增长；真实direct/proxy两种路径、完整五元组、绑定underlay防loop、DNS双备份/IPv6、重复Stop/Apply取消/部分失败/owned恢复必验。mock配置不当native I/O证据，不支持runner标UNSUPPORTED。

## 合理质量与资源门

受控无损健康容量，合法已发送业务正确完整，throughput约>=95%目标，注入约>=98%；弱网按实际k/r/partial/相关性解释，不统一应用零loss/探针全回/最大UDP全恢复。0.1%为auto模型选档偏好，不是全部弱网硬门。

同时报告p50/p95/p99、missing/timeout、连续零交付、unique goodput、CPU/GiB/CPU-s/RSS、wire放大、实际档位/反馈、真实损伤及queue/drop；returned-only p99不能隐藏失踪。低延迟/HOL以真实独立业务和定向证据判断。吞吐/CPU/p99 screen和绝对噪声带见本轮方案，稳定退化定位，单孤立late不得冒充payload损坏。

SOURCE/helper/config/seed/binary/hash/CPU型号与flags/quota/PSI/steal/实际注入可复核。CAPACITY_LIMITED/INVALID/INCONCLUSIVE/UNSUPPORTED/NOT_RUN/FAIL分开，不将runner上限写性能PASS。profile-on只诊断，不与off比较优化收益。

新contract另建version，历史11配对RTT/Q2/native失败结论不改。收口同SOURCE P6 manifest/hash核验后标ACTIONS_READY_FOR_PHYSICAL，PHYSICAL仍NOT_RUN；原试用部署不自动更新。

父验收完整文本：[归档](history/20261010-adaptive-network-parent/docs/ACCEPTANCE.md)。既有弱网/生命周期专项用作场景和硬门来源，旧范围限制或过严统一零损失条款不覆盖用户本轮合理验收要求；不得回写旧分析结果。
