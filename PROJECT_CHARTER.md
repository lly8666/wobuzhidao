# WBD 项目主旨与永久硬门

实时任务只有 STATUS.json。当前工作 next/adaptive-fec-aes-tun-20261010，正式方案 docs/ADAPTIVE_NETWORK_PLAN.md。此前长章程完整保存在 history/20261010-adaptive-network-parent/PROJECT_CHARTER.md；历史范围限制不覆盖本轮用户新授权。

## 优先级

真实业务首次到达效率第一，低延迟/p99、无跨业务HOL、突发稳定、较小CPU/带宽优先；有界内存可换CPU。外层尽量TCP/TLS-like，额外安全等级最后。认证/账号地址隔离/完整性/同Seq同密文/generation/MTU/资源有界不能取消。

用户允许链路高丢包时保留合理业务残余丢包；不得主动丢fresh业务凑延迟指标。受控无损健康容量应完整交付；高丢包超内层MTU大UDP不强求全恢复，其自身重组不能堵住独立业务。

## 运输语义

- 单进程数据面，不恢复DTLS/wolfSSL/回环转发拓扑。
- SYN、真实TLS/认证/fallback、稳态使用同FakeTCP association/四元组/序列空间。只有建立阶段允许有界有序BootstrapStream。
- 稳态每record独立解密，后到完整record、systematic和独立业务首次立即交付，不等ACK/洞/FEC组/其它lane。单数据报自身重组及内层TCP自身顺序是正常语义。
- 4096是可放弃shadow备份而非fresh窗口。有限预算/期限repair，缺失旧wire就结束；同Seq重发同wire，不重新seal，不让对端等放弃的洞。
- FEC lane-local，已有SIMD/长度分类、partial min(k,R)、32ms、3s、bounded heavy/compact和late first-arrival保留。新自动档只Normal，只影响新block，Game固定档。统一MTU预算、peer MSS、LINK单次分片，不隐藏截断。

## Windows N4 小路由与无 PowerShell 硬要求（用户 2026-10-13 明确授权）

Windows 正式产品不得依赖 powershell.exe 或活动 .ps1 脚本；需用 Go 调用 Windows 原生 API 来完成受限路由、租约/DHCP、DNS、IPv6/泄漏防护与 journal-owned 清理。国内外 IP/CN 表只在用户态内存分流，不得重新批量写入系统路由。允许旧项目 tun2socks/gVisor 的本地 TCP 双段物理接口绑定直连方案（区别于远端 WBD 代理），但所有真实路由/防泄漏/回退完整 Actions 验证前不得正式接线。原始旧脚本需在最终产品活动目录与打包中删除，归档 old/ 保存历史证据。

## 生命周期与平台

Account→Installation→Tunnel→Lane，租约绑定Tunnel，同账号多个installation地址隔离，服务端校验source lease。自动随机IPv4只内存7天，不要求持久化。

Normal1、Game2..4权威lane，最多10物理incarnation。健康换代A→A+B→B，candidate失败保留A；一切旧任务带generation，不复活退休lane。

payload idle与transport health分开；keepalive丢失不判业务空闲。Dormant保留Tunnel/lease/TUN/网络状态，有代理业务重建；direct和quality refresh不唤醒。默认idle0/rotate0不改，完整Dormant无独立服务端反向唤醒能力要如实说明。

Windows每Tunnel一个Wintun，Npcap/portable/owned网络journal保留，程序文件留自身目录。取消/重复Stop/失败恢复必须幂等，仅清owned资源。新增TUN分流不创建中国CIDR规模的系统路由。Linux/OpenWrt现有nft分流保留，server共享监听/TUN/owned NAT，不能回到每用户netns。

DNS默认1.1.1.1/8.8.8.8互备，IPv6默认捕获丢弃，已有LAN/CN/all语义与手动IP库更新保留。native direct socket必须绑定underlay防回环，不能把TUN原IP包直接扔网卡当完成。

## 本轮明确授权

新增Normal fixed/off/auto/auto-aggressive、每客户端FEC/三密码一次协商、双向低频quality feedback；AES128/256使用成熟跨平台标准库，独立记录不引入有序解密。TLS/借用站cipher一致尽力而为，站不支持时允许不同，但日志记录实际值。证书≠cipher支持，fallback不伪造握手。

新增中文GUI两秒近似RTT/丢包显示。普通auto初始20:20/最低20:4，可设off；激进在普通建议档上+1，最高20:20。旧fixed配置不隐式迁移auto。计划能力是否生效以catalog/源码/STATUS为准。

2026-10-12用户修订：Normal auto及显式普通off-start已有生产实现，具体资格仍按精确SOURCE/STATUS，不重复历史尚未实现任务。N2以 docs/ADAPTIVE_FEC_CURVE_ACCEPTANCE.md 的独立理论/可实现时间带和真实工况验收保护与开销：有可信数据时避免长期不必要高档。统计/控制软参数可凭跨工况反例最小修订，不能把原默认阈值变成永久硬门；运输/认证/隔离/完整性/same-wire/generation/MTU/资源与首次业务交付原则不变。

## 验收与协作

**最终统一物理机验收规则（用户明确追加）**：N0→N6 各功能先开发、在 Actions 测试并修复，全部功能与组合验收、同源 P6 构建及 Actions 证据齐备后，才进入统一的最后物理机测试阶段。不得在任一单功能或中间迭代后要求物理试跑；物理机测试不得用来代替缺失的 Actions 回归或冒充已通过的 Windows native 功能。Actions 无法验证的硬件项目要记录 UNSUPPORTED/待物理复验，不冒写 PASS。完成 Actions 只更新 ACTIONS_READY_FOR_PHYSICAL，保持 physical=NOT_RUN；由原聊天统一安排最终物理测试，禁止本开发线程自动部署。

构建和测试只在Actions。每性能run一条样本，一SOURCE/配置/seed/场景、一测量job，旧串行例外不跨任务继承。普通unit/race/功能可多job。源码精确SHA证据归属，新源码不继承历史资格；core/包/hosted/物理分别写.

弱网按真实k/r/partial/相关性解释，重点首次交付/p99/无HOL/CPU/有界恢复，不强求全探针/大包零loss；完整性、隔离、同wire、MTU和清理硬门仍严格。runner差异/容量上限突出报告。

每轮原子提交devlog+STATUS，参数同步全部入口。凭据与正文不上GitHub，观测有界、大raw及时清理。约80秒S2C、多秒late和剩余PMTU/native资格保留OPEN，按安排后续解决；尚未完成不能标全产品交付。
