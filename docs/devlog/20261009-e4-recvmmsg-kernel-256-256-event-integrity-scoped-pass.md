# E4 PID+唯一接收FD 的256次内核事件计数完整性：非产品限定通过（2026-10-09）

唯一工作分支 `next/performance-efficiency-20261008`，精确父HEAD `f76b1bb786b9e88fce19d2cb88cff8c62cd1156d`；正式产品 SOURCE 始终 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，本提交仅完整证据、STATUS及devlog，无产品、buffer、queue、FEC、MTU、wire、主线或物理改动。

真实独立 [Foundation37876138265](https://github.com/lly8666/wobuzhidao/actions/runs/37876138265) 全部 **SUCCESS**（仓库八套synthetic 7/4/6/8/7/10/12/7，Linux/Windows、race、TPROXY、TUN两后端、内核回退及短BPF步均通过）。受控Linux hosted runner中真正跑了局部两组 AF_UNIX SOCK_DGRAM 的单进程、单线程测试，固定目标FD30进行了**256次recvmmsg**，同PID其他FD31也进行了**256次**；内核tracepoint入口有严格 `pid==TGID && args.fd==30` 筛选，出口只为同OS线程已匹配目标入口的调用计数。实际输出 `TARGET_256_EVENTS_MATCHED_FUNCTIONAL_ONLY` 代表目标enter/exits各256且没有unpaired；fixture与bpftrace均退出0。它不测吞吐，也不要求在任意系统调度条件下无>20ms gap。

同Action里每次2秒attach、原两次recvmmsg/60ms gap、双FD两目标两干扰fixture都仍绿。GitHub artifact **11592840561**原始四个JSON各自SHA256已写 [结构化证据](../evidence/e4-recvmmsg-256-target-256-decoy-in-kernel-foundation-success-20261009.json)，而不是用仓库本身描述来替代真Action。原始[37875503896](https://github.com/lly8666/wobuzhidao/actions/runs/37875503896)的纯synthetic文本文字断言失败及修复提交仍保留，不改历史结果。

**严格局限**：一条5秒/每侧256调用的本地AF_UNIX fixture只是中小规模事件计数一致性；正式Game4两个产品可能累计百万次recvmmsg，eBPF的附加开销、内核BPF map峰值、可能错过的高速事件、宿主PSI/steal、Go goroutine跨OS线程迁移及FD的实际重用概率都尚无合格证据。不能因为这次pass就在300秒产品重采中启用BPF，不能宣称收包线程stall或host资源不足。**下一项**只能先按 `docs/PERFORMANCE_EFFICIENCY_PLAN.md` 刚加入的独立资源层单样本协议设计和执行不混测的trace overhead/lost-event校准，缺少资源重叠标NOT_CALIBRATED，不拿跨CPU PassMark强行折算。

原9V45 profileOFF Game4 lossless [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client AF_PACKET skmem.d33/server86以及同CPU型号profileON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client42仍是正式资源FAIL，OS线程聚合排队不是接收goroutine因果。其它Normal/Game protector/E6/P6/物理都不能被工具PASS放行，CPU收益UNPROVEN，E7约80秒下行中断OPEN_DEFERRED。
