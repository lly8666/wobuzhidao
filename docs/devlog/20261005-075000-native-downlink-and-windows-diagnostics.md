# 20261005-075000 五分钟下行损失与Windows诊断

## 本轮目标和阶段

用户要求连续修复并验收P7。父HEAD/SOURCE 3e3e09496ce78dbbefca68dc6e410bbc7de62404，同源配套Actions包已部署Windows原生Npcap/Wintun与ARM服务端。继续定位D01，不凭旧成绩关闭新SOURCE。

## Actions与原生证据

固定3e foundation37243533369、targeted37243533367、runtimeowner37243533255、lifecycle37243533272、predelivery37243533260、defaultnetwork37243533316、GUI37243533300 SUCCESS；独立Normal5205 run37243572929、Game5205 run37243575135、stateful lossless37243577151五项classification均PASS；P6 run37243579697三目标及aggregate成功，manifest/hash和两端version精确一致。链接均为https://github.com/lly8666/wobuzhidao/actions/runs/<run_id>。latest full70/strict18/1800s仍NOT_RUN，c5 full36不能继承为3e full36。

D01 seed1302完整300s，两方向均实际发9.99998Mbps；C2S goodput9.99962Mbps、缺24包；S2C9.10985Mbps、缺44639包（7.704% packet/8.901% bytes）。probe2971发/2763回，208超时，成功probe p95约479.55ms。旧90s双向完全停顿没有再现，generation1保持，但只是一份重复，D01质量仍FAIL。bad/duplicate0，client正常stop/exit0，owned NRPT/firewall/state清理通过。Linuxserver ready queue overflow0，FEC最终expired_missing_sources24；单端诊断不足以解释下行。证据evidence/physical-window-promotion-3e3e094-20261005.json及压缩小回执，原始pcap分析后删除。

seed1301在正式负载前因助手TEMP目录缺失失败，记SETUP_FAIL，不算完成300s。框架已先创建portable data/tmp。当前同SOURCE独立S16 seed1303运行，rotate60s/60s，结果未出。

## 修改与原因

Windows补默认关闭diagnostic-jsonl/diagnostic-interval，复用qualificationdiag输出owner/lifecycle/FEC/transport与Go资源。Npcap仅诊断on记录收发/read-gap和本产品handle的pcap_stats；driver stats由唯一pcap reader每秒采一次，外部JSON线程只读atomic；generation map Close后移除，不无限保留旧incarnation或读取已释放handle。Windows ABI明确6个uint32；缺stats导出记unsupported，不伪造0。无buffer/window/FEC/repair/交付顺序变化。

统一参数与GUI全集同步；GUI诊断路径限制portable目录，补实际配置/路径逃逸检查。默认off无文件；文件沿用Linux诊断无自动轮转，实机助手设置16MiB上限。on资源与off基准分列。增加并发snapshot race验证；hosted不装Npcap冒充driver原生验收。

## 复用来源、风险与下一项

无old复用，复用当前qualificationdiag/TunnelClient.DiagnosticSnapshot/Npcap call gate。产品编译/unit/race/性能只在Actions，开发机仅编辑/格式化/参数生成/证据整理。

新诊断补丁NOT_TESTED，先Actions core/race/GUI与独立Normal/Game5205，再精确SHA同源P6包、D01双端诊断。先收S16、保留失败；只修最早实测异常，不扩大buffer/FEC/4096、不吞stale。之后DNS互备等43case各条独立顺序推进。
