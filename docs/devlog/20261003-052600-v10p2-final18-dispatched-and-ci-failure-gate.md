# 20261003-052600 正式18样本已分发与CI失败传播

## 本轮目标和阶段

P5 V10.2正式资格，产品仍固定56eb5413c3cf2e559b82026e8a5783508764e2f4。canonical修改只涉及Actions验收入口及交接，控制面main为da82a86860842c659fea2b0ce7b145c7d9eaec9c。

## 修改与原因

- main既有final18控制器增加只读collector/aggregate job。新冻结ref perf-fixed/56eb5413c3cf2e559b82026e8a5783508764e2f4-r2从零18样本；每份一个独立run。汇总分析器单独固定6dafa657af9f577f8cc21256276bfb44d8ee3fd1，记录产品/分析器不同SHA，不混源码。
- 收集GitHub run/summary artifact receipt并逐份验证身份。全组完成后执行revision2原RTT门及完整矩阵门；任何失败不重跑覆盖，不P6晋级。
- 进一步发现strict/blackhole workflow分析器步骤continue-on-error用于上传证据，但末尾只检查collection成功，可能让分析失败仍显示绿。未来入口末尾同时检查sample和validate outcome，policy静态守卫防回退。正在运行冻结样本不改写；其成绩以五分类summary和正式aggregate为准。

## 复用来源

复用main既有分发器，未复用old产品。

## Actions证据

- [37065380317](https://github.com/lly8666/wobuzhidao/actions/runs/37065380317)，SHA6dafa657af9f577f8cc21256276bfb44d8ee3fd1，analysis-unit job111031801759：既有wall accounting2项和新增aggregate11项全部PASS。
- 6dafa65缺STATUS导致contract失败已保留；47e01b587fe01157afd4895c015e52a0242ebbe5 foundation [37065495783](https://github.com/lly8666/wobuzhidao/actions/runs/37065495783) 的contract/Windows/Linux unit+race/fuzz/真实fallback/privileged TPROXY/shared TUN已PASS，性能与P6 job按设计SKIPPED。
- [37065816473](https://github.com/lly8666/wobuzhidao/actions/runs/37065816473)，main控制器分发job111033217612 PASS；只读汇总job111033401800运行中。
- 新formal18的SOURCE_SHA均56eb5413c3cf2e559b82026e8a5783508764e2f4，18个run从37065827912至37065869715，均attempt1独立workflow_dispatch。截至本日志写入均RUNNING，不提前PASS。
- 本提交CI outcome policy待Actions，不在本地跑检查。

## 问题、排查与风险

严格区分workflow成功和summary五分类成功；冻结旧入口的continue-on-error漏洞由外部正式汇总拦住。历史failed samples、V10.1 17/1及诊断全部保留。本轮尚不满足目标速率>=30min soak，也不关闭P6/P7。

## 下一项原子任务

核对18原始结果与artifact-only RTT资格，失败先定位具体socket/队列/时序并停止晋级；通过后规划同版本Normal/Game独立目标负载长测。新长测需要有界抓包/统计，不沿用旧低速31min HTTPS结果替代。
