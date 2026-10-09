# E4 Linux AF_PACKET 实际 `ss` 格式最小内核预检（2026-10-09）

本提交只在 `next/performance-efficiency-20261008`，父 HEAD `74134df9db41d3ac78dc739559567fd333d318d3`，产品 SOURCE 固定 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，不改产品、内存队列、socket、netem、MTU、协议、样本配置，也不碰规范主线/物理机。

恢复时读取远端 Actions 后确认 `37866805754` 完整 SUCCESS：历史业务时间轴和100ms数值 `ss` 解析两套合成回归分别7/7及4/4、Linux/Windows、race、ACK、TUN/TPROXY/kernel fallback全绿。仍不能用纯字符串合成测试代替真实内核行为，因为 `37865738583` 前次300s的两个100ms观测器各3150行均 `ss_ok=false`，最终工作流 FAIL；它的原始业务 analyzer PASS_SCOPED_ACTIONS 不能覆盖这个 FAIL。

新增 `tools/test_afpacket_socket_probe_live.py` 作为**功能预检而不是性能样本**：既有 Foundation 的 OpenWrt privileged job 在已安装 iproute2 后新建唯一临时 Linux netns，并启用 lo；Python在其中只创建一个真正 AF_PACKET/SOCK_RAW/ETH_P_IP socket，运行一次本机 `ss -0 -a -m -n`，将结果交给正式 `afpacket_socket_probe.numeric_row()`，验证单socket、rb>0、d0、调用 monotonic bracket，然后自行关闭socket，shell trap 删除netns。日志只打印通过标志，不包含完整ss输出、地址、密钥或报文；完全没有产生业务包、配置WAN、启动第二性能job或300s测量。这个预检使之前两行合成/同行格式脱离真实kernel的风险有独立、廉价、可反复验证的门。

原9V45 Game4 lossless profileOFF run37857040784 发生client33/server86 `skmem.d`，原始 FAIL RESOURCE_AUDIT_WARNINGS/LOCAL_SOCKET_DROP **保持开放**。9V74健康的profileON samples 不构成修复。CPU收益UNPROVEN，E7约80秒下行中断OPEN_DEFERRED，正式 Game2/Normal等保护未放行。

新工作流尚未运行，结果为 PENDING_FOUNDATION；下一步必须读真实 Actions 及步骤结果，不可继承上次Foundation资格。
