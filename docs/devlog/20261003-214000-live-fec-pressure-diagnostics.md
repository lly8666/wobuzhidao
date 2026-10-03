# 20261003-214000 实时FEC满额退役与错误定位

## 本轮目标和阶段

P5交付前，起点23d302df5ed40fff5768482bca5c6b8e63626bfb。当前仍未通过全部hosted门。

## 修改与原因

实时FECPath使用AddLive：满额时最老且早于新BlockID的重恢复状态转既有compact状态；新修复包可以占槽重建。比所有重状态都保留直到3s更符合有界、低延迟、允许丢失的主旨。迟到旧block不挤走新block，仅保留compact元数据；迟到systematic保持首次交付和去重。reference Add容量错误契约不改；帧验证、20:R、8槽、3s最长恢复期限、8msflush、4096均不变。退役计数只常数增量，owner移除被压力退役的deadline，绝不改计数掩盖损坏。

LaneStats增加最多512字节LastPathError，仅出错时写入，无payload日志。HTTPS内容失败增加实际长度、hash、首个差异位置，仍要求102400字节完全一致，不重试放行。

## 复用来源

当前FEC RetireRecoveryExpired/compact delivery状态，无old复用；未改变wire。

## Actions证据

23的18独立标准弱网汇总37124942389 PASS；f12-l3-p1 37125022045 PASS。Normal180 37124966333 FAIL，3代server PathErrors=3/5/3且都在promote后1s内出现，cycle1-stress probe p95约1.039s/p99约1.866s，socket/capture/UDP drops=0。f10-l3-p0 37125010683 FAIL：实际HTTPS内容不等，manifest缺失是业务步骤失败后的次生错误，不是缺文件就是根因。原失败全部保留。新源码NOT_RUN；新增定向压力测试：parity-first新block恢复、旧源迟到首次交付、重复抑制、晚旧parity不挤新恢复、坏帧拒绝、reference契约保留。

## 问题、排查与风险

源码可确定旧压力算法会对合法parity-first报ErrDecoderFull，并丢新block的FEC机会；但现有日志只有PathErrors计数，不能断言这就是每次实际错误。LastPathError用于验证假设；若仍有其他错误，必须继续修，不能删完整性门。满额早退役改变恢复机会分配，是质量相关修改，需独立5205及完整资格，不能仅单测收口。HTTPS失败还未定位，不盲目改TCP关闭协议。未改门槛/速率/期限/窗口。

## 下一项原子任务

基础/race后只跑Normal180及f10-l3-p0诊断，取得具体错误原因再决定修复。原23长测可完成保留；新源不继承23结果。最终70配置、36生命周期、18弱网、Normal/Game1800与P6都须同源码PASS。
