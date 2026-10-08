# E3 首次真正阶段5205注入与正常单lane混合业务受限通过；转独立Game4（2026-10-09）

目标分支`next/performance-efficiency-20261008`，产品SOURCE不变`2acfede308802e8dfaddfdf7e15c17741a656e15`，上一helper精确`09f5abf5bb1781d4a346be0f2e859bc14e6c3588`。独立[Actions37812674106](https://github.com/lly8666/wobuzhidao/actions/runs/37812674106) job113433243998/artifact11565633299 代码preflight三条Python waveform测试OK、实际脚本构建和正式300s负载均成功，原Analyzer **PASS_SCOPED_ACTIONS issues=[]**，没有同run AB或矩阵。Normal1 mixed10Mbps/向、profileOFF、seed1830，真实TPROXY→加密outer raw→TUN、outer1400/自动record limit0/实际服务器TUN1273、FEC20:20/padding0、单向300ms、3s真正damaged drain。

真实双向netem不是伪固定5%：`stage-events.jsonl` `pre_start`→`pre_end`各向分别**5.015% / 5.022%**（0–75秒、约804k attempted）；`stress_start`→`stress_end`各向**19.980% / 19.990%**（75–225秒，约1.48m attempted）；`post_start`→`post_end`各向**4.981% / 5.024%**（225–300秒，约805k attempted），每段日志具真实qdisc snap、独立相位计数及持续时间硬门。总体外层丢弃C2S375919/S2C376452。前后实际3秒drain成功，业务300秒分母不扩。

真实业务不应隐瞒残余损失：C2S发9.97614784Mbps交付**9.97126997**，UDP **23 missing**（256B8/512B3/1372B5/4068B7），探针1500/1500零缺，returned p99**652.09ms**；S2C发9.97613504、交付**9.97125173Mbps**，UDP **34 missing**（96B1/256B10/1372B21/4068B2），探针1495发1493回 **2 missing**、仅回者p99**662.69ms**；TCP双向304条sha/length完整、HTTP(S)20/20/HTTPS证书body10成功。CPU client189.13/server187.74合计**376.87 CPU-s**、AMD EPYC7763 4vCPU/quotanull、host busy peak48.97%、PSI some34.82；不同损伤工作量不能作为CPU节省证据。Go总alloc在真正profileOFF未采集。

**这只是阶段网络注入+全程业务的`SCOPED`成功，不是严格整套5205业务分段PASS：** 现driver只完整记录整个300秒的UDP unique收发，尚未按各自真实发送时刻追踪每个丢失数据报属于75/150/75哪个阶段、对应deadline/repair恢复时限；因此不能把聚合的≤0.1% UDP missing和≤1% probe丢包升级成仓库真正phase-specific 5205验收，更不能谎称“无损”。E3正式退出仍BLOCKED。

这次只更改唯一`.github/efficiency-e0-sample.json`到**Game4 profileOFF、loss5205、seed1831、真实mixed各向逻辑3Mbps、300ms单向、300s分阶段5→20→5 +3s drain、FEC20:20、outer1400/自动record cap0、source仍`2acfede308802e8dfaddfdf7e15c17741a656e15`**；触发下一条独立Actions（一run一case、无同runAB）。Game4此前分别在off [37807156289](https://github.com/lly8666/wobuzhidao/actions/runs/37807156289)和on [37808417385](https://github.com/lly8666/wobuzhidao/actions/runs/37808417385) 0loss PASS，ON实有4 physical lanes；Game2 ON [37811388963](https://github.com/lly8666/wobuzhidao/actions/runs/37811388963) real2 physical PASS。它们不自动转化为Game4 5205证明。若下一wave有任何FAIL，保留原始失败和每阶段损伤实证，绝不美化。

剩余Game2 OFF、Stage B Normal lossless、Game4完整逐阶段交付/repair ledger、CPU/GiB至少3组可比层父/候选重复、E6/P6/physical NOT_RUN及原E7 80秒下行OPEN全部保留。source/main/MTU未变。详细机器证据：[5205三阶段真实收据](../evidence/performance-efficiency-e3-normal5205-waveform-scoped-pass-run37812674106.json)。
