# 20260924-040000 首次 scoreboard arm 不再依赖发送时间差

## 18a8b044 第一候选实测

第一候选 `18a8b044879c65c0e13ff0c3b04c4295483348dc` 已完成：

- next-runtimeowner-recovery `35905872242`：PASS（unit + race）
- next-p4-steady-targeted `35905872229`：PASS
- next-foundation `35905872230`：PASS
- next-lifecycle `35905872300`：PASS

独立性能样本继续遵守 one workflow run = one sample：

### Normal / lossless / seed101

- run `35906251206`
- compact artifact `10771346640`
- 五分类全部 PASS
- socket/link drop = 0
- C2S outer/app = 5.0916959365x
- S2C outer/app = 5.22309074x
- repair outer bytes = 0 / 0
- FastRepairs = 0，RTORepairs = 0

因此 armed persistence 没有把历史无损短重排重新变成假重传。

### Normal / 5205 / seed101

- run `35906907537`
- compact artifact `10771542248`
- 五分类全部 PASS
- socket/link drop = 0
- C2S outer/app = 5.23066704x
- S2C outer/app = 5.3619056x

与历史 `689dea19...` / run `35894596679` 比较：
- 5% pre/post fast repair从约27–33提升到约37–39；
- 但20% stress两方向 `FastRepairs` 仍然都是0；
- stress `Abandoned` 仍约7.8万/方向。

因此第一候选安全，但没有解决目标中的高损repair缺失。

## 根因

`selectFastRepairLocked` 第一候选仍写成：

- 至少3个SACK；并且
- `rackEvidenceAgeLocked(candidate)` 必须有效。

`rackEvidenceAgeLocked` 要求 `rackLatestTx > candidate.lastSent`。高吞吐数据路径一次 `send(records, now)` 会给同一批records相同的 `lastSent`。真实丢包时，累计head之后虽然已经有大量SACK，但如果这些后继来自同一批次，发送时间差为0，RACK evidence不可用，于是连armed状态都无法建立。

这不是重复repair的RACK问题，而是把“发送时间证据”错误当成了“首次scoreboard repair的存在性前提”。

## V2最小修正

首次repair现在分两层证据：

1. **scoreboard arm**：当前累计ACK head + 至少3个后继SACK，即可arm；不要求后继必须有不同的发送时间戳。
2. **strong RACK immediate**：若确实存在有效transmit-time evidence且达到现有reordering window，可直接fast repair，保持原路径。
3. 若只有scoreboard证据，则仍使用第一候选的“一条armed head + 跨完整recovery调度周期持续存在”后再发一次repair。
4. 已经重传过的candidate继续要求现有RACK时间证据；不放宽重复repair。
5. 4096、3s horizon、1s RTO、fresh/5 credit、128KiB burst、渐进重复成本、FEC/wire/性能门槛全部不变。

定向测试把 persistent-loss case 改成单次batch发送5条record，故意制造相同 `lastSent`；V2必须仍能arm并在持续缺口下repair。transient reorder、full-window fresh eviction及其它既有测试继续保留。

## 当前状态

本V2提交创建时：
- compile/unit/race：NOT_RUN
- performance：NOT_RUN
- 18a8b044 的全部PASS与“stress repair=0”结果保留，不覆盖
- 历史689dea19 final18/P6继续保留

下一步先Actions正确性；之后新SHA从lossless和5205重新独立测量，不能把第一候选样本当成V2资格。
