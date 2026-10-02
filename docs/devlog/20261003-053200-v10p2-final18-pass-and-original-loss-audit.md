# 20261003-053200 V10.2正式18样本通过与原始损失日志审计

## 本轮目标和阶段

收口P5 V10.2的120秒目标速率矩阵，保留同版本长测/P6/P7未完成状态。产品SOURCE_SHA56eb5413c3cf2e559b82026e8a5783508764e2f4；文档HEAD变化不算新产品优化。

## 修改与原因

- 回填18条独立正式样本与完整artifact-only资格，新增机器可读evidence供全新agent直接核对。STATUS/ACCEPTANCE/DEVELOPMENT_PLAN均指向同一实测状态，不另建交接主线。
- 补两份原始Game seed303完整产物的只读Actions审计，区分本机队列溢出与网络/有限恢复后的残留业务损失。
- 长测设计明确现120秒工具的逐包set/list和四点pcap不能直接放大15倍；先做有界统计/分块捕获框架，再做Normal/Game分开的1800秒正式样本。该长测框架尚未实现，不提前打包晋级。

## 复用来源

只修复现有新项目分析/Actions工具，不涉及old、FEC/recovery策略或产品Go源码。

## Actions证据

### 正式矩阵

主控制器 [37065816473](https://github.com/lly8666/wobuzhidao/actions/runs/37065816473)，分发job111033217612、只读汇总job111033401800均PASS。main控制SHA da82a86860842c659fea2b0ce7b145c7d9eaec9c；分析器固定6dafa657af9f577f8cc21256276bfb44d8ee3fd1；产品固定56eb5413c3cf2e559b82026e8a5783508764e2f4；冻结ref为perf-fixed/56eb5413c3cf2e559b82026e8a5783508764e2f4-r2。18份均workflow_dispatch、attempt1、独立run，没有诊断复用或失败重跑。

| 模式/损伤 | n | stress双方向最差goodput | 最大业务byte loss | stress探针RTT p95范围 |
|---|---:|---:|---:|---:|
| Normal/lossless | 3 | 9.99953Mbps | 0% | 607.73–612.77ms |
| Normal/5205 | 3 | 9.99945Mbps | 0% | 614.94–616.53ms |
| Normal/5305 | 3 | 9.97760Mbps | 0.22048% | 612.84–617.73ms |
| Game4/lossless | 3 | 3.00003Mbps | 0% | 602.93–609.94ms |
| Game4/5205 | 3 | 2.98564Mbps | 0.47966% | 602.28–610.85ms |
| Game4/5305 | 3 | 2.98568Mbps | 0.48109% | 604.67–606.90ms |

五分类全部18/18 PASS，全部socket drop0。全部无损阶段repair/abandoned0。逐seed模式跨独立lossless baseline、全部pre/stress/post的p95增量最大18.093871ms、p99增量最大36.069584ms，原200/500ms门未放宽。十二份有损样本post5连续3秒恢复窗口的起点offset1–2秒，不能把起点当完整窗口确认时间。aggregate_revision2 PASS、errors=[]，全六组各3种seed。

aggregate artifact11252477679 / digest sha256:01f4c0e467fb7e28f92ead0d15019c603be1e2429d30eeed35ce539ee59c816b，原完整产物由各run保留。具体18run与指标在docs/evidence/v10p2-final18.json。Normal无损client CPU49.97–96.35CPU-s/120s显示宿主异质性；不能声称固定提升百分比。线上IP/input仍Normal5.09–5.61倍、Game4约20.63–22.23倍，作为后续成本观察。

### 原始损失审计

[37067109489](https://github.com/lly8666/wobuzhidao/actions/runs/37067109489) / job111037605427 PASS，只读完整原archives，不启动负载。audit artifact11252728321 / digest sha256:4cc1b60d95f01b26f8f765797e3ff3aa6eb65fa1dae2df118ccbcc62f7057607；详见docs/evidence/v10p2-game-loss-audit.json。

- Game/5205/303 run37065865089：213个S2C缺包集中在发送第84秒；client UDP peak7/1024、4240B，server peak9/1024、4560B；overflow、forward/gate/send/stale/close errors全部0。
- Game/5305/303 run37065869715：215个C2S缺包集中在第78秒；client UDP peak21/1024、10640B，server peak11/1024、5824B；同样错误全部0。
- 两端最终queue/inflight与bytes均0。因此这两份少量损失没有证据指向V10.2 UDP ingress/send backlog或socket overflow。损失集中单秒，不能仅凭低比例就断言是随机netem没有被FEC恢复；FEC/LINK/Game上游具体时间链仍未完整定位。后续长测继续观察，无充分证据不改算法。

### 本轮代码/验收工具回归

6dafa65的aggregate fixtures11项+wall accounting2项在37065380317 PASS。6dafa65遗漏STATUS导致的contract失败保留，47e01b5 foundation37065495783与targeted37065495782均PASS。2e7b67f的新CI failure propagation/policy在foundation37066175304的contract、Linux/Windows unit/build、Linux race/fuzz、fallback、privileged TPROXY/shared-TUN均PASS；targeted37066175340与blackhole analyzer unit37066175326 PASS。无本地编译/测试。

## 问题、排查与风险

120秒资格完成，不外推到15/20Mbps、任意配置、30分钟稳定性或Windows/Npcap真实网卡。V10.1 17PASS/1CAPACITY_LIMITED、旧版本失败及当前小比例loss全部保留。P5整体仍IN_PROGRESS，目标速率长测NOT_RUN、P6新打包NOT_RUN、P7 NOT_RUN。

## 下一项原子任务

按DEVELOPMENT_PLAN最新有界长测设计实现测试工具，不改产品；先Actions collector边界unit与单条缩短版DIAGNOSTIC_ONLY，再Normal10M/Game4逻辑3M分别独立1800秒资格。新发现必须保留原始输入、drop、队列与时序，不能以“runner差”或重跑择优代替定位。
