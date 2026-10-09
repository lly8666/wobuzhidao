# E4 AF_PACKET recvmmsg 准确接收FD：基于内核inode的无流量预检（2026-10-09）

工作分支 next/performance-efficiency-20261008，父HEAD 9f280f0b9e003872e8adcbf824dc0c0ffba6be6b，SOURCE=ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072冻结。前一[Foundation37873408635](https://github.com/lly8666/wobuzhidao/actions/runs/37873408635)全部SUCCESS，真实5秒合成 AF_UNIX socketpair `recvmmsg` 两次syscall + 有意60ms用户态间隔经过eBPF tracepoint验证得到 `TWO_SYSCALLS_ONE_GAP_FUNCTIONAL_ONLY`，bpftrace与fixture都退出0。可证明kernel tracepoint事件计数与用户态调用间隔正常，**不可证明Go reader goroutine停顿、更不可证明观测高PPS开销低**。

冻结产品 `internal/faketcp/raw_linux.go` 明确用AF_PACKET/SOCK_RAW独立recvFD，sendFD使用AF_INET/SOCK_RAW。仅PID筛选的tracepoint可能误采别的recvmmsg，因此新增`tools/afpacket_recv_fd_identity.py`只读内核 `/proc/PID/net/packet` 的AF_PACKET SOCK_RAW socket inode，跟同PID `/proc/PID/fd` 中 `socket:[inode]` 交集，要求**恰好一个**；验证 /proc/PID/exe basename、PID stat starttime扫描前后相同、候选FD link没有更替，最多4096 fd，任何双socket/消失/权限不满足直接fail closed。不把实际FD、inode、process argv、地址或业务正文序列化。

新增7个纯合成测试涵盖恰好一个、错socket type、多个候选、缺失inode、非socket链接、malformed table、cap；接入Foundation repository-contract。扩展已有Privileged P4 Linux隔离netns真实AF_PACKET socket烟雾测试，在当前Python进程核对以唯一的inode选中的recvFD必须就是刚创建的那一个；只打印 `WBD_LIVE_AFPACKET_FD_INODE_PASS unique_socket=1`。本轮**没启用BPF FD filter**，也没添加任何游戏业务负载、改变协议/队列/socket buffer/sysctl。

故障资格不变：EPYC9V45 profileOFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client33/server86资源FAIL，profileON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) 客户端42个socket drop于140.769–140.871s，分析器/工作流FAIL；CPU获益UNPROVEN。E7约80s下行停顿OPEN_DEFERRED，Normal/Game保护和物理未验。下一项先读取完整Foundation+真实FD预检，如果unique无法证明，标BLOCKED而不是猜FD；之后还必须单独证明FD bpf过滤和高PPS探针扰动是否足够低，不为漂亮样本重复300秒Actions。
