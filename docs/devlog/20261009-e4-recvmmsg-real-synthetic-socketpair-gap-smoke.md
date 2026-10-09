# E4 recvmmsg 真正两次syscall与60ms间隔的本地功能预检（2026-10-09）

在工作分支 next/performance-efficiency-20261008，父提交 69da40b91b4a883f4f0118709801f682ac379021，产品SOURCE ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072仍冻结。前一[Foundation37873072683](https://github.com/lly8666/wobuzhidao/actions/runs/37873072683)全部SUCCESS；实际独立2秒bpftrace job artifact11590954134为 `TRACEPOINT_ATTACH_ONLY`，bpftrace exit0、stderr sha256空、真syscalls:sys_enter_recvmmsg/exit_recvmmsg可以附加。这个结果不代表事件正确捕获或足够低开销。

本轮增加独立短时**功能而非吞吐基准** `tools/testdata/e4_recvmmsg_socketpair_smoke.c`：只用本地AF_UNIX SOCK_DGRAM socketpair，等外部bpftrace `BEGIN` 附加就绪后进行2次不阻塞recvmmsg，中间在用户态明确`nanosleep(60ms)`，预期tracepoint报2个退出与恰好1个超过20ms的`上次syscall退出→下次进入`间隔；不触碰真实网络接口、Game产品、业务包或物理设备。`tools/afpacket_recvmmsg_functional.py`仅本地生成 PID 过滤的5秒trace，等待READY标记、释放短程序，超时杀掉进程并记录结构化结果，不输出pid、FD、地址或明文；内核计数、不可用、错误全显式区分。5个纯合成parser测试与额外trace script约束检查加到Foundation。只需这一步证明事件记录确实可工作，**还没有证明对高PPS业务的trace成本或不会漏报**。

保留旧9V45 4vCPU OFF lossless run37857040784 socket client33/server86 正式FAIL，与9V45 4vCPU ON run37871243581 socket client42 (140.768933–140.870761s) 正式FAIL；多OS线程总排队109.1ms不能证明Go recv goroutine本身停顿。新合成测试不触发独立300秒性能run、不调整SO_RCVBUF、queues、MTU、default100ms tick或wire，也不修改主线。CPU gain UNPROVEN、E7 ~80s下行断流OPEN_DEFERRED。

下一项读取本次新Foundation原始bpftrace功能stdout parser receipt和完整race/Windows/TUN/TPROXY回归。若两次退出/一次60ms间隔不被准确识别，保持`NOT_QUALIFIED`并调整低成本测试，不得直接向真实产品开全局监测。
