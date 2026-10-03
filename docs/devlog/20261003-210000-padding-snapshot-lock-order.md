# 20261003-210000 填充与状态快照锁顺序修复

## 本轮目标和阶段

P5/P6完整配置交付前验收，起点1a5a4f5af8ba175e6c6766385edb6ea641939efb。

## 修改与原因

70配置中的f12-l3-p1原始run37123332069 HTTPS读取超时。其余UDP正常、完整性0、server诊断在HTTPS开始时停更，client随后认证记录超时恢复失败。审计确认ActiveLanes持owner.mu调用snapshotFor→Lane.Config获取lane.mu，而startup padding在lane.mu下回调owner.mu，形成锁倒置。snapshotFor仅需构造后不变的Role/ParityShards，改为直接读取不可变字段；owner-held身份检查同样只读不可变配置，不获取lane.mu。没有改变padding预算、crypto/FEC、业务交付、超时或任何参数，也删除了快照中不必要的全Config复制。LaneStats原本已在owner锁外读取，保持。

## 复用来源

当前datapath配置不可变及owner/lane锁边界，无old复用。

## Actions证据

1a源码基础/30race、正式18独立弱网全PASS（汇总37122894211），三目标P6包37123233854 PASS；f12-l3-p1原始配置FAIL和artifact11274333391保留。不能用其余69配置及旧长测关闭P5。新增确定性测试持packet lane.mu时ActiveLanes必须返回所有正确metadata并不阻塞owner.Stats；新源码全部资格NOT_RUN。

## 问题、排查与风险

观察证据吻合锁倒置，尚无该失败进程goroutine dump，不能编造调用栈。新确定性锁边界+全真实配置/HTTPS重新验收。构造后cfg无写者；任何未来动态配置不得直接修改它，否则必须重新设计锁顺序，不能恢复owner→lane锁嵌套。所有测量一Action一条，未通过的原样记录。

## 下一项原子任务

冻结新源码，基础/race/锁边界通过后重跑70配置、36生命周期、18独立弱网、Normal/Game1800s、共享黑洞与同源码P6包；仅hosted完成后进入用户安排的P7。
