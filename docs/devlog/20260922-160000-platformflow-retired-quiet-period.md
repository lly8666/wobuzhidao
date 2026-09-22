# 20260922-160000 platformflow retired FlowID 静默期语义

## 基线与最近四个 Actions

当前原子基线 `d51b55a85f29e90ab15b1aaf52bb6073a6a50b14`。

最近四个 Actions：
- `d51b55a8` targeted 35693066377：PASS，无失败/skip。
- `d51b55a8` foundation 35693066394：仅 `p5-weaknet-5-30-5 (2)` FAIL，其余全部PASS。
- `08e6637e` targeted 35692660972：PASS。
- `08e6637e` foundation 35692661039：最终整轮PASS；steady Window owner边界修正完整回归。

d51 run2 job 106639733961 在120s产品测试尾部报：
- `server transport stats unavailable`
- `server shutdown: platformflow: malformed frame: unknown TCP server flow`

artifact 10680465078，zip sha256 `bad5eb09baa53fb08a898e804dcc3e1be35ab33282c84aa1b4ef130c33fc8f92`。

## 失败定位

artifact的outer事件直到约120.8s仍持续存在正常c2s steady record，最后还有FIN。因此失败顺序是：
1. outer tunnel/lane仍存活；
2. 一个内层platformflow TCP FlowID在server侧查不到；
3. 该ID也不再命中retired tombstone；
4. server按fail-closed返回 `unknown TCP server flow`，随后整体Run退出并导致transport stats不可用。

上一原子把tombstone绝对寿命从5s扩到8s，容量仍4096。该修改仍失败，说明继续增大固定值不是正确闭包。

## 根因

`tcpRetiredSet` 过去把map中的时间解释为固定 `retiredAt`：
- flow被本端删除时写入一次；
- 之后合法迟到duplicate Open/Data/Ack即使持续到达，只返回true；
- **不会刷新截止时间**；
- retire+8s一到，下一帧立刻变成“从未见过的FlowID”。

这与reliable tail的实际语义不符。retired tombstone表示“这个FlowID最近已存在，但本端已结束；仍可能有在途/重传尾部”，因此退役条件应是**连续静默窗口**，而不是从本端retire动作起算的绝对租约。

持续的迟到frame本身就是该tail仍活跃的证据。只要每次间隔都小于8s，就不应该因为最初retire发生得早而突然把后续frame升级成协议攻击。

## 最小修正

- `DefaultTCPRetiredTimeout` 保持8s。
- `DefaultMaxTCPRetired` 保持4096，不扩大缓存。
- `tcpRetiredSet.contains(id, now)`：
  - 未命中：仍false；
  - 距最后观察时间已>=8s：删除并false；
  - 合法命中且now前进：把该ID时间更新为now。
- `prune` 对同一字段按last-seen解释。
- duplicate retired `TCPOpen` 也通过同一contains，因此同样刷新静默窗口；它不会重建upstream。
- 真正陌生ID、静默满8s后的旧ID仍 `ErrMalformed`，fail-closed不变。

没有改：
- 内层500ms RTO / 8次重传；
- outer 3s repair horizon；
- 4096 active/retired容量；
- FEC、padding、Window、MTU、repair credit、业务pacing。

## 定向测试

新增 `TestTCPRetiredFlowQuietPeriodRefreshesOnLateTail`：
- FlowID 77在t0 retire；
- t+4/8/12/16/20s连续收到迟到ACK，每次都必须静默接受；即使绝对时间已超过最初8s很多也不能误报unknown；
- 同时从未见过的FlowID 78仍必须 `ErrMalformed`；
- t+20s最后一次tail后，`len()` 在静默8s前仍保留；
- 精确静默8s边界后，同一FlowID再来ACK必须恢复 `ErrMalformed`。

这直接验证“活跃tail可跨多个绝对timeout，但真正静默后仍及时回收”。

## 下一步

本exact SHA必须重新跑：
1. next-p4-steady-targeted全部6项；
2. next-foundation整轮，尤其两个非FEC 30% seed、fixed-FEC lossless、Windows/Linux active-go和soak。

若同SHA整轮PASS，则第5原子的代码回归闭环完成。之后开始专项新增真实Linux路径：netns/veth/netem + 正式client/server独立进程，先无损闭环和校准，再18份120s持续UDP主测；旧HTTPS门仍只作回归证据。
