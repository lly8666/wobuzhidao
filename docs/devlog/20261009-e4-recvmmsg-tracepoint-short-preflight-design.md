# E4 recvmmsg两秒内核tracepoint可行性预检（2026-10-09）

只在工作分支 next/performance-efficiency-20261008，父HEAD 42990db57968f447eaf8ef295d4d8e88d3d659f9，正式产品SOURCE ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072不变。此前[Foundation37872138573](https://github.com/lly8666/wobuzhidao/actions/runs/37872138573)已核实完整SUCCESS，但9V45同型号两次AF_PACKET socket drops（profileOFF run37857040784 client33/server86、profileON run37871243581 client42 140.768933–140.870761s）仍均为正式FAIL。产品1秒raw_io及100ms OS多线程聚合无法提供精准recvmmsg进出时钟；即使跨线程累计有109.1ms OS runnable等待，也非同一个Go reader goroutine阻塞证据。

新增 tools/afpacket_recvmmsg_tracepoint.py，只在Foundation中运行**两秒**PID2147483647（不存在）的bpftrace syscall enter/exit tracepoint附加能力测试；隔离产品及全部业务流量。设计中的长间隔阈值20ms、100ms单调时间桶由eBPF内部聚合，没有任何每调用一行stdout或payload/FD/地址/TID输出；不会使用ptrace、修改perf_event_paranoid/sysctl或mount全局tracefs。长期运行的真实调度开销、事件丢失、对目标socket FD的区分以及内核/Go因果都**没有测量**，必须将真实成功状态限定为TRACEPOINT_ATTACH_ONLY。

新tools/test_afpacket_recvmmsg_tracepoint.py包含六个纯Python测试验证PID及时间上限、tracepoint数、20ms/100ms界限、禁止逐事件打印/网络内容和map大小；在Foundation repository-contract执行。另加独立非业务e4-recvmmsg-tracepoint-capability job，尽力安装bpftrace，在无需弱网压测的GitHub runner运行两秒，上传JSON receipt；若kernel不允许或cloud perf权限阻断，JSON为UNSUPPORTED而非编造有效数据，也不扩大权限。

本次提交**不改**工具生成器/单样本config、产品Go、游戏queue/原socket buffer、MTU/wire或规范主线和物理机器；新job不产生另一次产品性能采样。旧9V45 OFF及新9V45 ON正经资源FAIL仍OPEN，CPU收益UNPROVEN，E7 ~80s下行中断OPEN_DEFERRED。

下一项：读取真实Foundation结果和独立capability receipt。只能在内核确实允许附加后，另行制定单条观测的最大开销与丢事件验证，并避免把慢recvmmsg自然等第一包错归因到接收线程停顿。
