# 20260923-151000 outbound raw-send clone 修复的顺序 A/B 验证

## 当前证据

产品提交 `75b5cc9a82458a7a46d382373339193276d6c51e` 仅删除 Linux `RawIPv4Endpoint.WriteSegment` 在同步 `Sendto` 完成后对已 owned marshaled packet 的第二次完整 clone。

验证树 `6c061d3034aae85d05323d64376b1807101ed109` 的 `next-performance-capacity-diagnostic` run **35823231138** / job **107059368064** / artifact **10733329818**（digest `sha256:118750e4311911b65b485c2f7207d48434e938b23a564cc710883eae2aa09c89`）：
- Normal1 lane、FEC20:20、lossless、seed631、每方向10Mbps；
- CAPTURE/CORRECTNESS/ENVIRONMENT/INPUT_VALIDITY/PERFORMANCE 全 PASS；
- C2S/S2C pre/stress/post eventual goodput 均约 10.0Mbps，业务丢失0；
- client/server AF_PACKET drops=0，全部 socket drops=0；
- Abandoned=0、RepairEvicted=0，PeakOutstanding=4035/4036；
- client/server process CPU约56.87/57.09 CPU-s / 120s；
- runner四核busy约76–78%，softirq约5.4–5.6%，steal=0，cgroup throttle=0；
- CPU PSI some avg10峰36.02%；
- client/server TotalAlloc约11.51/10.92GB，NumGC 1558/1335；
- server pipeline handler约28.12s/1,560,454 reads（约18us/read），handoff约24.12s（约15.5us/read）；
- outer/app raw input约C2S 5.0919x、S2C 5.2233x，仍作为FEC/分片等方向账本成本，不解释为重传；本样本 Retransmitted=0。

对照修复前 `65735cb2` seed631 capacity样本：两端约112–115 CPU-s，四核busy约97.7–97.8%，PSI some avg10 51.75%，PeakOutstanding在约2s到4096附近，13–20s出现Abandoned并随后AF_PACKET drops。去掉单个整包clone后同seed直接恢复到全PASS，说明每包CPU/分配成本是触发0.6s RTT下4096/ACK回路塌陷的主要实现因素之一，而不是通过扩大4096才能解决。

独立 seed601 `next-performance-recovery` run **35823219345**：core/race PASS；attempt1 Normal10 为 CAPACITY_LIMITED，artifact **10734456141**，因此仍不能宣称稳定完成。attempt2已重新采集并正在完成validator/上传。

## 本轮动作

修改 `.github/workflows/next-performance-ab-game.yml`：
- BASE_SHA 从旧的755资格计数阶段改为 `a924b7c9853e1d280cb7a7062dc46ccd30039b99`（raw receive scratch已修、但仍有发送后clone）；
- FIX_SHA 改为 `75b5cc9a82458a7a46d382373339193276d6c51e`；
- 保留 AB 与 BA 两个job；每个job都在自己的同runner内顺序执行before/after，排除runner级差异与顺序效应；
- 固定 Normal1 lane、FEC20:20、lossless、seed631、每方向10Mbps；
- after必须五类classification全部PASS，否则job保持失败；
- 同workflow保留 Game4四lane、每方向合计逻辑3Mbps、lossless、seed641，并继续输出完整方向wire ledger。

本轮只改验证入口与文档，不再改产品代码。生命周期、队列所有权、4096、FEC档位、Game副本、MTU、nonce、首次到达交付、同Seq同密文、tls-startup-padding均不变。

## 门槛

若 AB 和 BA 两个顺序中的 after 均五类PASS，且Game4 logical3继续五类PASS，则开始原严格18样本矩阵与目标负载长测。若任一Normal after仍CAPACITY_LIMITED，则继续读取该runner的CPU/AF_PACKET/4096时间线，并做下一单一实现修复；不扩大4096、不降注入/FEC/Game、不扩大buffer、不引入攒包等待。
