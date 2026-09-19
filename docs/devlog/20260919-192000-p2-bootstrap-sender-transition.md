# 20260919-192000 P2 Bootstrap Sender与Transition边界

## 本轮目标和阶段

继续P2，只完成STATUS指定的下一原子任务：从old/internal/faketcp/arq.go提取bootstrap所需Sender/Pending最小行为，并按DEVELOPMENT_PLAN第3节实现内部prepare/detach接收所有权边界。不开启完整steady-state ARQ，不接FEC。

## 定向审查结论

bootstrap_retransmit_test.go实际需要的成熟行为只有：
- Enqueue复制payload并保存Seq/End。
- BootstrapStream同步marker把该Pending标为Bootstrap。
- bootstrap RTO取shared RTO与2秒ceiling的较小值。
- bootstrap RTO重传不放大shared RTO。
- cumulative ACK释放已确认Pending。
- 后续普通Pending仍按普通RTO重传并指数backoff。

完整arq.go中的SACK/RACK、repair budget、partial reliability、Receiver pressure与4096 steady-state horizon都不属于本原子任务，因此不复制。

真实ACK-gated BootstrapStream还需要并发ACK等待，新增Sender.WaitAck(end, deadline)。Sender内部加锁并使用广播notify，使Write等待与packet receive推进ACK可以安全并发。

## prepare/detach边界

新增internal/faketcp/transition.go，依据DEVELOPMENT_PLAN固定的所有权规则实现：
- Prepare记录精确bootstrap结束Seq，不通过TLS header猜模式。
- prepare后，boundary之前的晚到bootstrap仍归旧路径；boundary及之后的提前新record复制到transition queue，绝不Feed回TLS。
- 纯ACK始终返回FakeTCP ACK分类。
- Detach一次性转交提前record队列，此后新record直接归新接收器；晚到旧bootstrap单独分类。
- queue硬上限64条，总字节上限64 * negotiated record wire max；单条也不能超过wire max。
- 同Seq同payload的提前重传去重；同Seq不同payload视为违反“重传字节不变”，候选abort并清空buffer。
- payload跨越bootstrap boundary属于reader/所有权歧义，候选abort并清空buffer。
- Abort只清候选buffer，FakeTCP ACK可由外层继续处理，不要求杀旧ACTIVE lane。

## 复用来源

源SHA固定b5c848f4e9afdffd15d1bc451560edf4e9390a35。

- old/internal/faketcp/arq.go + bootstrap_retransmit_test.go -> internal/faketcp/bootstrap_sender.go
- old/internal/faketcp/arq.go -> internal/faketcp/sequence.go新增seqLE
- transition.go为新实现，规范来源是当前DEVELOPMENT_PLAN第3节，不是从old整包复制。

docs/REUSE_LEDGER.json已更新。

## 测试

新增bootstrap_sender_test.go覆盖：
- 2秒bootstrap retransmit ceiling。
- bootstrap重传不改变shared RTO，后续普通payload仍正常backoff。
- WaitAck在cumulative ACK后解除、deadline超时。
- 重传Seq/payload逐字节不变。
- cumulative ACK跨uint32 wrap。

新增transition_test.go覆盖：
- prepare前bootstrap ownership。
- prepare后晚到bootstrap与提前record分流。
- 纯ACK不进入payload路径。
- detach转移queue及detach后直接record。
- 同Seq重传去重/冲突abort。
- 64条上限、单record wire上限。
- boundary overlap fail-closed。
- boundary跨uint32 wrap。
- prepare/detach状态机。

## Actions证据

本日志创建时新代码尚未执行Actions；最近已验证SOURCE_SHA仍为23cef50c0f50a5ca2c8ab7b752cadfe1ac9acdc3。提交后只以新精确SOURCE_SHA对应run判断成功或失败。

## 下一项原子任务

先验证本提交Windows/Linux unit/build与Linux race。若通过，再定向提取FakeTCP SYN/packet/persona最小闭包并开始把真实TLS bootstrap挂到同一association；继续不迁移steady-state完整ARQ/FEC。
