# 唤醒退避测试区分建连次数与异步全局诊断

## 本轮目标和阶段

开始SOURCE98c5af1，实机仍273。新core/race失败先按原日志修测试边界，未部署、未跑候选性能。

## 修改与原因

新增harness测试把全局RetryableErrors断言为1；旧lane取消时异步read/bootstrap/tick也会写此计数，Actions显示Normal2、Game4/5，不是每个业务包重复发起Wake。改为用atomic OpenLane计数与RecoveryAttempts直接验证32个退避期需求没有额外admission；独立无transport的Dormant+retryAt状态验证32个需求不增加attempts/errors，同时保留lastPayload活动证据。失败/partial附件清理、后续实际交付、同owner/lease、真正wire/绑定错误不可吞的门未改。dedicated lifecycle Linux race selector明确纳入新测试count3。产品代码不变，不为了测试改变error统计或删除异步诊断。

## 复用来源

现有audit harness；无old提取。

## Actions证据

98c5af1 targeted37316853627 Linux race和Windows unit在新增计数断言FAIL；startup37316853595 Windows unit同原因FAIL。lifecycle37316853620、GUI37316853418、defaultnetwork37316853634、harness37316853805、predeliverytools37316853688 PASS。98foundation/fullstack尚未作为资格；新修正提交正确性/race/性能/P6全部NOT_RUN。失败日志hash与原因保存在wake-retry-test-accounting evidence，不能当98已验或抹除。每性能Action一条。

## 问题、排查与风险

global diagnostic并非单次调用的独占计数，不能用sleep等待它恰为1或干掉真实错误记账。本轮明确改变测试测量对象，非放宽吞吐/p99/恢复/完整性门。部分Wake成功后失败的server/client异步尾巴由现有生命周期处理，后续确实交付同owner/lease才过门。旧273 native上行损失、raw p99和maxUDP根因仍保留。

## 下一项原子任务

冻结修正候选并通过Actions foundation/targeted/race/lifecycle/GUI/fullstack；再独立Normal/Game5205及matchedGame5305/lossless、P6和Windows真实失败Wake清障恢复测试。实机在候选通过前保持273。
