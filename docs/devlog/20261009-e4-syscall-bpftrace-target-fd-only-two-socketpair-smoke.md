# E4 recvmmsg 同PID双FD对抗性tracepoint过滤预检（2026-10-09）

只在工作分支 `next/performance-efficiency-20261008`，父HEAD `5c291c7f039daebd0b306c0b70cd8bdd75c6fcee`，产品SOURCE `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`冻结。上一[Foundation37874273922](https://github.com/lly8666/wobuzhidao/actions/runs/37874273922)已确认完整SUCCESS。此前内核实际两秒tracepoint可附加、AF_UNIX两次recvmmsg出口+60ms一次gap、/proc inode对真实Linux AF_PACKET/SOCK_RAW接收FD的唯一解析已在三个彼此独立的非业务Foundation资格中完成。但现有tracepoint只筛pid，不筛真正接收FD，在多socket进程会产生错误归因。

此次在 `tools/afpacket_recvmmsg_tracepoint.py` 的小型5秒程序新增**可选**`recv_fd`整数参数：`sys_enter_recvmmsg`同时筛 `pid == TGID && args.fd == expected_fd`，进入后只在当前OS线程`tid`埋`@entered_ns[tid]`并记`@enters`，`sys_exit_recvmmsg`只给有匹配entry的线程累加`@exits`。相邻两次系统调用的间隔改为**每一OS线程**`@previous_exit[tid]`，不能把不同线程的退出/进入当作一个goroutine间隔。超过20ms分别为内核内部等待与两个matched calls间空白；内核BPF内部仅做100ms时间桶聚合，不逐次printf，不上传任何PID/FD/业务字节。

新增极短纯功能fixture `tools/testdata/e4_recvmmsg_two_fd_smoke.c`：单一临时进程建立两组**本地AF_UNIX** socketpair，固定把两只接收FD映射为30、31；在BPF BEGIN就绪之后按顺序 `fd30 → fd31 → nanosleep(60ms) → fd31 → fd30` 发起四次非阻塞recvmmsg。目标端恰好2次，非目标端2次。`tools/afpacket_recvmmsg_functional.py --target-fd 30` 必须拿到同一内核tracepoint的 `@enters=2、@exits=2、一次长间隔、unpaired=0`，否则报告INCONCLUSIVE并使Foundation这一步**失败**；原单FD fixture也强化为enter/exit双向配对。纯合成测试增加FD值/线程配对及新parser失败例，同时新增5秒独立non-business step，编译fixture，不启用正式产品或网络网卡，也不运行业务压测。

该预检只验证在Linux hosted内核的**这条合成FD**上可以分辨目标与噪声FD。它还不能证明真实产品`recvmmsg`调用耗时、BPF样本丢失为0、观察高PPS的额外CPU/延迟开销足够小；也不能证明Go接收goroutine何时被调度，Go goroutine可能在OS线程之间迁移。本次不改产品Go、SO_RCVBUF、shard/ready/mux队列、MTU/FEC/wire、规范主线、物理环境或单性能sample配置。新增300s sample数0。

原始9V45 profileOFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client d33/server d86，以及新9V45 profileON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client d42（业务140.769–140.871s）均为正式资源FAIL，不随工具门通过而修改。E4 root OPEN，CPU gain UNPROVEN，E7约80秒下行停顿OPEN_DEFERRED。

下一项只读取新Foundation和真实双FD receipt，若成功，才继续评估独立的短时事件漏报/观察开销方法，而不是在一条300秒Game4里混跑A/B或更换宿主抽到绿色为止。
