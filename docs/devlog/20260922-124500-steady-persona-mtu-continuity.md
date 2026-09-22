# 20260922-124500 建连到稳态 persona / window / MSS / MTU 连续性

## 目标与基线

执行 `docs/WEAKNET_QUALIFICATION.md` 第1节第5原子，只处理建连到steady的window/scale、MSS、实际TCP options/persona与统一MTU实际头长，不改RTO、repair horizon、4096、FEC数学、padding或业务pacing。

基线 SOURCE_SHA `ce950d4877ecb51001b6ab2517f6b43a6edadac3`：
- next-p4-steady-targeted 35681889099：6/6 PASS。
- next-foundation 35681889120：整轮PASS。非FEC 5→20→5 / 5→30→5与FEC20:20对应两档各2 seed、Windows/Linux active-go、OpenWrt/Linux privileged、load/soak、HTTPS base和历史P6 job均成功。
- 上述是第4原子回归证据，不是新增真实路径持续UDP性能资格。

## 审计结论

1. persona没有第二套状态：Linux由同一个 `RawIPv4Endpoint` 持有legacy persona，Windows由同一个 `NpcapEndpoint` 持有Windows11 persona；bootstrap和steady都调用同一endpoint的 `MarshalSegment`，因此不应把persona复制进runtimeowner。
2. client SYN固定offer WS=8，但client post-handshake `advertisedWindowLocked` 之前没有按本端scale编码。可用接收容量256KiB时，wire Window错误写成65535；peer按已协商WS解释后会呈现远大于真实有界接收容量。server已有post-handshake右移8位逻辑，client/server不连续。
3. detach/handoff只携带Seq/ACK和peer profile；steady runtimeowner把Window固定写65535，丢失了bootstrap退出时真实本端Window/scale呈现。
4. peer MSS已经从SYN/SYN-ACK进入lane MTU，但 `ClientLaneParams/ServerLaneParams` 仍允许调用方手写Tx/Rx TCP header长度。当前serializer的record-bearing steady data不携带SACK/timestamp等option，真实header恒为20B IPv4 + 20B TCP；调用方若误填32会无证据缩小record MTU。
5. SACK是协商后steady ACK/control-only option。四块SACK实际option 36B，TCP header 56B；这些包不携带TLS-like record payload，因此不能拿56B去虚假缩小data record budget，但必须由真实serializer测试和抓包校验。timestamps当前未实现，按专项要求不伪造。

## 最小实现

### Window / scale handoff

- client post-handshake在SYN-ACK带WS时，按client SYN实际offer的 `DefaultWindowScale=8` 编码本端Window；256KiB空buffer对应wire Window=1024。
- `ClientHandoff` 新增 `AdvertisedWindow/WindowScale/WindowScaleSet`，在Detach前锁内快照。
- `ServerAssociation.SteadyWindowProfile` 同样返回当前post-handshake wire Window和本端WS元数据。
- `runtimeowner.TransportConfig` 接收上述本端呈现；兼容直接core测试时未显式设置则仍默认65535。显式0 window由 `AdvertisedWindowSet` 区分，不会被默认值覆盖。
- steady data、ACK-only、FIN和RST统一使用TransportConfig的Window；Stats暴露Window/WS用于Actions测量。

### MSS / 实际头长 / options

- MSS行为不改：client Tx继续使用SYN-ACK实际peer MSS，server Tx继续使用client SYN实际MSS；对方未advertise时仍走IPv4 536 fallback。
- `faketcp` 暴露serializer事实 helper：steady IPv4 header=20、record-bearing steady TCP header=20、任意Segment实际TCP header由 `segmentOptions` 计算。
- client/server datapath admission handoff不再信任调用方估算data header；若显式提供非0且与实际20不一致，fail-closed `ErrAdmissionHandoff`。统一MTU配置写入真实20+20。
- SACK继续仅ACK/control；四块SACK的实际TCP header=56由 `MarshalSegment` 同源测试确认。没有把control-only option长度错误扣到record-bearing data budget。

### Persona

- 不新增persona字段或锁。Raw/Npcap endpoint仍是单一序列化入口；新增测试确认Windows11 persona在steady data和SACK ACK仍TTL=128，legacy仍TTL=64，且data/control实际header与helper一致。
- 不新增timestamps、随机delay、假业务或凑包等待。

## 定向测试

新增/加强：
- client握手ACK wire Window=256KiB>>8=1024，Detach快照同值且WS=8。
- server `SteadyWindowProfile` 与真实post-handshake ACK Window一致。
- runtimeowner fresh data和ACK使用handoff Window；TransportStats保留Window/WS。
- client/server lane handoff实际IPv4/TCP data header均20/20；MTU1400 + MSS1360得到outer record packet严格1400。
- 显式伪报steady data TCP header=32在client/server两侧均fail-closed。
- Windows11/legacy persona在steady data继续维持各自IP呈现；四SACK control实际TCP头56。
- 现有MSS/SACK协商、FIN/RST、no-HOL、incremental index测试继续由targeted gate覆盖。

## 风险与下一步

此提交改变client post-handshake wire Window值（65535 -> 1024，在协商WS=8且buffer空闲时），属于修复已协商scale后的编码错误，不改变256KiB真实接收上限。需要exact-SHA Linux/Windows core/race/lifecycle和privileged Actions验证。

第5原子通过后，P4五项低开销修复才完成代码闭包；下一步不是直接宣称P4/P5关闭，而是按专项第5节先搭正式client/server独立进程的Linux netns/veth/netem真实无损闭环、qdisc/capture校准，再进入18份主测。
