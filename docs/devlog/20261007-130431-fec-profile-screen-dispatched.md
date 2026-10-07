# 20261007-130431 36条独立FECscreen已启动

## 本轮目标和阶段

按用户快速批量要求，固定产品d6cb6cee4c241aac8dd2f542a876edc57bf3d7db，仅测试harness 49eba9ba91985bbf771601788fd6d4348d950ef9 / ref qualification/fec-profile-screen-v2-20261007，runtime源码未改。

## 修改与原因

36独立Actions均已dispatch并索引，一run仅一条。off/4/8/10/12/16，每档Normal10M和Game4逻辑3M，分别lossless/5205/5305，均每方向300ms、120s30/60/30、混包、paddingoff。每profile/mode同seed配对无损，计划144阶段p95/p99检查，200/500ms门不变且返回覆盖同时报告。Github当前20运行/16排队，非一run并行负载。

## 复用来源

共用已有registered strict入口与共享脚本，默认formal20不变；screen独立schema不能混入final18，无old/runtime新复用。

## Actions证据

harness49 preflight37574063154 PASS，含新7单位数学/profile/无诊断/mismatch/连续性检查和shell静态语法、原seedednetem/资源门。a9首次新workflow404无性能dispatch的失败保留；本次实际registered dispatch元数据和36 runURL在docs/evidence/fec-profile-screen-d6-49eba9b-plan-20261007.json。产品d6旧14scoped/24RTT/P6和nativeS17证据仍独立。

## 问题、排查与风险

目前0/36性能产物收齐，不能提前说PASS或定稿。原20损失/bytegoodput门作为reference保留；其他profile理论解释需实际partial、业务fragment和Game相关性，不把完整块sourceq或原预算直接当所有business保证。吞吐、p99/timeout、无交付间隔、hosted容量/CPU/队列、正确性各栏分别记录。rawpcap分析后hash回执并删除，不上传原始payload/测试私钥。若正常不可恢复缺包符合能力边界，不为全收齐扩大buffer/repair/FEC或引入HOL；未解释的额外损失/时延/压力则必须定位。

## 下一项原子任务

Collect36independentFECscreenActions exactproduct d6/harness49, validate actualprofile/input/capture/CPU/queues/drops and compare144stagep95/p99 pairs to sameprofile/mode/seed lossless; show probe coverage and no-business intervals. Account actualpartial, businessfragmentation andGame correlation before interpreting theoreticalsource loss; retainoriginal referenceFAIL. If outcomes are consistent with declared capability, latency/noHOL/throughput/resource priorities, freeze profile implementation; rerun only clearly anomalous samples separately, no broad runtime rewrites. Native19remaining/M03/full70/18/1800s not closed.
