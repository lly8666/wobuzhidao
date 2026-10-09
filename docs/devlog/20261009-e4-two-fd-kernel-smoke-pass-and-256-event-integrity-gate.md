# E4 双FD过滤已绿，继续256+256真实syscall计数完整性非性能预检（2026-10-09）

仅分支 `next/performance-efficiency-20261008`，父HEAD `5f9544ba5afd4bcbea845319a02ed5911c182120`，冻结产品SOURCE `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`。最新[Foundation37875639024](https://github.com/lly8666/wobuzhidao/actions/runs/37875639024) **完整SUCCESS**，此前 [37875503896](https://github.com/lly8666/wobuzhidao/actions/runs/37875503896) repository-contract有一条静态字符出现次数assert误报FAIL，不能抹掉。现在一条完整Linux云runner双FD对抗性fixture验证了 `bpftrace` 的 `args.fd == 30` 和 target-only entry→OS-thread exit pairing：**fd30目标两次、fd31干扰两次**，只报目标 `@enters=2/@exits=2` 和1次设计的60ms长间隔，bpftrace/fixture退出码均0，CI由实际结构化状态精确fail closed。真实 artifact 11592670187含3个小JSON和各自SHA256见本次结构化证据。

接下来新增 `tools/testdata/e4_recvmmsg_two_fd_256_events.c`：在同一进程仅本地AF_UNIX socketpair、没有产品、网卡或外层性能业务，固定在fd30非阻塞完成256次recvmmsg，fd31也完成256次，使BPF以进程加目标FD过滤后只有目标 `@enters=@exits=256`、`unpaired=0`才PASS。这里不强制要求跨次gap=0，因为GitHub调度中可有自然停顿；只验确切事件计数，不把它冒充吞吐测试。纯parser正/反例扩展，5秒独立Foundation helper步骤，允许真正不支持内核时记录UNSUPPORTED但已验可附加环境中真实结果不达标必须FAIL。所有输出保留JSON/artifact，不发完整fd、PID、业务包。

正式Game4 9V45 profileOFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client skmem.d33/server86 与同型号profileON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client42均原始业务资源FAIL。无论256事件完整性门成功与否，高频BPF附加的延迟/CPU/RSS扰动及跨宿主差异**尚未测量**，因此继续不准在300秒产品诊断中启用tracepoint、不提高socket buffer和任何FIFO大小、不声称CPU增益。E4根因OPEN、E7~80秒下行停顿OPEN_DEFERRED、其它保护门未放行。

独立测量协议新增至 `docs/PERFORMANCE_EFFICIENCY_PLAN.md` 附录：一次一run/一个测量job的独立合成trace-on/off样本、严格同资源层/分层统计、源和内核事件一致、预设扰动预算；若无法观测event loss或匹配资源层就维持NOT_VALIDATED，不能把低频fixture结果映射到正式产品 CPU 成绩。
