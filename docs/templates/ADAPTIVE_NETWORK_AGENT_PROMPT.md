# 全新agent长期接手提示词

接手 GitHub `lly8666/wobuzhidao` 的 `next/adaptive-fec-aes-tun-20261010` 分支，连续开发并通过 GitHub Actions 验收，直到本轮 N0..N6 完成或有必须如实交代的外部阻塞。先核当前精确远端 HEAD、工作区和 SOURCE，不能覆盖别人的修改；当前进度只看 `docs/STATUS.json` 的 active_work / next_task / latest_log，不按本文首次编写时的阶段假设开工。

先读 `AGENTS.md`、`PROJECT_CHARTER.md`、`docs/STATUS.json`、`docs/ADAPTIVE_NETWORK_PLAN.md`、`docs/AGENT_CONTINUITY.md`。按需读 WIRE_SPEC、MODULE_MAP、PARAMETERS、ACCEPTANCE 和 REALPATH_TEST_FIXTURE_GUIDE。完整方案已经确定，不另做多种架构/密码/FEC算法比赛。旧计划/完整状态已归档到 `docs/history/20261010-adaptive-network-parent/`；它们和 old 的提示词都是历史来源，禁止执行旧“下一步”。

永久目标：真实业务首次到达效率第一，低 p99、无跨业务 HOL、突发稳定和低 CPU/带宽；有界内存可以换 CPU，额外安全等级最后。认证、payload完整性、账号/地址隔离、同Seq同wire、generation、统一MTU与资源有界不变。后到完整record/systematic/独立业务立即交付；4096只是可放弃shadow备份，fresh不等ACK。保留已有SIMD FEC、partial min(k,R)、长度分类、32ms/3s和迟到首次交付。Game竞速去重及A→A+B→B换代不改。

按 N0→N6 做原子开发和验收：

1. 在真实TLS受保护admission里一次协商FEC模式/范围/初始档和record密码，服务器同端口per-Tunnel支持不同客户不同配置，双向第一包正确，不等首个业务包猜参数。V3协议/KDF/向量先明确；业务包不加mode/cipher字段，未知或旧版本明确拒绝。
2. 建立有界低开销双向质量统计/反馈，约2秒一条摘要，允许重排成熟时间但不等待业务。估计外层丢包不冒充精确物理loss，不拿业务恢复后的loss或重传比例代替。UNKNOWN/过期/不足样本保持档位；PN跳号/发送失败/迟到/旧generation不能污染。health、quality与payload idle分开，Dormant不因刷新唤醒。
3. auto只Normal，默认初始20:20、最低20:4、最高20:20，可选最低off；快升慢降/稳定期/过载保护按方案实现。auto-aggressive在普通控制器建议上高一个离散档并受ceiling约束。Game只固定档，旧fixed配置不隐式迁移auto。只新block切档，旧block/裸LINK迟到仍正确处理，PN/BlockID/TUN MTU不重置，无HOL。
4. 保留ChaCha，新增AES-128-GCM/AES-256-GCM，使用Go标准库硬件检测/generic fallback，AEAD与AES header protection每lane预建。算法/方向KDF分离，独立PN/nonce、31B开销、same-wire repair保留。TLS1.3不能靠Config.CipherSuites控制，复用uTLS实际协商；借用站支持时尽量同算法，站不支持允许不同并记录actual/unknown，不把证书等同cipher支持，不伪造fallback握手。
5. Windows用少量捕获路由+TUN内有界分类，取消中国CIDR补集的大量系统route。direct用内核TCP重定向/native物理绑定socket与UDP mapping，解决完整五元组、源地址/回程、防回环、fragment/DF/ICMP范围。代理数据面不进入新TCP代理，Linux/OpenWrt保留nft。网络Apply取消/重复Stop/部分失败/owned清理幂等，不再卡界面。保留DNS双备份、IPv6默认丢弃、portable和三种分流模式。
6. 同步CLI/JSON/GUI/catalog/帮助/示例，中文界面每两秒显示大概RTT、上下行估计丢包、实际档位与UNKNOWN/休眠/过载。不要为显示打开重型diagnostic-jsonl/逐包计时。完成真实工况与三目标同源P6包核hash，标ACTIONS_READY_FOR_PHYSICAL，physical仍NOT_RUN。

**最新用户追加且必须持续遵守的开发/测试次序**：N0→N6 的各功能先完成代码，再在 Actions 运行该功能的 unit/race/功能/性能与所需回归，失败逐项修复并基于新精确 SOURCE 复验；整个开发期间不插入物理机试跑。只有所有功能、跨功能 Actions 验收及同源 P6 三目标包/manifest/hash 都收口后，才能标记 `ACTIONS_READY_FOR_PHYSICAL`（`physical=NOT_RUN`），之后由**原聊天统一进行最终物理机测试**。不能让物理机替代 Actions；Actions 无真实 Windows Wintun/硬件能力时如实记 `UNSUPPORTED/NOT_RUN`，在最终物理阶段统一核验，不能以 Linux/mock 冒充。任何中途的 Actions PASS 都不是物理部署授权，不自动部署、不合并主线。

所有构建、Go/unit/race/fuzz/功能/性能在Actions.本分支每个性能Action严格一条样本，一个SOURCE/配置/seed/场景和一个测量job；无matrix/并行负载/顺序多leg，不继承旧SIMD ABBA例外。单配置固定loss波形属于一个场景。复用现有五netns真实业务夹具，另做Windows实际TUN/direct功能，mock不冒充驱动资格；支持不足写UNSUPPORTED。

测试全面但门槛合理，按方案第9节执行：真实双向Normal10M/Game4逻辑各3M、大小UDP/TCP/HTTPS、0/低loss/5205/5305、动态升降/突发/乱序/反馈中断、多客户混合、rotation/idle/keepalive/DNS/IPv6/分流/MTU边界。损伤超过FEC实际恢复能力可有残余业务loss，不统一要求探针全回或大包全恢复；重点真实首次交付/p99/无HOL/CPU/有界恢复。硬门仍严格，原历史FAIL不能改写。

先核CPU型号/硬件flags/配额/PSI/steal、注入率、AF_PACKET/socket/内部queue drop。CAPACITY_LIMITED/INVALID/INCONCLUSIVE/FAIL/NOT_RUN分别记录，不同runner不直接拿CPU-s宣布收益。profile-on只诊断，普通性能默认off；两次同失败无新证据就缩小定位，不盲重跑或扩大队列。

每轮同一提交新增详细 `docs/devlog`、更新唯一STATUS并留evidence；每个新增参数同步全部入口，记录精确SOURCE/helper/config/seed/run/job/artifact/hash和实际失败/限制。原约80秒S2C中断和多秒late保持OPEN，按用户安排本轮后单独定位，遇到重现如实留痕，不能猜WAN或FEC。不自动部署用户现有机器、不移动旧qualification ref、不合并主线。凭据/密钥/ticket/业务正文不上传，不持久大pcap，清owned大raw保留摘要/hash。

最初计划提交只有文档，本轮产品尚未实现/未测；现在接手必须重新读取STATUS确认后续是否已有进度，按当前next_task继续，而不是重复N0或继承父分支PASS。
