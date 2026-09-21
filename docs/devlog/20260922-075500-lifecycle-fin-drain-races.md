# 20260922-075500 lifecycle FIN drain 竞态修正

## 失败证据

基线 `0655f55b4056dc5f7238387f5865344cb267a277`。第2原子已把steady FIN/RST/half-close接到runtimeowner，并把replacement关闭预算限制为 `min(ReplacementGrace, 2*InitialRTO)`，但同SHA Actions仍有当前修改相关红灯。

- Actions 35625178099：Linux/Windows active-go在replacement收敛或DORMANT后server状态处失败。
- Actions 35625327362：Windows active-go还复现Game4只即时观察到2个duplicate；另有5305 run1红灯保留原始证据。
- 其它大量job可通过不能覆盖这些失败。

## 根因

1. server replacement的2*InitialRTO预算没有被持续执行。fresh lane资格时会立即发old-lane FIN并设置closeStarted，但server tick仍只在 `promotedAt >= ReplacementGrace` 时再次调用retire逻辑，默认3s与测试3s边界重合。
2. server DORMANT先 `group.rt.Dormant()` 清steady transport，再删除 `byFlow`。这个窗口里迟到ACK/FIN仍可命中旧lane，得到transport missing/closed并终止整个server Run，随后defer Close清空byTunnel。
3. Game4四份副本由四个独立lane read loop异步处理。client已同步证明 `GameLaneCopies=4`，但server TUN在第一份交付后即可唤醒测试；立即读取duplicate统计可能只看到2。门槛应保持3 duplicates，只需要等待已发副本有界drain。

## 修正

- server tick只要replacement的 `closeStarted` 已设置，就每个tick继续评估FIN完成/关闭预算；ReplacementGrace仅保留为fresh qualification始终未出现时的fallback触发。
- DORMANT在清runtime transport前先把group标记dormant。此时若并发迟到控制遇到missing/closed/reset，只在该lane已非当前或group确实dormant时忽略；runtime Dormant失败则回滚标记。
- detached且 `byFlow` 已不存在时，只有 `started[flow]` 表示async admission handoff仍在进行才允许进pending。已attach后退役/DORMANT的迟到控制不再重新生成pending ownership；已关闭association直接忽略。
- Game3/4 closure测试仍要求 `GameDuplicates >= lanes-1`，改成3秒内等待独立lane副本drain，没有降低复制或去重要求。

## 范围

仍只处理第2原子生命周期关闭和现有Game4间歇红灯。未迁入old SACK/RTO参数，未改FEC、4096、padding或业务pacing。

## 下一步

本exact SHA必须重新Actions。Linux/Windows普通测试、race、replacement+DORMANT、Game4和privileged路径全部通过后，才关闭第2原子并进入第3项。
