# E4 双FD预检的静态断言失败与最小更正（2026-10-09）

唯一工作分支 next/performance-efficiency-20261008，先前[Foundation37875503896](https://github.com/lly8666/wobuzhidao/actions/runs/37875503896) `repository-contract` FAIL（不可改写）；因依赖未过，真实双FD kernel步骤没运行。失败准确是 `test_no_user_payload_or_cross_thread_gap_claim` 把BPF文本`@previous_exit[tid]`出现次数强制断言为2，真实脚本合法使用3次（条件检查、读取做差、记录新上次退出），标准输出`AssertionError: 3 != 2`。不是新的AF_PACKET应用丢包、不是bpftrace runtime失败。

本原子修复只把脆弱出现次数断言换成三个必要的完整字符串存在断言，并要求不能有旧`@previous_exit[pid]`全进程混线程逻辑；未更改任何真正的BPF过滤、syscall进入/退出聚合、测试硬门、双socketpair fixture及目标两次/干扰两次约束。保留新完整 Foundation 验证，没有证据不声称双FD kernel已过。

产品源码仍固定 ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072，原9V45 OFF resourceFAIL d33/d86和ON resourceFAIL d42保持OPEN，CPU gain UNPROVEN，E7 80秒下行停顿OPEN_DEFERRED，不扩大接收缓冲及业务队列，不运行300s业务。
