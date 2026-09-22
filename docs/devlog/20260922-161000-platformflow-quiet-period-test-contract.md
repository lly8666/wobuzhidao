# 20260922-161000 retired quiet-period 旧测试合同修正

## 基线

SOURCE_SHA `b197dacf9bc35a78a2d633e703dd8f3d15df750e`。

b197dacf引入的产品修改是：
- retired FlowID tombstone仍8s、容量4096；
- 合法duplicate Open/Data/Ack命中后刷新last-seen；
- 连续静默8s才真正过期；
- 陌生ID继续fail-closed。

## CI失败

foundation 35698531723 的 Ubuntu active-go 在 `internal/platformflow` 失败：
- `TestTCPRetiredFlowTailIsBoundedAndUnknownStillFailsClosed`
- `TestDefaultTCPRetiredTimeoutCoversLateReliableTail`

两条失败都不是产品逻辑异常，而是测试仍按旧绝对时间语义：
- 第一条在 `start+1ms` 已处理合法retired tail，却仍在 `start+timeout` 期待过期；
- 第二条在 `start+7s` 已处理合法retired tail，却仍在 `start+8s` 期待过期。

在sliding quiet-period语义下，这两个时刻都尚未静默满timeout，因此返回nil是正确行为。

## 修正

仅修改测试基准：
- 第一条到期点改为 `start+1ms+retiredTimeout`；
- 第二条到期点改为 `late+RetiredTimeout`。

不改：
- platformflow产品代码；
- 8s timeout；
- 4096容量；
- inner/outer RTO/horizon；
- FEC、Window、MTU、padding或pacing。

## 下一步

新exact SHA重新跑targeted/foundation。只有同SHA核心/race/privileged、fixed-FEC和两个30% seed都通过，才关闭第5原子代码回归。
