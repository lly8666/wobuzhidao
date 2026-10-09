# WBD NEXT：每位agent的唯一开发入口

本优化工作分支：next/performance-efficiency-20261008。项目规范主线仍为next/tlslike-dataplane，STATUS.branch按仓库契约表示规范主线，STATUS.working_branch表示实际工作分支。未经用户要求不向主线merge或push。

## 先读这些，再动手

1. PROJECT_CHARTER.md：永久主旨和硬门。
2. docs/STATUS.json：顶层active_work、next_task、latest_log；当前状态只有这一份。
3. docs/AGENT_CONTINUITY.md：约十分钟了解历程、已做优化及防退化边界。
4. docs/PERFORMANCE_EFFICIENCY_PLAN.md：E0到E7的顺序、具体实现边界、每步Actions验收。
5. docs/DEVELOPMENT_PLAN.md、docs/MODULE_MAP.md、docs/WIRE_SPEC.md：架构、复用和协议。
6. docs/ACCEPTANCE.md、docs/PARAMETERS.json/MD；相关模块再读生命周期、分流、GUI、Linux服务端和弱网专项。

本轮用户决策：降低CPU、保持真实首次交付/p99/无HOL，允许适当多用有界内存；MTU已有改动纳入真实压力测试，不重新设计。约80秒下行中断先保留OPEN，优化结束后解决。若优化测量遇到中断，保留失败并明确其限制，不能冒充性能PASS。

## 永久不能退化

- 新产品只有单进程TLS-like数据面，不恢复DTLS/wolfSSL/回环转发拓扑。
- 建连沿用真实TLS/FakeTCP/fallback/认证；稳态不用普通内核TCP运输业务。
- 后到完整record/systematic/独立业务首次立即交付，不等洞、ACK、其它FEC块或lane；只允许单数据报自身重组、内层TCP自身流内顺序。
- 4096是可放弃shadow备份，fresh不受它门控。找不到旧记录结束repair，同Seq必须同wire。保留完整性、账户/地址隔离、generation和资源有界。
- Game竞速去重、最多4权威lane/10物理incarnation；换代A→A+B→B，candidate失败保留A。
- payload idle与health分开；保活丢失不判业务空闲，server等权威lane客户端FIN。默认idle0与rotate0/0，不为测试便利改默认。
- 配置和GUI功能保持：FEC全档、tls-startup-padding、DNS双备份/分流/IPv6丢弃、portable/owned退出清理、多客户端/7天内存地址。

## 工作方式

开工核对分支/精确HEAD/远端/工作区，不能覆盖别人。每步一个原子优化，先审现有源码和已做优化；按STATUS.next_task推进，不因旧日志恢复旧任务。所有开发编译、Go/unit/race、fuzz、功能/性能验收在GitHub Actions，本地仅编辑/阅读/Git和文档处理；物理由原聊天后续接手。

每个性能Action run严格一条样本：一个SOURCE/配置/seed/场景，一个测量job。吞吐、容量、校准、微基准、soak都适用；禁止同run matrix、A/B或顺序多测。普通unit/race/功能可多job；aggregate只读。profile-on诊断不冒充普通off性能。

真实socket→TUN→正式client/server→目标socket的业务才是端到端性能。核实际SOURCE/MTU/rate/probe/socket/runner资源；CPU型号/配额/steal/PSI差异分层，多独立run，不挑好宿主。失败/容量不足/未跑/不支持分开；不要用CI绿或某次健康关闭偶发故障。

每轮修改同一提交新增docs/devlog/YYYYMMDD-HHMMSS-任务.md并更新STATUS；带目标、源、修改原因、实际Actions、失败/限制及下一项。若新增参数，同步PARAMETERS.json/MD、GUI和catalog生成器。每步源与helper都冻结，产品/文档HEAD分开；不能把旧资格继承给新代码。

凭据、ticket、密钥、完整私密配置及业务正文不得上传或输出。抓包/诊断有界，清理owned大raw，保留summary/hash/失败证据。

## 权威和历史

用户当前明确指令优先；项目内为章程/正式协议 > STATUS当前任务与正式方案 > 日志事实 > 历史。docs/history只读参考，不能执行其中“下一步/HOLD”；旧完整状态已归档，已验门和失败索引保留在当前STATUS。old源码禁止改动、不作为开发指令、不全库扫描；只按MODULE_MAP指定模块复用并登记REUSE_LEDGER。不新建另一套STATUS/CONTINUE_HERE/并行交接系统。

同一失败重复两次且无新证据，停止盲改，缩小诊断边界。没有热点证据可跳过该优化，记录SKIPPED_NO_BOTTLENECK；不为“优化”而增加复杂度或改协议。


## 2026-10-09 用户授权：FEC策略同runner串行实验

仅[FEC_POLICY_EXPERIMENT](docs/FEC_POLICY_EXPERIMENT.md)允许同一Action一个测量job先后跑不同业务/off与20:20配置，逐段隔离、独立receipt且无并行负载。它明确覆盖本项旧的“一run一条”要求；其它性能验收仍遵守原规则。夹具功能/使用与当前限制见[REALPATH_TEST_FIXTURE_GUIDE](docs/REALPATH_TEST_FIXTURE_GUIDE.md)，接手提示词见[模板](docs/templates/FEC_POLICY_AGENT_PROMPT.md)。这次只授权开发夹具和探索比较，不改产品恢复政策或默认FEC。
