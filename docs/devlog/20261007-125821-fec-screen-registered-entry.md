# 20261007-125821 复用已注册Actions入口

## 本轮目标和阶段

产品d6冻结，快速批量FECscreen测试；不更改runtime。

## 修改与原因

新workflow在非默认分支未被GitHub注册，API404，尚未分发任何性能run。a9df7da的preflight37573760403已PASS（数学/profile单位和原seedednetem门）。改为现有已注册next-strict-weaknet的显式qualification_kind=fec-profile-screen入口，fec_parity每次一个；默认formal20和原checker/原artifact路径保持，非20且不声明screen被shared脚本拒绝。screen仍独立schema，理论与原门结果保留，rawpcap审计删除，仅小回执上传。移除未注册重复workflow，避免两套测试入口。

## 复用来源

现有Actions和同一shared脚本，无old新复用。产品源码/参数清单不变。

## Actions证据

a9数学与profile checks/preflight已PASS，旧36计划未写入/无性能dispatch。当前入口修订待新提交preflight，随后冻结新harness SHA再36独立run；原a9冻结ref是失败入口历史，不用于正式screen。每性能Action一条，聚合只读。

## 问题、排查与风险

工作流注册404是平台入口问题，非产品故障。默认参数不能变成低档位；summary schema不能混入formal20。保留原checker门，不把理论参考当完整business oracle或回写历史FAIL。

## 下一项原子任务

验当前preflight+shell语法后分发36条；逐profile/mode同seed独立无损配对RTT与probe覆盖，收齐后只有异常项定向验证，符合性能/延迟/noHOL/能力则冻结实现。
