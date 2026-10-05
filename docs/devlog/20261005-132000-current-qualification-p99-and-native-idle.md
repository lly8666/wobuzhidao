# 当前完整配置/长测收齐，p99失败与原生idle证据归档

## 本轮目标和阶段

分支next/tlslike-dataplane，开始HEAD919d58b，产品与部署仍固定SOURCE660b370815603c50d38b92229abdec13ca77e1b1。用户明确p99与性能不可退化，继续验收、按证据修复，不能把CI绿当最终合格。

## 修改与原因

更新唯一STATUS和入口文档，归档70配置、18严格独立run、两1800s及S18原生证据。补前提交漏改STATUS造成的仓库契约错误。未改任何产品算法/4096/FEC/buffer、吞吐或尾延迟门槛。

## 复用来源

无新产品复用；只读同源Actions原summary及原生诊断。大pcap不下载，尾延迟只范围读取约1.04MB压缩小诊断；原生bounded raw capture已分析删除。

## Actions证据

精确SOURCE660：70/70配置PASS；18/18单样本五分类PASS、socketdrop0，revision2配对FAIL。Game5305 seed1382 run37255274295 stress p99=1314.347294ms vs lossless602.290026ms，差712.057268ms >500ms。原逐秒81/82探针1850.733281/1314.347294ms，探针全回。保留失败、不换seed覆盖；后续独立重现只能增加证据。

Normal37254546236/Game37254548514均1800s PASS，最差阶段吞吐9.9989909/2.999872Mbps；最高阶段p99=621.12872/604.474247ms；socket/internal queue原门通过。CPU809.82+817.18 /1080.36+1016.13 CPU-s，各除1800为进程平均核心占用。不同VM不能据此声称优化百分比。完整回执及artifact摘要压缩归档见evidence/qualification-660b370-20261005.json。

HEAD919 predelivery37256467593和GUI37256467603 PASS；foundation37256467620与targeted37256467646 FAIL的原receipt唯一错误Each change needs STATUS update，后续Go jobs SKIPPED。是本人的日志/STATUS流程遗漏，补本提交契约后仍要让Actions真实验证，不能虚报Go已PASS。

## 问题、排查与风险

尾延迟原窗口host busy约40–76%，GC累计暂停仅毫秒级增加，server handler max约7.64ms/queue max约41ms、四lane未换代；当前证据不支持直接认定持续CPU打满，也不足断言FakeTCP/FEC等待。下一需要probe目标端到达/回发时间及路径时序，定位上行还是下行；低负载平均指标不能洗掉p99失败。

S18 seed1391完整300s，双方约66s和182s进入Dormant且physical/active=0，约122/242s客户端恢复gen2/3，server约121/241s；lease始终同一。用户允许idle关闭较大余量，验双方在每次新业务前持续释放；不用精确30秒门。旧helper无start UTC，采用中点load elapsed与diagnostic UTC近似对齐≤2s，不伪装精确启动时间。双向活跃归一吞吐9.98866/9.95757M，byte loss0.11289%/0.42341%保留；此夹具静默期间无probe/DNS，因此p99 NOT_EVALUATED，功能PASS不等于质量无损。正常stop0、owned状态0。

M03两份最大UDP回程缺包仍FAIL，已新增MissingSequences/target序号与Windows IP前后计数helper待新同源诊断，不做第三次无新证据盲测。总体P7仍PARTIAL，剩余34独立case未跑，混源码历史不能继承。

## 下一项原子任务

验本提交helpers契约/core/race/predelivery/GUI；继续失败尾延迟的有界probe分段观测及同seed独立Action，保持现有500ms门与吞吐。合格helper后S19/Game4和S20纯下行各300s，最大UDP精确序号诊断，再逐项DNS/IP/FEC配置。每性能Action只一条；无HOL、延迟与吞吐不牺牲。
