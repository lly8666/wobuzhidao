# 原生 FEC 配置验收与 MTU 修复范围

## 本轮目标和阶段

在分支 next/tlslike-dataplane、起始文档 HEAD cb5ba95 上继续 P7。产品固定 02465264b37fb61c4d95b190fbfe8322e4912351，Windows/ARM 配套部署；负载助手固定 b393bc30bcbeff8a6e4922243820649cec31157c。收口两条 M03 真实结果，继续尚未执行的 FEC 配置工况；不修改 Go、协议、重传窗口、恢复期限或缓冲。

## 修改与原因

更新唯一 STATUS、方案和证据入口，移除当前段落里过期的“短预检运行/38样本”。原始失败及其作用范围保留。M03 稀疏大包实验与正式持续负载资格分开，不将最终回包或全绿 Actions 冒充所有包及时。

此前 Linux 内层 TUN 与外层 MTU 分离的修复已经通过定向 Actions 并部署：内层 9000、外层配置 1400。独立 CLI 只读回执确认客户端接收 record limit 1300。最大 UDP 的内层分片 48→8，推导 systematic 95→59；9000 字节 IP 包 systematic 13→8。推导与两样本的实际 encoder 计数吻合，其他少量 TUN 输入单列。无逐包新增工作，不据此宣称 CPU 或 p99 已全面改善。

## 复用来源

沿用已资格化的原生负载、DNS、Npcap、资源及限量抓包助手；WSL 只控制远端和整理证据。无 old 提取，无新协议或产品配置。

## Actions 证据

产品 024 的 17 项定向 run 全 PASS，详见 docs/evidence/server-inner-mtu-0246526-20261006.json：11 正确性门，五独立严格性能样本，P6 三目标包。18 个匹配 p95/p99 增量检查全通过，p99 最大增量 11.210221ms。Normal 最低阶段 9.999261867Mbps，Game 最低 2.999552Mbps。每性能 Action 一条。

文档 cb5 的基础 37437054262、targeted 37437054339、preflight 37437054298、GUI 37437054377 全 PASS；不把文档 HEAD 冒充编译发布 SOURCE。当前产品全 70 配置、18 条严格样本及 1800 秒长测仍 NOT_RUN，旧版本资格不继承。

## 问题、排查与风险

两条 M03 完整有效观察：seed1438 2046 发/2045 收，最大 UDP 丢 1；Linux 3414/Windows 3412 提交行、缺该包两个 IP 片。seed1439 2047 全收，却有一个 9000 字节 DF 包迟到 1278.2255ms；Linux/Windows 各 3415 行。两条元数据丢弃、内容损坏、重复交付和 server raw socket drop 都为零。1439 的迟到包回程只有一个 IP 包，延迟已出现在最终 Wintun 提交之前，不能只归因于 IP 分片。两条小包全部及时，当前证据不支持全连接 HOL 或 4096/FEC 容量塌陷。

两个主机时钟没有同步，server 时间只作上下文，不据跨机 UTC 推断单程时延或提前退役。最终 driver 提交及正确长度只证明提交成功，不代替应用交付。PN 被保护，现有外层聚合观测无法证明具体哪个 FEC shard 丢失；不导出密钥或业务 payload。剩余单包恢复长尾保留 PARTIAL，不盲目重复 M03，不为全收齐恢复严格 ACK、扩缓存或延长恢复期限。

历史完整五分钟样本 40、唯一工况 16、27 项 NOT_RUN，含跨 SOURCE 和失败，不是 40 次 PASS。原始限量抓包均完成审计后删除；源文件、摘要、哈希及失败记录保留。

## 下一项原子任务

独立 S06 seed1441：Normal1、每方向 10Mbps、FEC20:4、300 秒及默认 DNS 60 次。验实际 lane/parity 与 encoder 计数、真实混包输入及发送延迟、业务损失、p99、两端产品/助手 CPU、Npcap/raw/user drop、TLS/TCP/MTU/checksum、正常退出及 owned 清理。真实 WAN 丢包未知，如实记录失败，不能改判为已知无损链路。当前测试在运行，结果待收。之后按 S07/08/09/10 顺序各跑一条；低档 FEC 不替代 FEC20:20 主资格。

## S06 seed1441 结果

