# E3 Stage B：Game2真实两条物理lane也无损通过；独立首条300s 5205波形（2026-10-09）

仅分支`next/performance-efficiency-20261008`，产品仍精确冻结SOURCE `2acfede308802e8dfaddfdf7e15c17741a656e15`，主线、物理网卡和自动协商record/MTU均未动。已有Game4独立[profileOFF 37807156289](https://github.com/lly8666/wobuzhidao/actions/runs/37807156289)和[profileON 37808417385](https://github.com/lly8666/wobuzhidao/actions/runs/37808417385)两条真实lossless混合业务`PASS_SCOPED_ACTIONS`、UDP/probe零缺、TCP/HTTPS hash完整，ON实读4 physical/4 logical lanes、服务端4096外层队列drop32（仍不是零），自动TUN1273。Stage A原Game4 FAIL run37802779025 73586外层读队列溢出与319 UDP缺失，不能删。

## Game2 profileON 正式成功，不只配置成功
独立[Actions37811388963](https://github.com/lly8666/wobuzhidao/actions/runs/37811388963) job113428829753/artifact11565756148，product `2acfede308802e8dfaddfdf7e15c17741a656e15`，helper `4ffa0427c94f0e9c24c8a9064d444d8cb8c790e4`，Game2/seed1829/各向总逻辑3Mbps、0%netem/300ms单向、300s业务+3s drain、20:20 FEC/padding0/record自动0、诊断ON。原始Actions **success**、analyzer `PASS_SCOPED_ACTIONS issues=[]`，分析器新门**实读client/server各2 physical+2 logical**、两条lane refs ID1/2不同generation；没有拿Game4伪装成2lane。C2S/S2C goodput各2.97231648Mbps、96/256/512/1000/1372/4068B UDP两向各size missing0、探针1500/1500与1495/1495全回，returned p99=613.25/613.40ms，TCP各304流hash完整、HTTP/HTTPS20/20和10证书通过，服务端read1446139、共享队列peak1008/4096、**0 outer overflow**、queue最大age30.205ms。profileON product CPU205.42s（runner AMD EPYC7763 4vCPU）是诊断成本；不能和不同runner或Game4的CPU比宣称改进。原历史Game4 FAIL和曾经CI编译FAIL仍原样保存。

## 真正阶段5205不是固定5%
E3保护门另需Normal1与Game4的**真实分阶段5%→20%→5%**。本轮创建一条独立**Normal1**的300秒mixed流样本，loss编码`5205`只用于一项参数，业务时间线严格0–75s **5%**、75–225s **20%**、225–300s **5%**；最后3秒drain继续5%，每向逻辑10Mbps、300ms单向、FEC20:20/pad0、outer1400/auto0、正式client TPROXY→加密outer→server TUN、profile OFF、seed1830。**每个Action仅一个workload/run，不是一个run里AB或多场景matrix。**

- `tools/large_mtu_loss_stage.py`在不损害原fixed0/5/20/30的情况下支持`--fixed-loss 5205`三个netem时期，记录`pre_start/end`、`stress_start/end`、`post_start/end`和源真实qdisc计数，确保真实3s drain。
- `tools/check_large_mtu_mixed.py`独立核实每一阶段实际75/150/75秒（±2s）、每方向丢包率各自与5/20/5差距≤2百分点、不能跨`tc qdisc change`用全局递增计数假装均匀丢包；并要求整体业务输入足额、方向UDP eventually loss不大于0.1%、独立probe timeout不大于1%、TCP hash一致、HTTP/HTTPS证书完整和socket/interface无额外drop。真弱网达不到则原样FAIL。
- `tools/test_efficiency_5205_waveform.py`是完全离线的**确定性校验器单测**，先于任何300s业务在GitHub workflow执行，覆盖three-stage qdisc统计重置、阶段时间损坏fail closed、原fixed loss0流程保持不变。此新助手还**未实际在Actions通过测试**，不提前写绿。
- `tools/prepare_large_mtu_harness.py`在manifest中明确记录`stages_s=[[0,75,5],[75,225,20],[225,300,5]]`和`waveform:5-20-5`，绝不只将5205视为一个固定百分比，也不篡改正式`scripts/strict_weaknet_sample.sh`。

严格限定：现业务generator目前按完整300秒记录送达大小，尚无每段独立UDP送达核账，因此**即便全程业务无误也只得`WAVEFORM_SCOPED_PASS`，不能继承成仓库完整phase-specific 5205硬保护PASS**；若要正式E3/5205放行仍需按阶段交付率/repair deadline与弱网尾部守护。E3尚有Game2 profileOFF、StageB Normal1 lossless、Game4真5205以及3次同CPU资源层OFF成本重复。E2 CPU收益未证实，原E7约80s下行OPEN，E6/P6/物理NOT_RUN。所有原始FAIL保留。机器证据：[Game2正式收据及本5205配置](../evidence/performance-efficiency-e3-game2-on-pass-and-true-normal5205-candidate-20261009.json)。
