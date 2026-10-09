# E4 FD扫描器的临时目录句柄关闭误报修复（2026-10-09）

仅 `next/performance-efficiency-20261008`；本原子提交不是产品改动，产品SOURCE ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072冻结。上一live kernel FD识别[Foundation37873762122](https://github.com/lly8666/wobuzhidao/actions/runs/37873762122) 的p4-openwrt-tproxy-privileged job中 `Live Linux AF_PACKET ss parser smoke in isolated netns (no traffic)` **FAIL**。原始日志：`os.readlink('/proc/5472/fd/4')` FileNotFoundError，经本helper转换为`FD churn during packet socket selection`。

该报错来自**Python自己的`Path.iterdir()`已完成并关闭的临时枚举FD**：先把路径全列表化，再单独逐项调用readlink时，枚举器本身的fd节点已被内核删除，不能据此推断产品进程socket中途换代。改为`with os.scandir(root/'fd') as descriptors`：遍历时就在仍打开的目录迭代器中readlink，其他线程真实关闭FD依旧fail closed；同时保留进程exe校验、starttime前后相等、候选AF_PACKET inode唯一、最终链接不变与最多4096 FD硬门。不扩大资源，不容忍真正的失踪或多socket。

此修复只涉及`tools/afpacket_recv_fd_identity.py` + docs/STATUS/evidence/devlog，现有真实同一netns AF_PACKET烟雾验收继续作为Foundation gate，等待新Action实际验证，不为工具bug重新开启Game4 300秒压测。之前5秒AF_UNIX recvmmsg功能smoke [Foundation37873408635](https://github.com/lly8666/wobuzhidao/actions/runs/37873408635) SUCCESS、2次syscall和1次60ms长间隔保留，但还不是正式Go receiver的FD/CPU负载证据。

原9V45 profileOFF lossless socket d33/d86及profileON同型号d42正式资源FAIL、E4 ROOT OPEN保持，E7~80秒下行停顿OPEN_DEFERRED，CPU gain UNPROVEN。
