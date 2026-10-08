# E0：Game4 TCP tick阻塞已定位到同步重传发射，不是扫描（2026-10-08）

目标分支 `next/performance-efficiency-20261008`，本轮父HEAD `a4518b6535d94ff4c79dd44d196e943fa41e47a9`，冻结产品SOURCE `408a5cfb44883d959073c4d41c07c23a767a72db`，helper `a4518b6535d94ff4c79dd44d196e943fa41e47a9`。其[基础Actions 37788321913](https://github.com/lly8666/wobuzhidao/actions/runs/37788321913) Linux/Windows单测、Go race/fuzz、TUN/TPROXY特权和[lifecycle37788321939](https://github.com/lly8666/wobuzhidao/actions/runs/37788321939)均success；独立唯一单样本[Actions37788802499](https://github.com/lly8666/wobuzhidao/actions/runs/37788802499)，job113350384633/artifact11555084630、attempt1，正式client TPROXY、encrypted raw、server real TUN，Game4/各向逻辑3Mbps/mixed TCP+UDP+HTTPS，FEC20:20、pad off、outer1400、record auto0、lossless/300ms one-way、300s+3s真实drain，seed1821、诊断ON。**原始Actions failure、analyzer FAIL**，issues `LOSSLESS_UDP_MISSING_c2s`, `LOSSLESS_PROBE_MISSING_c2s`, `LOSSLESS_PROBE_MISSING_s2c`。Sample和ledger步骤success不等于整体PASS。之前四条Game4真实FAIL全部保留。

## 业务完整性与队列（缺失不伪作延迟0）
C2S 发送2.976156Mbps、交付2.938091Mbps，普通UDP总**2510 missing**，96B1376/256B452/512B195/1000B114/1372B258/4068B115；probe1500发1471回，**29未回**，仅返回者p99=810.900ms、最大2543.087ms。S2C发送2.976144Mbps、交付2.972316Mbps，UDP缺0，但probe1495发1467回，**28未回**，返回者p99=812.276ms、最大2609.939ms。两向TCP hash/length mismatch0、HTTP(S)20/20、10 HTTPS证书/正文校验通过，不能覆盖UDP/probe失效。服务端read3053523、handler2735784，恰好相差ready overflow**317739外层segments**；ready有限4096峰4098、队列年龄最大267.595ms，仍是不合格用户态损失。该样本Linux socket/interface drops=0，不能用未发生socket drop隐去用户态损失。

## 真正热点：18.36秒在发射，扫描仅31.9毫秒
真实server tick总30.4921s/3216次、max219.46ms；service 18.40634s、其TCP段18.40193s。新增逐TCP维护相位共3173快照：
- `tick_tcp_flows=15812`次flow检查，`tick_tcp_due_frames=7231`个到期TCP修复帧，abort1。
- `tick_tcp_flow_snapshot`累计**0.003290s**、最大0.02683ms；
- `tick_tcp_scan`（锁、过期判断、`RetransmitDue`与复制）累计**0.031902s**、max2.759ms、over10ms0；
- **`tick_tcp_emit`实际同步`flow.tunnel.Send`累计18.363001s、单tick emit最大218.678ms、over10ms281**；
- `tick_tcp_abort`累计0.0000617s。合计emit占TCP tick累计wall约**99.79%**。每个到期帧平均只是在此总体wall除以7231≈2.54ms，不是每帧实测p99。这份数值强烈表明“改扫描数据结构”收益微小、真正耗时在同步发送和复制/FEC/内核发射，但不等于证明任一具体syscall单独造成UDP missing，也不能将outer drop317739与inner UDP missing2510简单相等。

AMD EPYC7763 4vCPU，host busy max82.97%、CPU PSI some avg10 max37.77，steal0、quota未知；client/server CPU192.96/167.21=360.17 CPU-s（**诊断ON不能拿去比OFF算收益**），无可靠独占CPU压满证据，原FAIL不能被重标纯capacity。原summary/ledger/manifest/diag SHA256在[机器证据](../evidence/performance-efficiency-e0-game4-tcp-emit-fail-37788802499.json)。

## 选择最窄的下一步，不盲改
现有`platformflow.TCPReliabilityConfig.RTO=500ms`，而业务测试设置300ms单向，预计RTT至少600ms；因此发送中的7231个到期帧有“首次ACK之前先重传”的**结构性风险**。但不得未经证据直接改默认RTO为750ms或1s：真正5%-30%损伤时首修复延后可能严重破坏p99与恢复。也不可将TCP due 帧塞入无限后台队列、扩4096 buffer、牺牲first-arrival/noHOL、削FEC20:20和Game竞速。优先审`TunnelFlow.Send`现有generation/owner/fence能否将**已经到期、同一次tick得到的帧**立即发射成批（不等时间/凑包），或更精确识别并抑制过早重传；如不能保持内外TCP wire/生命周期/重传语义，明确不动产品。任何实际优化要先相关unit/race，随后独立Normal1 lossless/5205及Game4 lossless/5205四条一Action一case验收，不用旧FAIL填绿。

本轮仅新增原始证据和devlog/STATUS，**没有实施E1代码修改、更无CPU优化收益**；E6/P6同源包和物理NOT_RUN，原80秒S2C故障E7 OPEN，主线/物理/固定MTU均未动。
