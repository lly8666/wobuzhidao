# 服务端退休SYN候选的错误隔离

## 本轮目标和阶段

从7d276aa继续，配套实机仍部署2bf。复核9318224 foundation37326843336发现真实失败，先修这条共享server关闭边界，不盲重跑原生。

## 修改与原因

runtimeentry/lifecycle.go将SYN响应提取为emitSYNACK：AddSYN返回半开duplicate对象后，admission/partial Wake rollback能并发Close它；SYNACKSegment因此返回ErrHandshakeState。该失效候选没有合法响应可发，局部忽略，不终止共享listener、不复活对象、不更换ISN；其他client和健康duplicate行为不变。真正IO.Emit失败继续传播，且错误上下文明确emit server SYNACK；即使Emit本身返回ErrHandshakeState也不能被误吞。

新增确定性测试，在lookup之后、响应snapshot之前Remove/Close，确认0发包/0复活、随后另一client和其duplicate回复保留原ISN；EPERM/EBADF/自定义error/Emit返回ErrHandshakeState均原样errors.Is可见。next-lifecycle race selector加入测试，predelivery dedicated job重复failedWake和新测试30次。只改建连失效分支，无正常稳态额外锁、队列、发包、重试、参数或wire变化，无HOL。

## 复用来源

现有ServerAssociationTable与SYNACKSegment、lifecycle harness，未提取old。

## Actions证据

父9318224 foundation37326843336 Linuxrace FAIL：TestBusinessDemandAfterFailedDormantWakeRetriesWithoutClosingOwner/4后续清障Wake lane1 client handshake failed；cleanup读取server Run为invalid association handshake state。实际日志保留外部wake-retry-gates/37326843336-failed.log。

源码中handleSegment其他association state错误已有local处理，SYNACKSegment返回同error仍能直接终止Run；lookup与关闭之间存在窄窗。确定性测试锁定这个路径；未捕获旧失败精确调用栈，不能把所有历史sharedcredentials失败都宣称同一根因。

7d276aa predelivery37329484643四job PASS；其中real-kernel tc fixture artifact11353626855验证raw发送成功、目标真正丢包、其他协议/端口/peer不受影响、清障恢复、foreign clsact/filter/root不改。ARM5.4 native尚NOT_RUN。新产品候选core/race/性能/full36/P6均NOT_RUN，提交后执行；实机不先部署。

## 问题、排查与风险

原生1410的EPERM与此次retired object ErrHandshakeState是独立问题，不能混为同一错误。不会为通过本机OUTPUT拒绝测试吞EPERM或将未发包写成成功。已有五独立性能仍属2bf/hbce旧snapshot，不继承到新候选；已知后来race失败也明确公开。

## 下一项原子任务

新SOURCE core/race、failedWake+retiredSYNACK30重复、36+aggregate、五独立性能/18RTTpairs和P6通过后，配套部署新源码并执行fresh1411 tc-egress300s失败Wake；同时保存actualdrop、两端samePID、独立target/resource、新业务清障恢复与ownedcleanup。保留旧Normal wake loss/rawp99/maxUDP/32未测工况。
