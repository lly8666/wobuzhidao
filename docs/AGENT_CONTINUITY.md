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

2026-10-13接手快照（后续实时任务仍以STATUS为准）：V3 admission、per-Tunnel FEC policy、低频质量反馈、Normal双向auto live切档已实现；ChaCha、AES-128-GCM、AES-256-GCM均已接入认证和Lane。AES128 Action38273052020、AES256 Action38281815825各自100s真实五netns无损双向业务限定PASS，不代表有损/CPU/ARMnative资格。Windows GUI已开放三套件和自动FEC，Action38283111076原生273项检查PASS；质量显示的物理网络效果未验。不要重复N0/N1、AES核心或GUI参数开发。

N2 noHOL Go/race Action38257097791限定PASS；更正TCP种子/目标listener后，SOURCE ee4424223401e5b5ea0aec68e0caf28897408bb0 的Action38326649686完成真实120s混合TCP/UDP、每向10Mbps/300ms/约1%损伤限定PASS。快速UDP C2S12000/12000、S2C11900/11900，p99 308.50/307.58ms，最长空槽10ms；四个长TCP流各67帧完整、120条短TCP首达和CRC检查通过。它不是全工况无HOL或GE低延迟资格；首轮Action38326041291的夹具FAIL保留。

N4已有纯IPv4/DNS分类、512flow/256fragment缓存及受认证Router/Direct分派边界，Action38289962670两平台限定PASS。Windows UDP物理接口绑定socket与回包IPv4/UDP重构核已在SOURCE 1b3e923、Action38333124333完成hosted原生Windows同机UDP echo+Linux race限定PASS（实际执行，非SKIP；PacketWriter仍模拟）。正式入口仍保留`NewTUNInput(router)`源/MTU过滤，但已通过`ForwardOrProxy`接线到真实UDP物理socket及TUN PacketWriter；SOURCE 49adb/Action38337768067 Windows/Linux限定PASS。SOURCE 67b7/Action38338092331完成hosted Windows PowerShell Stop取消、一次回滚测试限定PASS；SOURCE 725d因旧模拟DHCP字段缺失导致Action38338521944/ Foundation38338521920原样FAIL，SOURCE ab2d修复模拟、Action38338808653限定PASS，且保护外来Wintun IPv4及DHCP状态。TCP内核WFP redirect/签名驱动、少量捕获路由、完整owned实机清理与真实Wintun回注仍OPEN；不能重复已完成入口/UDP核或冒称实机资格。N6最终三目标同源发行包未开始，physical NOT_RUN。

当前新增任务读 [ADAPTIVE_FEC_CURVE_ACCEPTANCE](ADAPTIVE_FEC_CURVE_ACCEPTANCE.md)：独立理论/检测曲线→预声明允许带→真实工况单条Actions→必要修复。有限过保护余量、升降时延、parity/CPU/业务首交付/p99共同验收，不能只看业务0loss；30s/60s/512/32窗口等软参数可有证据地小改，永久硬门保持。质量报告是双端对齐统计的必要反馈，不是额外主动业务探针，fixed/off不运行auto。

auto仅Normal，上下行分别根据RX反馈决策；初始20:20/最低20:4/最高20:20，可设min off；aggressive在普通建议上+1。质量估计不是业务残余loss，也不是重传比例；无反馈UNKNOWN/HOLD。切档不重置PN/BlockID/MTU、不抛旧组、不形成业务等待。

AES用Go标准库，AEAD/HP每lane预建、独立nonce/方向密钥，AES HP不再每包建ChaCha；同Seq保持原wire。TLS1.3不能靠Config.CipherSuites设置，uTLS实际CH协商与decoy best-effort明确分开。

Windows目标少量系统路由捕获，TUN内有界flow缓存/区间查表，Direct必须使用内核TCP重定向+native物理绑定socket和UDPmapping；UDP及入口软件接线已限定验证，但旧大型补集路由尚未切换，缺少TCP WFP驱动，未有真实Wintun外网/owned Apply资格；不是原包盲注网卡。代理业务仍原数据面，Linux/OpenWrt保留nft分流。DNS优先/IPv6/休眠/退出清理不得丢。

## 未解决问题与防回退

- 约80秒S2C中断原因未知，按用户指令本轮后单独定位。不要归因WAN/FEC/runner；测试重现需留证。
- 历史11配对RTT FAIL、S01/S16/M03 late/missing、Q2部分probe缺失都在父STATUS/evidence，未被新方案消掉。不能回写为PASS。
- PMTU/ICMP、最大UDP/DF各边界和新源码完整压力/Windows驱动/ARMnative尚未全验，不拿cross-build替代实机。
- 更大socket queue曾加1.5..1.9s延迟而没有收益；不要默认扩大buffer。4lane逻辑goodput不计副本。
- 允许合理弱网残余loss，不允许payload损坏、fresh门控、无界state、全局HOL或owned清理破坏系统。

按STATUS当前next_task接续，不从N0重做。每性能Action一条，使用真实夹具、先核CPU/配额/PSI/注入、失败分层。每轮devlog+STATUS同提交，catalog/GUI同步，未跑不写通过。阶段交接刷新本摘要/方案/长期提示词；旧SOURCE失败仍原样保留，不能把OPEN一律当当前阻断。不要把194KB父STATUS重新搬回当前进度或为简单任务造第二套交接文件。
