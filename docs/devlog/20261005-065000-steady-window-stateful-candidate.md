# 20261005-065000 稳态窗口与有状态中间路由候选

## 本轮目标和阶段

用户明确要求持续测试、修复不停。开始HEAD b055280e620376c040608c8d8b521e70adbd1012，目标next/tlslike-dataplane；已建立持续目标，优先D01原生中断，再补五分钟矩阵。现部署仍固定SOURCE6181db6，产品候选不能未过Actions就上实机。

## 修改与原因

faketcp.steadyAdvertisedWindow先前将建连256KiB缓冲按WS8折算成window1024，稳态有效窗口仅262144B，却允许FEC扩大的fresh流量无ACK发送门。候选将仅稳态报头window field改65535（有WS8时16776960B，无WS时65535B）；bootstrap真实容量/零窗口不变，WS协商不变，未分配额外内存，4096 shadow/FEC/无HOL/有限修复均未动。客户端handoff和服务端SteadyWindowProfile共用这一现函数。更新测试明确断言bootstrap仍小窗口、steady独立满field及满bootstrap缓冲不冻结steady；不是放宽正确性门。

strict_weaknet workflow新增stateful_middlebox（默认false）与可选40-hex product_source_sha；单run仍一源码/配置/seed/样本，不做matrix/A-B。隔离router启用非liberal conntrack并DROP INVALID，只作用测试443。原脚本和 analyzer保持目标10M/3M/FEC20:20/300ms每方向原门，记录前后规则计数、源SHA与harness SHA分开。可用同一新harness另开独立run测固定旧SOURCE，从而验证真实中间路由是否能复现；不是算法性能选型。平台正确性/race仍由push CI及定向门执行。

测试助手新增连续Linux/Windows outer header counts（不存payload），Windows独立Npcap observer不冒充产品handle stats；API布局与统计语义核对[Npcap官方说明](https://npcap.com/guide/npcap-api.html)和[libpcap头文件](https://github.com/the-tcpdump-group/libpcap/blob/master/pcap/pcap.h)，Windows pcap_stat按六uint布局。Linux观察双向需ETH_P_ALL；仅ETH_P_IP在本拓扑看不到outgoing，因此首份仅C2S不能证明server没发。业务助手增加1s live/interval；诊断写文件失败独立计数，不得打断实际发送。

## 复用来源

复用当前runtime/faketcp窗口handoff、strict原真实raw+TPROXY+TUN fixture和P2 PCAP parser。无old移植、无新协议/配置开关。

## Actions证据

本提交候选尚未取得资格，product_state=IN_PROGRESS；最近已验产品SOURCE仍6181db6。push后执行基础/定向/race、独立Normal5205、Game5205、stateful旧SOURCE独立样本及candidate独立样本，原始结果失败同样保留。当前不得部署候选或称D01已修好。打包与实机复验等上述门通过。

## 原生诊断与失败保留

两份新增固定6181诊断复跑很早看到流量中断，但均未跑满300s，不能计完成项：seed1105 live File.Replace与Get-Content共享权限冲突使负载助手退出，改为计数忽略诊断IOException；seed1106随后Windows UDP Send InvalidArgument。第二份客户端实际退出1，日志明确logicaltunnel stale transport lane generation lane1 got1/current2，前后网络owned清理0。这不是普通DNS查询失败，也不能把候选window直接称根因修复。

第一份早期计数：Windows observer持续C2S超过21万packet，而ARM本tupleC2S停在约5253；ARM raw emit仍持续，接收 record/path errors0、PeerFIN/PeerRST false，decoder没有heavy backlog。异常有界capture只见大量S2C继续发、window1024，未见C2S；说明丢失发生在包到达对端解析之前，外部TCP窗口检查是强假设，但尚未定位具体设备。两个观测socket计数与产品接收计数分开。第二份在自动新generation附近致命退出又揭示生成旧密文到SendNormal之间的竞态，需要下一独立修复，不靠吞错误、重发原包或让业务等TLS候选。

临时server诊断配置在远端保护副本（无凭据入库），复跑后恢复原配置并重启；client正常退出或fatal后owned清理0，测试任务/lo地址/observer helpers回收。原始pcap现场分析后删；保留本地round3/round4小计数与失败。该轮目录不是完整资格回执，后续收口摘要加入真实失败范围。

## 问题、排查与风险

扩大外观窗口不是无限高BDP/所有有状态设备通过保证；无WS仍64KiB。需stateful Action和同源native复验。尚未修改发送路径竞态；GUI/分流/其他五分钟门仍NOT_RUN。不得因此前ACTIONS_PASS给候选继承资格。

## 下一项原子任务

先验本窗口候选；并准备source packet到wire emission的有界promotion fence，候选TLS构造不持逐包阻塞锁，generation旧密文仍不能借新key重发。用定向确定性换代竞态测试而非依赖高负载碰运气。每阶段独立Action5205基本过门后继续，最终配套P6包再重复原生D01。
