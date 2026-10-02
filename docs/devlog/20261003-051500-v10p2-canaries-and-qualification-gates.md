# 20261003-051500 V10.2诊断收口与资格汇总硬门

## 本轮目标和阶段

接续P5性能恢复。起始主线HEAD 48bd3a4c3941f73d06140f9a2ce49127532b91f5；产品固定56eb5413c3cf2e559b82026e8a5783508764e2f4。本轮不改产品、FEC、4096/1024 repair额度、内核buffer或生命周期，先核实未回填canary，续跑lossless，再补正式汇总防误通过检查。

## 修改与原因

- aggregate_strict_weaknet_loss_tolerant_v1.py发现原重复数pass字段未参与最终判定，lossless分类未进入失败门，重复identity会覆盖；因此缺组或坏baseline可能误PASS。修复全六组重复门、所有样本分类门、同源码/配置、完整seed集、重复身份、有效探针以及跨run RTT门。
- 正式汇总可强制GitHub run receipt：独立run ID、workflow_dispatch、attempt1、精确源码、completed/success。新增11项fixture回归由next-performance-analysis-unit在Actions执行；本地不执行测试。
- WEAKNET_QUALIFICATION同步正式18样本完整性要求。原p95/p99阈值与业务损失门槛保持。

## 复用来源

无old复用；直接修复当前分支既有artifact-only分析器。

## Actions证据

产品SOURCE_SHA均为56eb5413c3cf2e559b82026e8a5783508764e2f4，单run单样本，attempt1：

- exact-source正确性/race/Windows/privileged TPROXY：[36039856693](https://github.com/lly8666/wobuzhidao/actions/runs/36039856693) PASS，已记录证据不改写。
- Normal/5205/seed101：[36040934871](https://github.com/lly8666/wobuzhidao/actions/runs/36040934871) 五分类PASS；summary10825874497。stress双向10.00017/10.00030Mbps，业务byte loss0、socket/link drop0、RTT p95 610.756ms，post5 offset2s；CPU72.51/73.08CPU-s/120s，线上IP/输入5.4813/5.6153倍。
- Game4/5305/seed101：[36042176928](https://github.com/lly8666/wobuzhidao/actions/runs/36042176928) 五分类PASS；summary10827615584。stress双向2.99991/3.00007Mbps，socket drop0、RTT p95 604.785ms，post5 offset1s；CPU108.70/101.48CPU-s/120s，线上IP/输入21.3534/21.8905倍。
- 本轮续跑Normal/lossless/seed101：[37064194890](https://github.com/lly8666/wobuzhidao/actions/runs/37064194890) 五分类PASS；job111027824702，summary11251787086、full11252026915。stress双向9.99938/9.99974Mbps；socket drop0、RTT p95 616.501ms；全部阶段双向repair/abandoned/forgiven gaps0。CPU89.03/87.99CPU-s/120s，线上IP/输入5.0904/5.2223倍。
- 分析器fixture回归当前NOT_RUN，推送后由独立Actions确认，不能声称已通过。

## 问题、排查与风险

不同runner CPU时间明显异质，不能把72/89/108 CPU-s直接解释成产品退化；goodput、drop、时延与输入有效性均已逐样本核对。线上放大仍约Normal5倍/Game4约21倍，是后续成本观察项，本轮不重新设计FEC或Game。无损pcap有少量fresh seq重排，repair仍0，不等于HOL或损坏。V10.1 final18的17PASS/1CAPACITY_LIMITED永久保留。V10.2目前仅诊断通过，final18、同版本持续目标负载soak与P6/P7未关闭。

## 下一项原子任务

Actions通过分析器fixture后，以新冻结ref perf-fixed/56eb5413c3cf2e559b82026e8a5783508764e2f4-r2从零分发正式18条独立样本。控制器不生成负载，汇总读取不可变产物和run receipt；任何FAIL/CAPACITY_LIMITED保留并停止资格推进，不能用重跑覆盖。