实际配置Normal1/FEC20:4，300.000543s。状态FAIL，errors=['business_loss']；吞吐{"c2s": 9.999886933333334, "s2c": 9.999953386666666}，业务损失{"c2s_packets": 6, "s2c_packets": 0, "c2s_percent": 0.0009312018574347825, "s2c_percent": 0}，探针{"sent": 2980, "received": 2980, "p95_ms": 61.7291, "p99_ms": 70.9062}。产品/助手CPU秒{"client": 196.484375, "server": 118.58, "client_helper": 22.734375, "server_helper": 35.24308336}，server rawdrop最大43，WindowsNpcap{"interface_dropped": 0, "driver_dropped": 0}、user overflow0。输入{"client": {"TxPackets": 579419, "SendLagSamples": 579419, "SendLagOverflowSamples": 217, "SendLagP99UpperMs": 1.9, "MaxSendLagMs": 99.50829999996813, "PacingMode": "byte-budget-1ms-batch32"}, "server": {"TxPackets": 579418, "SendLagSamples": 579418, "SendLagOverflowSamples": 54, "SendLagP99UpperMs": 1.2, "MaxSendLagMs": 35.77725526541542, "PacingMode": "byte-budget-1ms-batch32"}}，DNS{"total": 60, "success": 60}，selectedNIC plaintext53观察{"state": "PASS_NO_PLAINTEXT_DNS_OBSERVED", "scope": "selected physical NIC only, plaintext IPv4/IPv6 TCP/UDP53; not DoH/DoT/otherNIC", "observations": 303, "dns_frames": 0, "observer_drops": 0}。owned清理/限量raw审计删除门检查保留。

实际encoder parity/source比例client 0.20008590081319436、server 0.20004066216328437，accountingPASS_ACCOUNTING；含DNS/探针/setup/drain的全diagnostic窗口，字节比例不是精确业务窗口放大。低档partial仍min(N,R)，不改分组/期限。真实WAN未人工丢包不等于可证明底层0loss；matched native RTT baseline NOT_RUN，不把本条p99伪装成严格增量门PASS。证据docs/evidence/native-s06-0246526-seed1441-20261006.json，完整样本41、unique17/43、其余NOT_RUN26，包括失败和跨源码。下一S07一条独立样本；M03不关闭。

## 最新源码完整 hosted 资格已启动

产品冻结024资格ref，70 functional configuration（全FEC/lane/padding/JSON优先级/1280-1500MTU与record limits/defaults，低负载仅功能）、18独立strict120s（Normal10/Game4×3，lossless/5205/5305，各seed1451/1452/1453）、Normal1800s seed1442 run37440874448/Game1800s seed1443 run37440878333已实际dispatch。每性能Action一条，campaign88独立run意图/状态持久化防重复；不把旧660/2b全资格继承。还未拿到全部结果，不提前写PASS，不替换失败sample/弱化pairedp99原200/500ms门。M03 tail/业务损失继续保留；两条长测覆盖换代、1s损失、队列、FEC/LINK drain和heap收敛。下一继续nativeFEC20:8 seed1444及后续档位；Windows/ARM配置只在一个nativecase运行，Actions另用独立runner互不串扰。

## S07 seed1444 结果

实际配置Normal1/FEC20:8，300.000680s。状态FAIL，errors=['business_loss', 'physical_dns_scope']；吞吐{"c2s": 9.999846613333334, "s2c": 9.999953386666666}，业务损失{"c2s_packets": 8, "s2c_packets": 0, "c2s_percent": 0.0013344026616901594, "s2c_percent": 0}，探针{"sent": 2980, "received": 2980, "p95_ms": 68.8179, "p99_ms": 76.1558}。产品/助手CPU秒{"client": 231.53125, "server": 133.22, "client_helper": 21.390625, "server_helper": 36.217096240000004}，server rawdrop最大0，WindowsNpcap{"driver_dropped": 0, "interface_dropped": 0}、user overflow0。输入{"client": {"TxPackets": 579419, "SendLagSamples": 579419, "SendLagOverflowSamples": 5, "SendLagP99UpperMs": 2, "MaxSendLagMs": 12.280099999998129, "PacingMode": "byte-budget-1ms-batch32"}, "server": {"TxPackets": 579418, "SendLagSamples": 579418, "SendLagOverflowSamples": 74, "SendLagP99UpperMs": 1.2, "MaxSendLagMs": 42.77551579212968, "PacingMode": "byte-budget-1ms-batch32"}}，DNS{"total": 60, "success": 60}，selectedNIC plaintext53观察{"state": "INCONCLUSIVE_OR_LEAK", "scope": "selected physical NIC only, plaintext IPv4/IPv6 TCP/UDP53; not DoH/DoT/otherNIC", "observations": 302, "dns_frames": 6, "observer_drops": 0}。owned清理/限量raw审计删除门检查保留。

实际encoder parity/source比例client 0.40009735425495363、server 0.4000664338617975，accountingPASS_ACCOUNTING；含DNS/探针/setup/drain的全diagnostic窗口，字节比例不是精确业务窗口放大。低档partial仍min(N,R)，不改分组/期限。真实WAN未人工丢包不等于可证明底层0loss；matched native RTT baseline NOT_RUN，不把本条p99伪装成严格增量门PASS。证据docs/evidence/native-s07-0246526-seed1444-20261006.json，完整样本42、unique18/43、其余NOT_RUN25，包括失败和跨源码。下一S08一条独立样本；M03不关闭。

