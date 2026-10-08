# 回退过严的 ACK worker 关闭后 channel 空断言（2026-10-09）

只在 `next/performance-efficiency-20261008`，父HEAD `bd02d8d42d66d9b638c6dc9138e25d663d822605`；不修改产品代码/协议/参数，不触碰主线或物理机器。产品SOURCE仍 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`。

[Lifecycle run37856286059](https://github.com/lly8666/wobuzhidao/actions/runs/37856286059) 的job113581128291实测`go test ./...` **FAIL**：`TestACKFeedbackBlockedWriteKeepsNoHOLAndOnlyLatestACK`在test.go:149打印`ACK history replayed after worker close`，确认为上一次新增的**非原始**断言。原有worker行为允许在 Close 之前已开始、native Emit正在进行中的ACK写在释放阻塞以后完成；同步`server.Close()`并等`ackWorker.done`后channel内仍保留先前合法写入的包，并不等于关闭后重新选择或发送。不能把已入途的ACK认定为close后复活。原始测试只验证阻塞写仍首次业务无HOL、100重复ACK合并、一份pending/latest-only及解除阻塞后的正确最新ACK；真正关闭不复活语义由另一个`TestACKFeedbackCloseDropsPendingAndFencesOldWorker`独立覆盖。

因此**只删除**刚引入的`server.Close();wait(done)`之后读ACK channel且要求为空的过严附加断言。保持显式`entered`握手、首次业务顺序/内容、`ACKWorkerAttempts==1`/coalesced/bounded、第二ACK Ack序号和noSACK/noPayload、worker.done收口、原close test的`calls==1`及pending/running/closed断言。不使用sleep、不放松原门，产品无代码改动。原run178...和run37856286059 FAIL保留。之前Foundation run37856284837 YAML无jobs FAIL已修复，当前 Foundation run37856511252来自上一helper，结束结论不继承到本提交。

下一项新helper原生Actions真实验收定向100次普通及30次race，后full race和lifecycle；如果仍不通过继续缩小问题，不能跳到性能样本。Game4 37853468730 scoped PASS不变，CPU收益UNPROVEN，Game4旧run37817466498 UDP446丢失与37851028041 packet140drop FAIL、其它门及E7 80秒中断OPEN、E6/P6/物理NOT_RUN。
