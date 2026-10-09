# Foundation 同时门控两个 AF_PACKET 数值解析器（2026-10-09）

只在分支 `next/performance-efficiency-20261008`，父HEAD `a2adbbf9bcc51786186455e42b944116e60ff941`，正式产品源码仍为 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`。上一提交 a2adbbf 的 repository-contract job 已 SUCCESS，证明历史时间轴只读解析器在真实 `p_raw ... skmem:(...)` 同行格式、虚构旧双行格式以及缺测/multi-socket/drop counter reset 上合格。注意之前 b0b707 引入的错误 synthetic 字符串已真实修复，而旧 Foundation 37866550583 FAIL 的记录并未被覆盖。

发现 Foundation 当前只跑 `test_packet_socket_drop_forensics.py`，而新100ms现场观测器使用独立的 `test_afpacket_socket_probe.py`，该文件此前仅在很贵的 fullstack 300s 测量工作流前置执行。因此将无特权、无产品编译开销的两套合成解析测试与 `py_compile` 都纳入 Foundation repository-contract：源文件 `tools/packet_socket_drop_forensics.py` 和 `tools/afpacket_socket_probe.py`，包括真实同行格式、中断不可用、禁止多socket混算；这能未来在正常commit即刻反馈，不需要启动300s性能任务来验一个正则。

本原子提交只修改 Foundation workflow、STATUS 与新devlog；没有修改诊断器业务逻辑、正式校验器、网络、运行时、队列/缓冲、样本配置、官方SOURCE、主线或物理设备。不会触发任何性能action。

前次 profiled Game4 lossless [run37865738583](https://github.com/lly8666/wobuzhidao/actions/runs/37865738583) 最终 FAIL 是100ms探针 `ss` 格式解析错误（两个端点各3150样本全部null；原始业务 analyzer PASS_SCOPED_ACTIONS），不能把其100ms数值事后编造出来。原始9V45 OFF [run37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) socket d33/d86 FAIL 保持；9V74 ON干净宿主不能证明旧根因已消失。CPU获益 UNPROVEN，E7约80s下行中断 OPEN_DEFERRED，Normal/Game其他性能保护项、E6/P6和物理资格未放行。

下一步核对本次 Foundation workflow 真正执行双suite及跨平台回归，任何失败先记录后修复；不因工具缺陷重复抽样找幸运runner。
