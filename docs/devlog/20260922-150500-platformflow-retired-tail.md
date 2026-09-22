# 20260922-150500 platformflow retired FlowID 迟到尾部寿命

## 基线与证据

本原子基线 `08e6637e2899f876794eb787e4399c1eb45d30ee`。

前一原子 steady Window owner 边界：
- targeted Actions 35692660972：6/6 PASS。
- foundation Actions 35692661039：提交本原子时28/30 PASS、0 FAIL，仅两个soak仍运行。
- 其中非FEC 5→30→5 run1/run2均已PASS；run1在上一SHA fbc8555a曾因final-5%第三个连续成功来不及完成而FAIL，08e6637e已恢复。
- 不把仍运行的soak提前写成整轮PASS。

FlowID根因来自第5原子首候选 `aa7ec95dca504e819f1c09d6a5cd7a4b3519c68f`：
- foundation Actions 35684241516 / job 106607609116 在120s 30%旧护栏尾段出现 `platformflow: malformed frame: unknown TCP server flow`，随后server transport stats不可用。
- platformflow对已删除TCP FlowID仅靠 `tcpRetiredSet` 吞掉迟到 `TCPData/TCPAck/TCPClose`；真正陌生ID必须继续fail-closed。
- 默认 `RetiredTimeout=5s`，容量4096。

## 参数链推导

不直接拍一个更大值，也不扩大4096容量。

现有默认内层TCP reliability：
- RTO = 500ms；
- MaxRetransmits = 8；
- segment初发后，最后一次允许的内层重传约在 `8 * 500ms = 4s`。

outer steady当前正式repair horizon：
- `runtimeowner.DefaultRepairHorizon = 3s`。

一侧已retire后，对端并不一定同步完成关闭；因此仍可能：
1. 在最多约4s内继续发出内层可靠重传；
2. 最后一个内层frame封入outer steady record后，再在3s outer repair horizon里产生合法迟到副本；
3. qualification主路径还有300ms单向时延，timer/tick也有调度误差。

因此5s不能覆盖完整尾部。新默认取：
- 4s inner tail
- + 3s outer repair
- + 1s delivery/tick slack
- = 8s。

1s slack高于本轮300ms单向目标，又不引入动态后台扫描或大容量缓存。

## 修改

- `DefaultTCPRetiredTimeout`: 5s -> 8s。
- `DefaultMaxTCPRetired` 保持4096，不扩大容量。
- tombstone set实现、淘汰算法、陌生FlowID fail-closed语义不改。
- 不改变TCP reliability RTO/retransmit次数，不改变outer repair/FEC/padding。

## 测试

### platformflow

新增 `TestDefaultTCPRetiredTimeoutCoversLateReliableTail`：
- 使用默认配置创建client/server retired set；
- FlowID=77在retire后7s收到迟到ACK和Data，两侧都必须静默吞掉，不重建upstream；
- 同时FlowID=78从未存在，仍必须 `ErrMalformed`；
- 到精确8s后FlowID=77 tombstone失效，再收到ACK也必须恢复 `ErrMalformed`；
- 断言最大retired容量仍等于4096。

### runtimeentry 参数一致性

新增 `TestPlatformFlowRetiredTimeoutCoversSteadyRepairTail`：
- 从实际 `DefaultTCPConfig().Reliability` 计算inner tail；
- 与 `runtimeowner.DefaultRepairHorizon` 直接相加，再加1s slack；
- 若未来有人修改inner RTO/retry或outer horizon而没同步tombstone，测试立即失败；
- 容量仍检查4096。

## 不做的事

- 不把真正陌生FlowID改成忽略；攻击/协议错误仍fail-closed。
- 不扩大4096 retired集合。
- 不增加假业务、等待、随机delay。
- 不修改RTO、FEC数学、repair credit、gap forgiveness、Window或MTU。
- 不用重跑覆盖aa7ec95d的旧失败证据。

## 下一步

本exact SHA先走targeted和foundation。若旧30%护栏不再出现unknown-flow且core/race全绿，则第5原子的三个回归修正（fast-repair短重排、steady Window owner边界、FlowID tombstone）可以合并做同SHA回归判定。之后开始新增专项真实Linux netns/veth/netem正式进程无损闭环与校准；旧内存SegmentIO HTTPS门只保留为回归护栏。
