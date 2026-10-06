# 20261006-223000 Windows有界最新ACK发送

## 本轮目标和阶段

开始HEAD a75badb（上一原子诊断归档），产品部署仍78b8faf。基于ACK反馈209.055s而Owner45.797s/Deliver5.786s的实机诊断，缩窄到Windows接收线程同步native ACK等待，不改FEC/4096/repair/MTU。

## 修改与原因

runtimeowner/ack_feedback.go只有每generation一个lazy worker、一个pending位和容量1wake token；无包/业务正文/ACK历史缓存，堵住native写入也不会继续生成worker。sendACK原2records/2ms/urgent决定仍在接收交付之后；后台在发送选择边界读取当前Seq/ACK/SACK，繁忙时只合并已过期确认。成功data piggyback只在gap-free且当前Ack覆盖时清待发，不吞SACK。FIN ACK必须同步经过cfg.Emit后handleSegment才返回，challenge ACK/selectedrepair也保留原同步。Windows正式入口内部开启，其他平台/嵌入者默认关闭；runtimeentry为新的active/ref继承并保留retiring精确cfg，close/RST取消pending，owner锁内不等worker/driver；已选择IO仍由native gate drain。失败锁存/计数并通过原ackAsyncError tick fatal出口，worker不自旋重启；closed失败不污染新ref。计数区分queued/coalesced/piggybacked/attempts/success/failure及pending/running；资格计时新增实际worker emit，不能把enqueue耗时下降当CPU下降。默认off逐包观察，新增锁内固定计数无新timer/业务等待。原文件局部gofmt，无其他重构。

## 复用来源

无old复用；本分支既有ACK决策、timer、tick错误出口、native IO gate/owner generation fence复用。

## Actions证据

当前NOT_RUN，无本地编译/unit/race。新测试真实runtime/datapath在首ACKnative emit受控堵住时继续0->2->1乱序first-arrival交付、100重复ACK只留一份最新反馈；close堵住worker时清pending、停止旧任务；原错误tick可见且不重启；SACK和旧piggyback不取消最新反馈；通过actual FIN handleSegment验证确认同步。真实TLS/Game replacement-DORMANT-wake原测试同时打开新flag并核验每代配置；runtimeowner恢复Actions新增worker race10。接着独立Normal/Game5205及同seed lossless、12阶段p95/p99、P6。每性能Action一条。

## 问题、排查与风险

上一条stageon可扰动临界容量，51534用户queueoverflow/4343downmissing/19probe未回/p99712.93ms保留，不是普通off回归或CPU证明。worker仍可能与fresh争native sendMu；收益需要stageoff实机证明，不声称减少209秒CPU。正常TCP会合并ACK，但短窗口有损外观/有限修复边界保持诚实；无HOL/有界状态优先。FIN强制同步防两边过早retire；旧已在途ACK不能撤销但不携业务/密文、不混新ref。49普通300s/23unique/20NOT_RUN不变另2诊断，M03保留。

## 下一项原子任务

先全部Actions门，失败保留并修最窄原因，不扩缓存/soften p99。之后同源P6+配套部署，显式stageoff单300s Normal10再Game4×3验吞吐/p99/overflow/真实worker生效与退出清理；必要诊断再显式开启worker Emit计时，不能替代普通性能样本。未验不能标完成/P7关闭。
