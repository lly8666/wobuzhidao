# 20261008-165834 优化方案分支集成Actions核验

## 本轮目标和阶段

next/performance-efficiency-20261008，起始bf11fbfbe64d518e7ba189d51bfb4512df4df733。只写集成资格回执，不新增产品优化或性能负载。E0测量未开始。

## 修改与原因

STATUS与方案evidence更新精确SOURCE的三项Actions结论；latest_log指向本日志。文档HEAD与被执行SOURCE分开，不因docs-only提交继承普通性能资格。源代码仍与MTU父c480cce一致；main82c3c61的最新b4物理失败/历史原路径保留。

## 复用来源

无新增old源码复用。首个集成提交的两父为82c3c61和c480cce，继承已有MTU改动，不宣称本轮新CPU优化。

## Actions证据

SOURCE bf11fbfbe64d518e7ba189d51bfb4512df4df733：
- next-foundation run37752919413: SUCCESS；<https://github.com/lly8666/wobuzhidao/actions/runs/37752919413>；repository continuity, Windows/Linux full unit/build, Linux race, arm64 cross-build and privileged OpenWrt/TUN/fallback。
- next-lifecycle run37752919401: SUCCESS；<https://github.com/lly8666/wobuzhidao/actions/runs/37752919401>；parameter catalogue, entry builds, lifecycle/fault tests and focused repeated race。
- next-windows-gui run37753038498: SUCCESS；<https://github.com/lly8666/wobuzhidao/actions/runs/37753038498>；portable build/manifest, owned network restoration, native GUI parameter/widgets/switching/portable writes。

Jobs/Run API核验均completed/success。foundation实际7个非skipped jobs SUCCESS；旧extended性能/soak/p6跳过，不计通过。GUI只有Windows portable/native widgets/manifest资格，不是Linux/ARM同源P6。诊断CPU、普通性能、真实TCP/UDP混合大包压力和新SOURCE物理全部NOT_RUN。

## 问题、排查与风险

无本轮新产品功能失败。未做优化收益测试，不能宣传CPU下降；runner异质、正式100ms与测试2/10ms tick差别、platformflow8936能力边界以及旧longmix FAIL按方案E0核。80秒下行仍OPEN_DEFERRED_UNTIL_AFTER_E6，独立1.23s大包迟到及其它旧FAIL不变。

## 下一项原子任务

全新agent执行E0真实助手审核/资格化和profile-off业务基线。bf11fbf基础功能已验无需无意义重复；新helper/产品改动再验自身SOURCE。随后每步依方案保护测试推进，不操作物理机、不合主线、每性能Action一条，失败完整留痕。
