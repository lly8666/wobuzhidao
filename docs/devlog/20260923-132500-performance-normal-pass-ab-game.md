# 20260923-132500 Normal10首次全PASS与A/B+Game4验证启动

## 阶段与结论边界

本轮对应 `docs/WEAKNET_QUALIFICATION.md` 第9.2–9.4节。功能生命周期仍保持36/36 COMPLETE；本轮没有改生命周期或队列所有权。性能主线取得第一个目标负载无损PASS，但整体性能专项**尚未完成**，不会用单个lossless Normal样本替代Game4、严格弱网矩阵或长测。

## raw receive修复最终 Actions

SOURCE_SHA `a924b7c9853e1d280cb7a7062dc46ccd30039b99`。

- `next-foundation` run 35816817929 SUCCESS。
- `next-lifecycle` run 35816817993 SUCCESS。
- `next-p4-steady-targeted` run 35816817923 SUCCESS。
- `next-realpath-calibration` run 35816817901 SUCCESS。
- `next-strict-harness-preflight` run 35816818035 SUCCESS。
- `next-performance-recovery` run **35816817908**：attempt1普通core通过，但full `-race` 中 `TestLifecycleEntryGameReplacementDormantWakeKeepsStableLease` 仅出现一次3.05s状态等待超时，没有race detector报告，Normal job因此未启动。没有为此修改生命周期或放宽门槛。
- 同一run的attempt2重跑失败job：core/race job **107042968647 PASS**；Normal10 lossless job **107043226145 PASS**。artifact **10732381400**，digest `sha256:8451f705dd9be990d391b221f4c077b671215a956e21f512f567b3ed7c9fc6e2`。

Normal1 lane、FEC20:20、每方向10Mbps、lossless、seed601 的分类首次全部为 PASS：`CAPTURE/CORRECTNESS/ENVIRONMENT/INPUT_VALIDITY/PERFORMANCE` 均PASS。C2S和S2C pre/stress/post实际发送与eventual goodput均约10Mbps，业务byte/packet loss均0%。client/server AF_PACKET `ss_packet` drops均0，所有UDP/raw/packet socket drop也为0，接口drop delta为0。

资源侧不支持“runner太慢”的结论：120s窗口client/server进程CPU分别约76.03s/76.56s；server工作分散在多个OS thread，AF_PACKET rmem峰值约199552/1048576（0.1903），不是靠扩大socket buffer获得PASS。

## 修复前后同seed变化

相对O(1) qualification后的 `7550844af73ca07481a932d17d6793ccb23ae05a`，raw scratch复用+owned payload把Normal10从CAPACITY_LIMITED推进到全PASS。最终server读取 **1,579,582** 个包；handler累计47.80s，均值约 **30.3us/包**；readCh handoff均值约 **29.6us/包**；queue age均值约54.7us。此前755样本server reads 269,830、handler均值0.359ms、AF_PACKET drops631,423，C2S/S2C仅约1.6–1.8/5.9–6.0Mbps。

该修复没有改变队列容量、注入率、FEC档位、Game副本数、MTU、repair horizon、4096 recovery、wire、休眠/PeerFIN/blackhole/lease语义。

## Normal方向字节账本解释

严格发生器按UDP payload **64/256/1200B等次数循环**。进入TUN后IPv4+UDP增加28B，因此三种inner长度为92/284/1228B。

Normal C2S使用server record limit1250：TLS-like固定开销31B、FEC头56B，LINK frame MTU=1163B、fragment payload预算=1143B，所以1228B inner packet分成1163B与105B两个LINK frame。S2C使用client record limit1300，对应LINK frame MTU=1213B、fragment payload1193B，1228B分成1213B与55B。于是每3个业务包形成4个FEC source frame。

每20个source（5个业务尺寸循环）实际LINK payload合计两方向都只有8220B，但FEC20:20 parity按该block最大source shard宽度生成：C2S最大1163B，S2C最大1213B。因此full block中：
- C2S source FEC wire约9340B；parity约24380B；
- S2C source FEC wire约9340B；parity约25380B；
- 再叠加40个TLS-like records各31B、IPv4/TCP header和纯ACK/control。

这解释了当前完整送达后outer/app比回到 C2S **5.09134174x**、S2C **5.22305748x**。它不是“5倍重传”：最终server侧S2C实测FEC encoder `source_shards=394858/source_bytes=195022224`、`parity_shards=394858/parity_bytes=501074802`，transport `Retransmitted=0`、repair attempts=0、padding=0。S2C仅这两项FEC wire就约696.1MB，对150MB原始业务已是约4.64x；TLS-like和外层TCP/IP/ACK继续补足到5.223x。混合小包使parity按块内最大shard补齐是主要确定成本，不能误报为实现错误，也不能通过降FEC档位规避验收。

## 当前验证动作

新增独立 `next-performance-ab-game`，不触碰主 `next-strict-weaknet` 路径，因此不会提前启动18样本矩阵：

1. 两个独占runner分别做同runner顺序 **A/B** 与 **B/A**；A固定 `7550844a`，B固定 `a924b7c9`，同为Normal10/FEC20:20/lossless/seed631。每个runner顺序执行两个版本，输出goodput、AF_PACKET drops、CPU、GC/TotalAlloc、handler/readCh与成本比，排除运行顺序影响。
2. 另一个独占runner跑 **Game4 / lanes=4 / FEC20:20 / lossless / 每方向逻辑业务总量3Mbps**，seed641。发生器只注入一份3Mbps逻辑业务，Game owner内部复制到4 lane，绝不是每lane各3Mbps。
3. Game validator打印完整 `cost` 与 `recovery_accounting`，逐方向核对FEC source/parity、Game logical/copy/replication extra、repair、health、padding、ACK/control/outer成本。

若B在A/B与B/A中都稳定保持目标PASS且Game4逻辑3Mbps通过，下一步才运行原18份严格弱网矩阵，再做目标负载长测；否则回到最早失败边界继续单原因修复。
