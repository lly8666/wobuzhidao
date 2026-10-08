# E0 TCP+短HTTP(S) 300秒原始FAIL，启动独立Normal mixed诊断（2026-10-08）

目标精确分支 `next/performance-efficiency-20261008`，原产品SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733` 不变，当前helper SHA `c1fb6b7b024591b2e5dd6ffd3ec0d00004a13819`。独立 [Actions run 37762364098](https://github.com/lly8666/wobuzhidao/actions/runs/37762364098)，job 113261625674、artifact 11543356424、attempt1、一次300秒性能样本；原分析器 `FAIL`，原workflow failure；**绝不更改为PASS**。未操作物理机。原始manifest/biz-http/target-http、netem、resource、pcap hash删除收据仍在Action artifact；细表见[evidence](../evidence/performance-efficiency-e0-run37762364098-tcp-fail.json)。

## 本轮真实被测项与通过的窄覆盖
Normal1，双向各10Mbps逻辑总预算、TCP 4持续流+300短framed连接+10 HTTP与10个CA/主机名验证的**真实HTTPS**，seed1802，0%netem，300ms单向，外层1400，正式record cap0，真实server TUN MTU1273，3秒drain、两端诊断JSONL均不存在。20个短HTTP(S)全部返回、10个HTTPS certificate verified且完整响应体和SHA256校验通过，server http10/https10/错误0；HTTP幸存p99=1836.959ms，missing0；单纯此项有证据但不替代长流目标。

## 未达资格、不可掩盖的失败
- C2S：发送7.7165Mbps、应用交付7.7127Mbps；四长流+300短连接的全部304条有接收账本/sha相等，但低于10Mbps输入目标；`sendall` >10ms backpressure 359次，单次最长5443.6ms，四长流累计并发发送阻塞1187.3s。探针1500/1500，p99=627.837ms，missing0，active10ms零窗最长1010ms（诊断不等于跨业务HOL已确认）。
- S2C：发送7.7463Mbps、应用交付7.7145Mbps；304条发侧连接中303条有收侧账本，flow2源已记录71,581,696B但未见最终接收摘要，故`TCP_HASH_MISMATCH_s2c`原义为**无法核对完整性/长度**，不能直接说数据被损坏；`sendall` >10ms 420次，max5420.3ms，阻塞累计1186.2s。探针1495/1495、p99=627.126ms，missing0；最长活跃零业务约1020ms。原始三项问题：`INSUFFICIENT_INJECTION_c2s`/`INSUFFICIENT_INJECTION_s2c`/`TCP_HASH_MISMATCH_s2c`。
- Linux runner AMD EPYC7763、4 vCPU，client CPU96.75s、server95.92s、RSS峰38.54/38.64MiB，steal0、CPU PSI avg10最高31.69、host busy最多21.68%、raw socket/interface drop0；**不满足证明容量不足的配额或本机drop证据**，不得以`CAPACITY_LIMITED`替代FAIL，也不得将较低CPU（因较低交付）称效率改善。
- 静态四流有序TCP的等待不自动等同外层跨业务HOL；HTTP与小UDP仍前进，但长TCP流backpressure、尾部接收摘要缺口已达hard gate、需继续查真实应用写入/关闭时间、内核send queue、生成器timing、TUN/回收及ACK路径。没有确认80秒故障的因果，不提前做E7。

## 下一独立样本和退出条件
已在同提交将`.github/efficiency-e0-sample.json`变为Normal **mixed 5Mbps TCP +5Mbps UDP，总10Mbps/方向**，lossless 300s/3s drain、seed1803、HTTPS短业务仍进入同总配额，profile-off。下个workflow run必须只跑这一条，不做同run A/B或旧TCP重放；结果按原始source/helper/run/job/artifact和runner PSI、queue/drop、probe返回/未回、hash照实记录。它仅用于定位混合业务可用性，**TCP原FAIL不会因另一个case通过而消失**。后续独立Game4逻辑3Mbps和jumbo边界、诊断alloc/batch/repair账本未执行，E1-E7未开始，P6/物理未跑，80秒下行仍OPEN留E7。
