# E4 真实 ss 同行格式回归用例修复（2026-10-09）

仅工作分支 next/performance-efficiency-20261008，父提交 b0b707eb95139c502e91f2bf7dffaf2a25c15447；产品源码固定 ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072。

前次 b0b707 修复 AF_PACKET 只读解析器能接受 iproute2 真实格式：p_raw 和 skmem() 同一行。但是新合成回归用例中的 Python 字符串错误使用了双反斜线+n，使得实际字符串没有换行符。GitHub Foundation 37866550583 的 repository-contract FAIL：test_real_iproute2_inline_packet_socket_format，真实日志 AssertionError None != {r:0,rb:1048576,d:33}。必须保留这一CI失败而不能误称修复已验收。

本提交仅修正 tools/test_packet_socket_drop_forensics.py 和 tools/test_afpacket_socket_probe.py 中字符串的 newline 转义，确保测试构造的首行 header 和真实一行 p_raw skmem 之间确有换行。parser 实现、工作流、样本配置、源程序、MTU、缓冲和队列均不变。更新 STATUS 与新devlog；新 Foundation 仍待正式验收。

前次唯一300s profiler ON run37865738583 原始业务 analyzer PASS_SCOPED_ACTIONS，硬性workflow FAIL（100ms探针所有3150/side读数因旧解析器输出null）不变；不能倒填raw socket读数、不再为修工具重复采300s。同源旧9V45 profileOFF run37857040784 socket drop client33/server86 FAIL OPEN，CPU收益 UNPROVEN，E7约80秒下行中断 OPEN_DEFERRED，E6/P6/物理 NOT_RUN。

下一步读取完整Foundation实际结果，任何产品优化须先确定socket drop根因和可靠测量。
