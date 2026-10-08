# E3：严格阶段5205首跑失败为探针echo归属错误，修正分析器并独立重跑（2026-10-09）

## 必须保留的原始 FAIL 与业务事实
唯一刚结束的[Game4 run37816217223](https://github.com/lly8666/wobuzhidao/actions/runs/37816217223)，job113445415351、artifact11567915996、产品SOURCE `2acfede308802e8dfaddfdf7e15c17741a656e15`，helper `d6ba32ed7596b907822a06f782f2dd2303d35a1f`，seed1832，Game4/每向3Mbps真实mixed、300ms单向、300s+3s、profileOFF/auto record0/FEC20:20，实际5%/20%/5% netem。Actions原结论**failure**、原Analyzer **FAIL**：`5205_STAGE_PROBE_LOSS_OVER_1PCT_c2s_pre`和`5205_PHASE_DELIVERY_INVALID_s2c`。预检、正式业务300秒及资源账本步骤都是success，但最终门failure不可改写。

初读新分析器将C2S前75秒probe的`375发/370回`认为5 missing（1.33%）并发现S2C计数矛盾；查明这不是对业务产品有证据的丢包。正式业务源`biz`小探针是kind3发送→`target`收到kind3回kind4→**同一个biz源端**收到kind4并统计probe_received/RTT，反向`target`也有独立小探针源。新分析器却把源`biz.probe_stage_tx`同另一端`target.probe_stage_rx`的**另一条独立反向探针流**比较。因此5 missing是跨方向错配产生的**假阳性**；S2C则因源/目标错配收到更大量另一流的回包而构成负missing直接INVALID。旧总账依然证明同一run两向普通UDP**0/0 missing**、probe原各自源端**1500/1500、1495/1495均0 missing**、returned-only p99约619.855/617.402ms、TCP304/304和HTTP(S)20/20完整。真实netem三段C2S约4.995/20.047/4.988%，S2C约4.985/19.983/4.980%；C2S发送阶段pre/stress/post各有UDP25792/51503/25767发与同数收、>3s为0。S2C阶段因旧summary只存`INVALID`，compact原artifact又未包含原始双端业务`biz/target`计数文件，所以**无法完整重评分阶段结果**，不能把旧Actions FAIL擦成PASS或假称已覆盖所有阶段。

## 原子代码与测试修复，产品未变
这次只修改`tools/check_large_mtu_mixed.py`：保留真实UDP仍是`phase_delivery(src,dst,"udp")`，**独立小探针改为`phase_delivery(src,src,"probe")`**，归属发送端本人的kind4 echo及其本地`probe_received`总账。附加`tools/test_efficiency_5205_phase_delivery.py`单测：探针源端既发kind3又收kind4时归账正确，强制反方向端点假对比必须抛错，防止再把不同行程的两个探针端点错接。原`large_mtu_mixed_business.py`和Go正式产品字节完全不改，阶段基于实际send monotonic戳并继续严格验证per-size/1s/3s延期、每段0.1% UDP与1% probe上限；也不增加新队列或更改RTO500、4096容量/4lane first-arrival/FEC20:20/autoMTU。

本提交将唯一新样本配置设为**Game4、5205三段、seed1833、profileOFF、同一SOURCE**，只发一Action一个300s真实样本（不是A/B或多工作负载），helper SHA来自本次完整commit并先py_compile、waveform+修正后phase tests；需要等实际run/job/artifact取得原Analyzer原判定，再保存每段网络注入、UDP/probe真实送达和>1/>3s计数。没通过就保留新FAIL、不调宽容差，不能用旧正确业务总账重标旧workflow。E3其他保护与匹配runner CPU收益仍未完成，E6/P6/物理NOT_RUN，E7约80秒下行OPEN。

机器证据：[本次分析器假阳性、原始FAIL和新helper修正](../evidence/performance-efficiency-e3-5205-probe-echo-attribution-bug-run37816217223.json)。
