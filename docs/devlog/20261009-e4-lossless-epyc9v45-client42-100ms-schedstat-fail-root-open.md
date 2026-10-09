# E4 Game4 无损同型号 9V45 重现 client socket drop 42 次，100ms 见证但根因仍开放（2026-10-09）

工作分支 `next/performance-efficiency-20261008`；本提交以 `2c500ad58a126ee47dd0a29b737dc4253a3a52fc` 为父，产品正式 SOURCE 仍是 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，本轮是**证据回执，不改产品、helper、buffer、queue、MTU、wire、主线或物理**。一次合法独立 [Game4诊断run37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581)，job113629611881，完整压测artifact11590847194。同期 [Foundation37871243592](https://github.com/lly8666/wobuzhidao/actions/runs/37871243592)全SUCCESS。

## 不可抹除的正式结论

**整条workflow FAIL**；300s+3s实际client/server业务、100ms socket/OS线程探针audit、只读same-run witness、CPU ledger、artifact upload均SUCCESS，唯**正式应用分析器**本身 outcome FAILURE：`RESOURCE_AUDIT_WARNINGS, LOCAL_SOCKET_DROP`。它们是产品资源资格失败，不能因为真实业务全交付、P99良好而改写为PASS。4 client /4 server active及physical；两向 UDP app datagrams各103062、丢失0，独立C2S probe1500/1500 p99=611.515ms、S2C1495/1495 p99=612.480ms，HTTPS10/10验证、HTTP+HTTPS20/20。

HOST是 **AMD EPYC 9V45 4vCPU**，和上次原始OFF FAIL的型号相同；未测出cgroup CPU配额、steal max0，CPU PSI 10s avg10 max36.03%，capacity_limited_evidenced=false。CPU client88.24+server87.22=175.46 CPU-s是**ON plus profile/extra sidecar only**，不得与原始OFF166.97 CPU-s或9V74 profileON的结果比较。正式所测源及seed/Game/MTU/FEC/网络配置一致。

## 新100ms证据

- client AF_PACKET socket skmem.d **0→42**落在真实business_start之后 **140.768933–140.870761s**的保守边界。旧OFF9V45 client d33发生于142.0525–143.0525s（另server d86于86.0525–87.0526s）；新的服务端d0不代表旧server问题解决。两侧100ms `ss` 3150/3150正常、无缺测，OS schedstat采样client3147完整、2线程population变化、1基线，server3148完整、1部分、1基线。
- 事件上下文里匹配OS线程**累计可运行排队等待下界109.104819ms**（跨2个100ms观测），重叠的1秒正式业务raw receive仍有4340次calls、12658条messages，cgroup `nr_throttled_delta=0`；说明该1秒仍在读包，并存在OS可运行调度排队，**不表示同一个Go goroutine连停109ms、不表示recvmmsg系统调用耗时109ms或cgroup容量饱和**。更不能断言内核socket为什么丢。
- 客户端socket `rmem`在d前后的两个100ms snapshot均0，整个100ms观察到峰值354304/1048576 bytes；1s原resource峰244736/1048576。不能排除100ms之间几十毫秒瞬间满socket。原始OFF在失败窗口的两次1s `rmem`也均0且cgroup throttled增量0，不能解释为资源本来足够。
- 运行时上游client mux四条队列峰值699/668/706/627，cap4096各自、无overflow；服务端Game ingress分片峰值772/752/703/646 cap960各自、0 oldest drop/reject；server共享ready peak2800/4096、overflow0；物理链路netem接口drop0。这些**不排除更早在内核packet socket发生过的丢包**，但说明没有观测到应用有界队列自身丢弃。

同一套见证器、resource和正式summary哈希锁定于 [结构化artifact回执](../evidence/e4-game4-lossless-client42-epyc9v45-100ms-schedstat-run37871243581.json)。`manifest.json` SHA256=17e2656f8460011fd6e0e843881c71f7d31197007e0c3005592fcd04f52063e1, `summary.json` SHA256=f305992e8b5a279936db1350df1c390d4e7f04d9c07a800ca1c7627dc771d1a3, `e4-packet-drop-witness.json` SHA256=40eabffc55d87771f32ed8370b158ad484b7215b5cb271cb1f2dcd5488374821，保留官方Action link。

## 决策与下一项

**停止额外300s Game4复抽**，不挑能跑绿的9V74来关闭9V45 FAIL，也不扩大packet socket SO_RCVBUF、ready/shard mux队列或改变raw batch填充等待。不引入尚未证明的专用线程/优先级修改。在任何新产品候选前先拟定性能扰动可控的单测限定 `recvmmsg` syscall开始/结束/间隙与`skmem.d`的同一monotonic时间线；若不可在冻结SOURCE下获得且诊断将扰动产能，记录BLOCKED rather than改源码掩盖历史资格。E4 root OPEN、CPU收益 UNPROVEN、Normal/Game2其它protector NOT_RUN、E7约80秒下行停顿 OPEN_DEFERRED、E6/P6/physical NOT_RUN。
