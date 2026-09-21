# 20260922-001500 稳态 FIN/RST/半关闭接线

## 本轮目标和阶段

分支 `next/tlslike-dataplane`，基线 `e3253322ac863c11636f1c127e14b77bdc6bf800`。对应 `docs/WEAKNET_QUALIFICATION.md` 第1节第2项：把P2已有FIN/RST/半关闭语义接到detach之后的steady入口，控制包使用steady `sendNext/recvNext`；覆盖rotation、DORMANT、显式退出和尾部数据，并保持detach与网络关闭分离。

前一原子修复的 Actions 35616392974 已取得定向PASS：repository-contract、Windows/Linux active-go（Linux含race）、OpenWrt privileged、Linux shared-TUN iptables/nft均通过。该workflow中的长时P5 jobs不提前记为整轮PASS。

## 修改与原因

- `internal/runtimeowner/runtime.go`
  - steady pending metadata保存TCP flags，FIN使用同一有界repair队列并保持原Seq/flags重传。
  - 新增steady `CloseWrite`：FIN使用当前 `sendNext` 并消费一个序号，ACK使用当前 `recvNext`；FIN后禁止新业务写，但读半边继续。
  - FIN按steady序列空间处理；乱序FIN先进入有界receive span，尾部record补齐后才推进FIN，避免尾部数据被关闭越过。
  - RST只在当前steady `recvNext`接受；不合格RST只回ACK。本地 `Reset` 使用当前steady Seq/ACK。
  - detached boundary之前的bootstrap重传字节不再解释成steady record；server transition detached后，属于steady边界的ACK/record/FIN/RST直接由runtimeowner处理，不再拿bootstrap `NextSeq`解释控制包。
  - DORMANT与Runtime.Close在本地transport detach前先尝试steady FIN；`laneTransport.close`本身仍只是本地资源关闭。
- `internal/runtimeentry/lifecycle.go`
  - rotation不再在fresh lane资格后立刻硬删old lane。先对old lane发steady FIN；fresh authoritative lane继续工作，old lane只做有界关闭收尾。FIN双向完成可提前退役，否则ReplacementGrace后硬收尾。
  - server replacement使用同一机制。
  - client显式Close调整顺序：runtime先发steady FIN/关闭owner，之后才关闭association与SegmentIO，避免先断carrier导致FIN无法上网。
- 定向测试
  - FIN先到、尾部record后到：不得提前peer half-close；尾部仍首次乱序/首次到达语义正常交付，随后ACK推进到FIN+1。
  - peer FIN只关闭读半边，对端仍可发送尾部数据并自行FIN。
  - RST的Seq/ACK取steady状态。
  - lifecycle real entry统计线上TCP FIN，要求automatic rotation、DORMANT、显式Close都真正经过steady FIN。

未扩大4096、未恢复严格累计ACK等洞、未修改FEC数学、未增加假业务/随机延迟/凑包等待。

## 复用来源

本原子任务没有读取或迁移新的old代码；只把当前P2已有FIN/RST/half-close语义接到active steady owner。REUSE_LEDGER无新增条目。

## Actions证据

- 前一原子SOURCE_SHA：`e3253322ac863c11636f1c127e14b77bdc6bf800`
- 前一原子Actions：<https://github.com/lly8666/wobuzhidao/actions/runs/35616392974>
- 已完成定向PASS：repository-contract、Windows active-go、Ubuntu active-go+race、OpenWrt privileged、Linux shared-TUN iptables/nft。
- 本FIN/RST原子提交在创建时尚未执行Actions；必须由本exact SHA重新验证，不能继承前一SHA结论。

## 排查结论与风险

核查确认两个真实缺口：server detached steady packet仍先进入 `ServerAssociation.HandleSegment`，FIN/RST继续参考bootstrap序列；rotation/DORMANT/Close又会直接走本地association/IO关闭。修复把steady控制所有权收回runtimeowner，并把资源detach与线上FIN分开。

rotation关闭是有界的，不让旧lane缺包阻塞fresh authoritative lane。DORMANT/进程退出要求及时释放资源，因此只保证在本地detach前尝试发steady FIN，不等待完整四次挥手。

## 下一项

本exact SHA先跑定向Actions；通过后才进入第3原子任务。下一项开始前按专项授权定向读取冻结 `old/internal/faketcp/arq.go`、`repair_horizon.go`、`adaptive_pressure.go` 及直接依赖/测试，先记录旧参数值、单位、触发条件和新架构适用性，再迁移最小有界SACK/fresh优先repair credit/RTT-RTO闭包。
