# 20261003-112000 FEC带宽修复正式18样本收口

## 本轮目标和阶段
用户要求解决带宽过大；产品固定f240d5177c6a8aaa02de570079926b2600c480fd，文档HEAD变化不算新产品。P5保持IN_PROGRESS，带宽子任务ACTIONS_PASS。

## 修改与原因
本提交只回填实测状态和evidence，产品Go源码不再改。20:20固定最多3长度组消除了组内max-length parity膨胀，source即时发送/独立8ms及v1/owned复制保留；低档位single-group逐字节输出、off/Game/4096/buffer保持。此前单独修复server retiring锁保护。

## 复用来源
无old提取；复用当前FastBlockEncoder/codec及原严格真实路径框架。

## Actions证据
SOURCE_SHA f240d5177c6a8aaa02de570079926b2600c480fd。foundation37091267143（Linux/Windows unit/build、Linux全量race/fuzz、fallback/privileged TPROXY/TUN）、targeted37091267211、lifecycle37091267130、startup-padding37091267158均PASS。生命周期完整回归37091267131：36 samples+aggregate=37/37 jobs PASS。旧6395185 foundation37091096393的真retiring race失败保留。
独立无损canary37091442668 PASS：C2S/S2C线上3.19182/3.20778倍，parity196.27/198.64MB，源194.98/195.00MB；每向输入150MB，10Mbps满速，loss/drop/repair/abandoned0，RTTstress p95 612.46ms。
正式coordinator https://github.com/lly8666/wobuzhidao/actions/runs/37091747064 ，main控制4d122f93d3cbe110cdc403d2ec09f7e5d379b2a2，分析器6dafa657af9f577f8cc21256276bfb44d8ee3fd1；冻结ref perf-fixed/f240d5177c6a8aaa02de570079926b2600c480fd-r2。18条独立workflow_dispatch attempt1，全部五分类PASS；aggregate revision2 PASS/errors=[]，全六组各3seed。每性能run只测一条，无重跑挑选。
aggregate artifact11263225179，digest sha256:8c7dc1a8448ff0b2cb387bab1a87e6fa8d0ba2313acc82678cf8f187d0dce605。完整18run与摘要artifact登记docs/evidence/fec20-size-class-bandwidth.json。

| 模式/损伤 | n | 新线上IP/业务倍数 | 原倍数 | 平均省带宽 | stress最低goodput | 最大业务byte loss |
|---|---:|---:|---:|---:|---:|---:|
| normal/lossless | 3 | 3.192–3.208 | 5.092–5.224 | 37.96% | 9.99962Mbps | 0.00000% |
| normal/5205 | 3 | 3.488–3.506 | 5.474–5.615 | 36.94% | 9.99878Mbps | 0.00137% |
| normal/5305 | 3 | 3.368–3.384 | 5.300–5.437 | 37.11% | 9.86443Mbps | 1.35023% |
| game/lossless | 3 | 13.280–13.347 | 20.627–21.163 | 36.28% | 2.99987Mbps | 0.00000% |
| game/5205 | 3 | 14.144–14.219 | 21.676–22.227 | 35.37% | 2.98609Mbps | 0.45940% |
| game/5305 | 3 | 13.912–13.976 | 21.352–21.889 | 35.51% | 2.99958Mbps | 0.01629% |

全部socket drops0，全部无损repair/abandoned/业务loss0。全pre/stress/post逐seed对独立lossless基线配对p95增量最大27.490269ms、p99最大30.301353ms；原200/500ms门未放宽。退回post5后连续3秒恢复窗口起点offset1–2s（不是窗口完成时间）。
CPU跨runner：Normal约52.18–87.48CPU-s/client/120s、Game约58.38–109.86；不能按独立异质host声称固定CPU提升百分比。旧样本Normal49.97–100.06、Game62.07–112.84，主要CPU范围重叠；本轮是已证实字节节省和通过资格，不是微基准。

## 问题、排查与风险
Normal5305 seed202 S2C stress byte loss1.3502276%、goodput9.86443Mbps，比旧该组最大0.22048%更差；其余五方向0.174–0.345%。返回post5三份两方向全部loss0。小block改变纠删恢复概率是风险，具体最差样本损失集中链未定位，不凭PASS宣布质量完全相同。符合用户30%链路容许残留业务损失且优先低延迟/吞吐/no-HOL的现行门，但必须告知该取舍，长测继续关注。
Game4/5205最大0.4594%（旧0.4797%）；5305最大0.0163%（旧0.4811%）。payload/transport完整性门PASS、no-HOL/期限/ownership单测及race通过。仍有FEC20:20固有校验、Game4四份复制和header成本，不能承诺接近1倍。
仅120s资格；目标>=30min稳定性、15/20M、当前产品P6打包、P7物理资格NOT_RUN；不继承旧P6资格。

## 下一项原子任务
恢复既定有界目标负载长测框架开发，但产品基线改为f240d5177c6a8aaa02de570079926b2600c480fd；Normal1每向10M/Game4逻辑3M分别独立1800s，跟踪新Normal5305最差损失聚集时间链、outer成本、有限队列/RSS/CPU及post恢复。不无证据再改FEC/repair，不放宽门、不加buffer，P6/P7后续。