## S08 seed1445 结果

实际配置Normal1/FEC20:10，300.000661s。状态FAIL，errors=['probe_coverage', 'business_loss', 'physical_dns_scope']；吞吐{"c2s": 9.999385386666667, "s2c": 9.999980053333333}，业务损失{"c2s_packets": 33, "s2c_packets": 0, "c2s_percent": 0.005946678528312432, "s2c_percent": 0}，探针{"sent": 2979, "received": 2978, "p95_ms": 74.6926, "p99_ms": 84.9588}。产品/助手CPU秒{"client": 246.546875, "server": 138.26999999999998, "client_helper": 21.4375, "server_helper": 35.549342440000004}，server rawdrop最大41，WindowsNpcap{"driver_dropped": 0, "interface_dropped": 0}、user overflow0。输入{"client": {"TxPackets": 579419, "SendLagSamples": 579419, "SendLagOverflowSamples": 230, "SendLagP99UpperMs": 2, "MaxSendLagMs": 110.12159999995674, "PacingMode": "byte-budget-1ms-batch32"}, "server": {"TxPackets": 579419, "SendLagSamples": 579419, "SendLagOverflowSamples": 79, "SendLagP99UpperMs": 1.2, "MaxSendLagMs": 48.087579080572596, "PacingMode": "byte-budget-1ms-batch32"}}，DNS{"total": 60, "success": 60}，selectedNIC plaintext53观察{"state": "INCONCLUSIVE_OR_LEAK", "scope": "selected physical NIC only, plaintext IPv4/IPv6 TCP/UDP53; not DoH/DoT/otherNIC", "observations": 303, "dns_frames": 2, "observer_drops": 0}。owned清理/限量raw审计删除门检查保留。

实际encoder parity/source比例client 0.5001152504227042、server 0.5001360177997188，accountingPASS_ACCOUNTING；含DNS/探针/setup/drain的全diagnostic窗口，字节比例不是精确业务窗口放大。低档partial仍min(N,R)，不改分组/期限。真实WAN未人工丢包不等于可证明底层0loss；matched native RTT baseline NOT_RUN，不把本条p99伪装成严格增量门PASS。证据docs/evidence/native-s08-0246526-seed1445-20261006.json，完整样本43、unique19/43、其余NOT_RUN24，包括失败和跨源码。下一S09一条独立样本；M03不关闭。

## S09 seed1446 结果

实际配置Normal1/FEC20:12，300.000687s。状态PASS_SCOPED_NATIVE_CAPACITY，errors=[]；吞吐{"c2s": 9.999980053333333, "s2c": 9.999953386666666}，业务损失{"c2s_packets": 0, "s2c_packets": 0, "c2s_percent": 0, "s2c_percent": 0}，探针{"sent": 2980, "received": 2980, "p95_ms": 78.6204, "p99_ms": 90.2461}。产品/助手CPU秒{"client": 260.90625, "server": 142.76, "client_helper": 22.59375, "server_helper": 35.36572932}，server rawdrop最大0，WindowsNpcap{"interface_dropped": 0, "driver_dropped": 0}、user overflow0。输入{"client": {"TxPackets": 579419, "SendLagSamples": 579419, "SendLagOverflowSamples": 17, "SendLagP99UpperMs": 2, "MaxSendLagMs": 17.64259999998785, "PacingMode": "byte-budget-1ms-batch32"}, "server": {"TxPackets": 579418, "SendLagSamples": 579418, "SendLagOverflowSamples": 0, "SendLagP99UpperMs": 1.2, "MaxSendLagMs": 9.849129837164128, "PacingMode": "byte-budget-1ms-batch32"}}，DNS{"total": 60, "success": 60}，selectedNIC plaintext53观察{"state": "PASS_NO_PLAINTEXT_DNS_OBSERVED", "scope": "selected physical NIC only, plaintext IPv4/IPv6 TCP/UDP53; not DoH/DoT/otherNIC", "observations": 303, "dns_frames": 0, "observer_drops": 0}。owned清理/限量raw审计删除门检查保留。

实际encoder parity/source比例client 0.600061562249456、server 0.6000458154068623，accountingPASS_ACCOUNTING；含DNS/探针/setup/drain的全diagnostic窗口，字节比例不是精确业务窗口放大。低档partial仍min(N,R)，不改分组/期限。真实WAN未人工丢包不等于可证明底层0loss；matched native RTT baseline NOT_RUN，不把本条p99伪装成严格增量门PASS。证据docs/evidence/native-s09-0246526-seed1446-20261006.json，完整样本44、unique20/43、其余NOT_RUN23，包括失败和跨源码。下一S10一条独立样本；M03不关闭。
