# WBD：新agent唯一开发入口

当前工作分支 `next/adaptive-fec-aes-tun-20261010`，从 FEC SIMD 分支最新文档头 a8913e3b 建立，继承产品 SOURCE 7fb98fab。规范主线仍 next/tlslike-dataplane，STATUS.branch 保持仓库契约，STATUS.working_branch 是实际工作线。不要改旧分支、移动qualification ref、合并主线或自动部署现有试用机器。

## 只按当前任务开工

先读 PROJECT_CHARTER.md、docs/STATUS.json、docs/ADAPTIVE_NETWORK_PLAN.md、docs/AGENT_CONTINUITY.md。接手提示词在 docs/templates/ADAPTIVE_NETWORK_AGENT_PROMPT.md。只有 STATUS.active_work / next_task / latest_log 是当前进度。历史 FEC SIMD Q2、E0..E7、旧模板的下一步不是本轮任务。

本轮 N0..N6：一次受保护协商每客户端FEC/密码；低开销质量反馈；Normal自动/激进自动档；AES128/256；Windows少量路由+TUN分流；中文GUI两秒显示；Actions集成与打包。Game只固定档。新参数当前尚未实现，不能把方案默认值当运行值。完整设计、初始阈值、字段/MTU/迟到包边界及合理验收已确定，不另开算法比赛。

## 永久不能负向优化

- 真实业务首次到达最高优先，低p99/无跨业务HOL/突发稳定/低CPU和带宽；允许有界内存换CPU。
- 单进程TLS-like独立record；只有建连可用有界有序BootstrapStream，稳态不进入普通TCP可靠字节流。后到完整record/systematic/独立业务立即交付，不等洞/ACK/block/其它lane。
- 4096仅可放弃shadow repair缓存，fresh不门控；same Seq same wire。认证、完整性、账号地址隔离、generation、统一MTU/peer MSS和资源有界是硬门。
- 保留已优化FEC的即时source/partial min(k,R)/32ms/3s/迟到首次交付/长度分类。只对新block切档；旧组保留原期限，不能扩大各档重复状态预算。
- Game2..4权威lane/10物理incarnation，A→A+B→B、candidate失败保留A；不将自动FEC扩散到Game。
- payload idle与health分开，保活/质量反馈不唤醒Dormant；rotation默认0/0、idle默认0保持。DNS双备份、IPv6默认丢弃、分流、portable/owned清理、多客户端/7天内存地址保留。

## 开发、测试与留痕

所有编译/Go/unit/race/fuzz/功能/性能在GitHub Actions。本地只编辑、阅读、Git和文档处理。每轮同一提交新增详细docs/devlog并更新唯一STATUS；源码/helper/配置/seed/运行/hash/失败限制和下一步记录齐全。参数实际新增时同步PARAMETERS.json/MD、catalog生成器、CLI/JSON/GUI。

本分支每个性能Actions run严格一个SOURCE/配置/seed/场景、一个测量job；无matrix/并行或顺序多leg。旧SIMD/FEC-policy串行例外不适用。单场景预声明loss波形合法，不能拿波形当理由串行换算法和配置。功能多client正确性可多job但不是性能资格。新workflow精确branch/config受限，保留旧guards/旧分析结论。

复用真实路径夹具：socket→正式client/TUN→FakeTCP→损伤网络→正式server→目标socket；Windows TUN/direct单列真实平台功能。先核runner CPU/flags/quota/PSI/steal、注入和drop；CAPACITY_LIMITED/INVALID/UNSUPPORTED/NOT_RUN分别保留，不冒充PASS。弱网不用一律零loss/探针全回；硬门不放宽，p99/missing/throughput/CPU共同报告。详细门槛见本轮方案第9节。

同一失败重复两次且无新证据就缩小边界，不盲重跑。诊断ON不当普通OFF性能；不为了界面读数开启重型JSONL/逐包计时。秘密/keys/tickets/正文不上GitHub、不输出，抓包有界并清owned大raw，保留摘要/hash/原FAIL。

## 权威与归档

用户最新明确指令 > 本分支章程/正式wire > STATUS/本轮方案 > 事实日志 > 历史。本轮明确新增自动FEC/密码/WindowsTUN分流，覆盖旧文档“不做动态比例/不比较密码/仅Windows补集路由”的范围限制；没有覆盖其它硬门。

父分支完整状态和开发计划已归档 docs/history/20261010-adaptive-network-parent/，只作来源证据，不能执行历史下一步。old源码只读不全库扫描，不恢复DTLS；按MODULE_MAP最小复用，提取old则登记REUSE_LEDGER。源码/evidence/devlog和旧Actions工作流不删除，未验项和80秒S2C问题不能抹去。原多秒late probe仍OPEN。新源码资格从零开始，不继承parent PASS。
