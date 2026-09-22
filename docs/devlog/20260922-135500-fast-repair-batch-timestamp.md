# 20260922-135500 fresh fast-repair 批量时间戳修正

## 基线失败

基线 `59fa2e2eb2553751cf2d357063e3fee73404f2ab`，Actions 35688167216。

contract、lifecycle-focus、iptables/nft均PASS；Linux/Windows steady-core同时FAIL。失败集中在runtimeowner：
- 40ms selective SACK fast repair未触发；
- repair-budget测试预期一次defer但没有进入repair选择；
- 1024 deep-hole与Seq-wrap测试的fast repair未触发；
- 新增3ms/12ms重排门测试在12ms仍未触发。

## 根因

`laneTransport.send(records, now)` 是批量发送入口，同一批多个fresh records有意使用同一个 `now` 写入 `lastSent`。因此后续SACK证明“更高Seq已到达”时，`rackLatestTx` 可以与首洞 `lastSent` 相等。

上一提交除了要求首洞年龄超过RACK reordering window，还错误要求 `candidate.lastSent.Before(rackLatestTx)`。这个严格纳秒时间序关系不是loss证据，且与批量send语义冲突。

## 修正

fresh fast repair条件收敛为：
- candidate是累计洞head；
- 至少3个更高记录已SACK；
- candidate有真实lastSent；
- 当前时间不早于lastSent；
- candidate年龄达到已有 `rackReorderingWindowLocked()`。

不再要求同批记录的发送时间有严格先后。

因此：
- 3.29ms lossless FEC短重排仍小于最低10ms，被挡住；
- 12ms新测试允许repair；
- 40/50ms既有fast-repair恢复。
- retransmitted RACK分支、repair credit、RTO/horizon/FEC均不变。

## 下一步

本exact SHA重新Actions。只有targeted恢复并且foundation fixed-FEC lossless回到0 repair，才关闭这个根因。FlowID tombstone与steady Window=0继续保持独立后续原子。
