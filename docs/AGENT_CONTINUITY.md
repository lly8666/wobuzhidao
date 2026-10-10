# 新agent十分钟接手摘要

本轮任务只看STATUS；具体实现与验收见ADAPTIVE_NETWORK_PLAN。父分支完整状态与原方案在history/20261010-adaptive-network-parent/，是证据不是执行清单。

## 继承什么

父文档头a8913e3b，产品7fb98fab来自next/fec-simd-20261010。单进程TLS-like独立record、真实TLS/uTLS bootstrap、有限4096 shadow、即时systematic、lane-local FEC、LINK大包兼容、方向锁和平台能力已经实现。FEC使用klauspost/reedsolomon SIMD span，fused状态先读源码；已有source/partial/长度组/32ms/3s语义保留。

|历程|留下的经验|不能推导|
|---|---|---|
|旧DTLS/wolfSSL|FEC/弱网/生命周期算法经验|不恢复旧多进程/回环/旧提示词|
|P0..P4 TLS-like|真实建连/独立加密record/TUN/Game/身份|不能保证完整网站指纹一致|
|ownership/repair/方向锁|不可变wire、fresh不等ACK、跨方向争用降低|4096不是可靠flight窗口；未找到旧包无需继续repair|
|FEC生命周期/带宽/MTU|bounded退役、尺寸分类、inner预算、单包自身分片|更大buffer/更多库存不是万能修复；partial不等于20满组|
|Windows分流/GUI|中国表补集内核路由、portable/owned清理|实际约12723条路由曾导致Apply取消/Stop卡住，本轮必须替换与修复|
|FEC SIMD|Q1同runner lossless CPU/GiB约降低10.923%；core等scoped门通过|Q2 300s四leg三质量FAIL，一host描述CPU约低13.213%不是完整资格|
|物理试用|7fb同SOURCE包部署，基础连接/DNS/HTTPS/分流smoke|不是完整5分钟弱网/ARM SIMD/rotation/idle/p99验收|

物理用户试用机器已做owned网络恢复，GUI断开卡住的产品修复尚未完成；恢复网络不等于软件修好。原fixed20:20/Cisco/Normal部署本轮不自动升级。

## 新增固定的测试与物理机顺序（2026-10-10 用户指令）

N0→N6 按当前 `STATUS.active_work/next_task/latest_log` 连续开发，每项功能完成后**先在 GitHub Actions 验证**，失败按精确 SOURCE 修复复跑；阶段或单项通过绝不提前去物理机器试用。等所有功能、组合 Actions 验收、同源 P6 包/manifest/hash 全部准备完毕，再一次性提交 `ACTIONS_READY_FOR_PHYSICAL`，此时 `physical=NOT_RUN`。**物理机测试统一放在最后、由原聊天组织**；本开发线程不自动部署用户机器。Actions 环境跑不了的 Windows native Wintun/真实硬件功能显式 `UNSUPPORTED/NOT_RUN` 并说明最后物理待验，不能把模拟器或 Linux 夹具称为 native PASS。不要用最后的物理测试替代前面的 Actions 测试，不要移动主线/qualification ref。历史物理试用只作历史证据，**不构成本轮提前物理复验的许可**。

## 当前产品事实与新任务

当前admission没有FEC策略/密码，server仍固定本机FEC；tlsrecord只ChaCha。本轮先一次受保护V3协商，server per-Tunnel支持不同客户，第一S2C包就正确，不靠首个业务包猜配置。

auto仅Normal，上下行分别根据RX反馈决策；初始20:20/最低20:4/最高20:20，可设min off；aggressive在普通建议上+1。质量估计不是业务残余loss，也不是重传比例；无反馈UNKNOWN/HOLD。切档不重置PN/BlockID/MTU、不抛旧组、不形成业务等待。

AES用Go标准库，AEAD/HP每lane预建、独立nonce/方向密钥，AES HP不再每包建ChaCha；同Seq保持原wire。TLS1.3不能靠Config.CipherSuites设置，uTLS实际CH协商与decoy best-effort明确分开。

Windows少量系统路由捕获，TUN内有界flow缓存/区间查表，direct使用内核TCP重定向+native物理绑定socket和UDPmapping；不是原包盲注网卡。代理业务仍原数据面，Linux/OpenWrt保留nft分流。DNS优先/IPv6/休眠/退出清理不得丢。

## 未解决问题与防回退

- 约80秒S2C中断原因未知，按用户指令本轮后单独定位。不要归因WAN/FEC/runner；测试重现需留证。
- 历史11配对RTT FAIL、S01/S16/M03 late/missing、Q2部分probe缺失都在父STATUS/evidence，未被新方案消掉。不能回写为PASS。
- PMTU/ICMP、最大UDP/DF各边界和新源码完整压力/Windows驱动/ARMnative尚未全验，不拿cross-build替代实机。
- 更大socket queue曾加1.5..1.9s延迟而没有收益；不要默认扩大buffer。4lane逻辑goodput不计副本。
- 允许合理弱网残余loss，不允许payload损坏、fresh门控、无界state、全局HOL或owned清理破坏系统。

接手N0→N6，每性能Action一条，使用真实夹具、先核CPU/配额/PSI/注入、失败分层。每轮devlog+STATUS同提交，catalog/GUI同步，未跑不写通过。不要把194KB父STATUS重新搬回当前进度或为简单任务造第二套交接文件。
