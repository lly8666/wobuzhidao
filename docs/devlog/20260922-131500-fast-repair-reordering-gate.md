# 20260922-131500 fresh fast-repair 重排门

## 基线与失败证据

基线 SOURCE_SHA `aa7ec95dca504e819f1c09d6a5cd7a4b3519c68f`。

- targeted Actions 35684241526：6/6 PASS。
- foundation Actions 35684241516：3个红灯，不能继承前一SHA成功。
- fixed-FEC lossless job 106607608805：`TestP5FixedFECMeasurementHarness` 报 `lossless fixed-FEC scenario used repair retransmits=1`。
- artifact 10675374245 / zip sha256 `2a6fe47c4a4e18dc4cf509df3596fb5a43d5efec8ecc3bdfc2d7f0ce4d322f95` 的真实serialized pcap中，同一client steady Seq=27284、payload=151B在约3.29ms后重复发送。该间隔远低于1s RTO，因此确认是fresh fast-repair，而非RTO repair。
- pcap同时显示同机/FEC流在毫秒级出现后续记录先到的短暂重排；无丢包、无FEC reconstruct。

## 根因

`selectFastRepairLocked` 对已经重传过的记录使用RACK reordering-window，但首次发送的累计洞只要上方已有3条SACK就立即fast repair，没有任何时间门。并发FEC/source/parity/ACK调度可在lossless路径产生几毫秒短暂重排，于是被误判成丢包。

已有 `rackReorderingWindowLocked` 已定义：
- 无SRTT时最低10ms；
- 有SRTT时 `max(SRTT/4, 10ms)`。

本问题不需要新增参数或FEC等待。

## 修改

fresh fast-repair现在同时要求：
1. 累计洞仍是pending head；
2. 至少3个更高序号记录已SACK；
3. `rackLatestTx` 证明后续发送已交付；
4. 首洞发送时间早于最新已交付发送；
5. 首洞年龄达到已有RACK reordering window。

重传后的RACK分支不变；repair credit、effective RTO、3s horizon、gap forgiveness、4096、FEC数学均不改。

## 测试

新增 `TestSteadyFreshFastRepairWaitsForReorderingWindow`：
- 5条fresh记录，后4条被一个SACK块证明；
- t0+3ms：后4条payload正常SACK-retire，但不得fast repair；
- t0+12ms：相同证据已超过最低10ms重排门，允许首洞fast repair一次。

原有40ms selective-ACK fast-repair、50ms Seq wrap、budget和RTO测试继续保留，因此没有把真实修复路径禁掉。

## 下一步

本exact SHA先跑 targeted；随后看foundation的fixed-FEC是否恢复0 repair。另两个foundation红灯独立处理：
- platformflow FlowID tombstone 5s小于合法迟到帧寿命；
- server steady Window错误冻结bootstrap detach瞬时余量（失败pcap中为0）。

不把这些根因揉进本提交。
