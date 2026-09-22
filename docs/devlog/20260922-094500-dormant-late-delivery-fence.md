# 20260922-094500 DORMANT late-delivery fence

## 前置结果

SOURCE_SHA `c16fb3f18ee7ce77706fef3716ef941eecfff06a` 的 effective repair RTO 修正已证明解决第3原子的30%回归：
- next-foundation 35676490092 / p5-weaknet-5-30-5 run1 job 106584023708 PASS，artifact 10673381808，ZIP `sha256:e48e2f8725507c6962679ae1fa162db3ec30d866716c88b184006e5b7af6b684`，repair=588，wire_amp≈5.0558。
- run2 job 106584023553 PASS，artifact 10673630740，ZIP `sha256:b53c6375de57ea06657ebf4b22f3900008a57a65f46bd779aeea5e69833aa312`，repair=584，wire_amp≈5.4706。
- Windows/Ubuntu active-go普通回归均PASS。

但独立 targeted 35676490093 的 Linux race step复现：
`TestLifecycleEntryGameReplacementDormantWakeKeepsStableLease` 在显式server DORMANT之后 `TunnelStats(...)` 返回 `ok=false`。同job普通测试PASS，runtimeowner effective-RTO新测试race PASS。

## 根因

`dormantGroup` 已正确在调用 `group.rt.Dormant()` 前设置 `group.dormant=true`，并在transport清理后删除byFlow/table entry。

仍有一个过渡窗口：一个旧generation业务payload可能已经进入 runtimeowner 的 owner->server delivery callback。该callback此前无条件：
1. 更新 `group.lastPayload`；
2. 调 `Router.DeliverFromOwnerAt`；
3. 将platformflow/service错误原样返回。

DORMANT并发关闭业务flow后，这个在途payload可得到 `unknown TCP server flow` 等退役错误；错误继续穿过runtimeowner/handleSegment，最终令 `LifecycleServer.Run` 返回并执行Close，清空 `byTunnel`。因此表象是DormantTunnel本身成功，但紧接着TunnelStats的group消失。

这与effective-RTO代码无共享状态关系；只是其targeted race调度再次把既有生命周期边界暴露出来，必须修清而非豁免。

## 修正

新增 `deliverTunnelPackets` 作为唯一server owner->业务投递边界：
- callback开始时若group已经DORMANT，直接丢弃旧generation业务，不刷新payload activity；
- active group仍按原逻辑投递，任何错误原样传播；
- 若投递开始时active、期间并发进入DORMANT，而service因flow已退役返回错误，则再次检查group状态；仅在此时已DORMANT才吞掉该退出lane局部错误。
- 不改变ACK/FIN/RST控制处理；该helper只包业务packet delivery。
- 不增加热路径新锁：原callback本来就为更新 `lastPayload` 获取 `s.mu`，新逻辑复用同一把锁；第二次检查只发生在投递已经返回错误的冷路径。

增加定向单测证明DORMANT后的late business不返回错误且不刷新activity。现有Game replacement/DORMANT/wake test和Linux race继续作为真实并发回归。

## 范围

不改recovery参数、RTO、FEC、4096、MTU、padding或业务pacing。下一步先重新跑exact-SHA targeted gate；只有race和Game4稳定后才进入第4原子索引优化。
