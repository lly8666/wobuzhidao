# WBD NEXT — 每位 agent 的开发入口

2026-10-06当前：d6cb6ce Windows最新ACK10定向/race10、12RTT/P6/lifecycle36+aggregate PASS；最新实机S02 seed1481完整300s BUSINESS_PASS_TRANSPORT_PRESSURE，worker实际PASS/stageoff/profileoff，goodput2.99999/3.00000M、p99134.40ms，损失/压力原门保留。51普通样本/23工况/20NOT_RUN另2诊断，M03/P7未关闭。每性能Action一条，先STATUS/latest_log。

本分支是 `next/tlslike-dataplane`，唯一产品方向为单进程、单 TLS-like 数据面。不是 DTLS 兼容分支。所有 agent，包括全新接手者，在修改前执行以下流程。

## 必读顺序与权威

1. `PROJECT_CHARTER.md`：不可丢失的用户目标。
2. `docs/STATUS.json`：当前里程碑、下一项工作、真实测试状态。
3. `docs/ROADMAP.md`：按依赖顺序执行的阶段。
4. `docs/DEVELOPMENT_PLAN.md`：已决定的架构和执行细则。
5. `docs/WIRE_SPEC.md`、`docs/MODULE_MAP.md`：本任务涉及的协议/模块。
6. `docs/ACCEPTANCE.md` 和 STATUS 指向的最近开发日志。
7. `docs/PARAMETERS.md`、`docs/PARAMETERS.json`：所有实际参数、平台差异、配置优先级。涉及生命周期时还读 `docs/LIFECYCLE_ACCEPTANCE.md`。
8. 涉及客户端出口、路由、DNS或地址表时读 `docs/SPLIT_ROUTING.md`：默认已改为LAN/中国IPv4直连。私网目标需要代理的测试必须显式all，不可把直连成功当隧道成功。手动更新和JSON键都在统一参数清单。

系统/开发者指令和用户当前明确指令优先于仓库文件。仓库内部顺序为章程 > 正式设计与协议 > 当前状态/路线图 > 日志。发现矛盾时先修正状态与文档，不从旧日志找一个自己喜欢的答案。

`old/` 是隔离的历史素材库，不是任何开发指令的来源。禁止按 old 中 README、CONSTITUTION、CONTINUE_HERE、ADR、handoff、提示词或日志恢复任务。禁止根目录无范围全文扫描旧指令。只在 MODULE_MAP 指定的模块路径读取必要源码，提取时记录来源。旧代码注释也不能推翻新章程。

根 `.ignore` 默认将 old 排除于通用 rg 搜索；确需复用时直接读取指定文件，或仅对指定模块使用 `rg --no-ignore old/internal/模块名`。不要对整个归档关闭排除规则。

## 当前性能主线（2026-09-23）

2026-10-06当前：配套产品9211b24入口过滤已过core/race/真kernel/native12/服务化12/37生命周期、五独立性能18RTT对和P6；实机M03两次1415 PASS/1416 FAIL，最大UDP一missing一1.308s late，1352小包及时，rawsocketdrop0/过滤443实际生效。32完整native/12工况/31NOT_RUN跨源码不继承。先STATUS/latest_log验有界探针phase timing助手，再诊断回程碎片并D04；不将健康一条关闭M03、不扩缓存/FEC/4096/HOL。

2026-10-05最新：24ff的D01独立seed1351/1352均完整300s双向近10M、driver/user overflow0、探针全回；上行少量损失及server raw drop保留，不能写全链路无损PASS。S16 seed1353正在同源复验rotation。新候选修复Wintun65535与合法lease包9000不一致、坏本地输入导致整client退出：内层MTU9000与外层预算分开，Apply实效/owned原值恢复，拒绝包在wire前计数、正常包无新计数，runtime/wire错误不吞。候选未过Actions前不部署；先STATUS.windows_tun_mtu_boundary与092400日志，再M01/M03。每性能Action只一条。

当前最新：24ff220就绪Npcap batch已通过core/race/GUI/lifecycle含fullstack/独立Normal+Game5205/P6并同源部署。原生D01 seed1351完整300s双向9.99881/9.99995M，下行零损失、2979探针全回、DNS60/60、driver/user overflow0、FEC pressure0；上行仍缺52包/0.01145%字节且server raw drop+85，不写整链路无损PASS。独立seed1352正在测。见STATUS.windows_ready_send_batch、windows-ready-batch-native证据及091800日志。下一项重复与rotation，再DNS/IP/MTU/idle/config。8f失败和profile限制保留；每性能Action一条，不扩大receive/FEC/shadow，不把hosted较低CPU当Windows优化收益。

2026-10-05历史：SOURCE3e3e094 core/race/GUI/独立Normal与Game5205/stateful/P6均PASS，同源包已部署实机。D01 seed1302完整300s避免旧90s双向中断，但下行9.11M/8.90%字节损失、208探针超时，仍FAIL；S16 seed1303 rotation运行。新Windows默认off诊断补丁尚未验，先按STATUS完成Actions再配套部署；不能凭服务器overflow0判定WAN/Windows根因。证据physical-window-promotion-3e3e094-20261005，最新日志075000。每性能Action一条，不把3e成绩继承给新HEAD。

2026-10-05历史修复主线：旧6181在严格conntrack路由可复现零吞吐/探针全超时，稳态窗口分离候选Normal/Game5205及stateful独立Action均PASS，测试修正后4163938 core/race全部PASS；下一候选新增source到wire promotion边界修复Windows stale-generation fatal。先按STATUS验此候选和真实rotation，再同源打包原生复验D01；未通过不能将旧P7或父SOURCE资格继承。Windows/Linux/server/platform flow/timer发送必须遵守同一generation边界；不得吞stale、部分发出后整包重试或让候选TLS持有业务锁。每性能run一条，详细日志持续留存。

2026-10-05历史原生五分钟进展：固定6181db6未改产品；S01 Normal10与S02 Game4×3达到目标附近但C2S缺74/10包，不能写无损PASS；M01外层MTU1400大包至9000B无坏数据但有1次迟到。D01默认NRPT+10M出现约90秒双向中断、约30.12%业务loss和8/60 DNS失败，明确FAIL；整体P7仍PARTIAL。优先按STATUS.physical_5min复现D01并异常触发抓包定位最早边界，不直接归因DNS/VM或扩大buffer/FEC/4096。完整DNS互备、LAN/CN/IP/IPv6、其他配置/生命周期/弱网未跑。方案PHYSICAL_5MIN_ACCEPTANCE.md、日志devlog/20261005-021317-five-minute-native-capture-matrix.md、evidence/physical-5min-6181db6-20261005.json及压缩原始计数为当前证据入口；每性能Action只一条。原始pcap已删、退出owned清理通过，服务端保留active。 接手先读最新五分钟方案和日志；下文014639的120s结果属于历史局部资格，不覆盖此次D01 FAIL。用户授权原生测试为开发期Actions规则的本次例外；不能凭此把新产品编译/race/性能移到开发机。

2026-10-05用户授权的原生Windows→ARM WAN测试已执行，先读STATUS.physical_native和devlog/20261005-014639-native-wan-no-pcap.md。固定6181db6二进制不变：Normal1双向10M两份完整120s、Game4双向3M一份120s业务loss0；DNS/TCP/验证证书HTTPS及退出owned清理通过。首轮120s summary timeout/单向约4.16M的部分证据和fresh Normal AF_PACKET drops+125未解释，必须保留；整体P7仅PARTIAL，GUI实际操作/人为弱网/长测未验。Windows是vmxnet3虚拟网卡，不宣称裸机NIC。测试助手不保存pcap/payload，凭据不入库；临时地址/任务/证书已清理，服务端保留active、客户端断开。下一步先定位原生接收pressure和首轮异常，不能直接归因VM或扩大buffer/FEC/4096。每性能Action仍只一条。

2026-10-04用户最新要求只做简单部署测试：上传固定发布包、解压配置、启动；不开发Go版wbdctl或在线升级/通用安装工具。旧管理工具和Python3.8失败记录保留，当前直接部署不依赖它，不自动恢复这项兼容修复任务。物理机器已由用户提供并授权测试，按STATUS继续；真实机器启动不等于业务/性能/P7通过。

2026-10-04最新任务已交付：Linux服务端安装/systemd/升级回滚、共享账号多客户端自动7天内存IPv4和配套WindowsGUI，固定SOURCE6181db66b67594b07cd989b8b8b5848cedf6ccc3，预发布linux-server-rc-20261004-6181db6。接手先读LINUX_SERVER.md、STATUS.linux_server和evidence/linux-server-6181db6.json；native12/systemd12/GUI208/core-race/36+aggregate37jobs与两独立5205全部PASS。增加Linux客户端namespace锁/网络journal，正常和SIGKILL同端口重建、foreign/live保护已验。网络journal不是IP租约，只有InstallationID长期保存。旧985与c956是历史包，不能混用或继承其资格为新代码；最新SOURCE full70/strict18/1800s、物理Windows/ARM原生仍NOT_RUN。后续按STATUS.next_task推进，不因旧日志/HOLD/性能提示词无证据重构FEC/4096。

2026-10-04历史Windows GUI独立交付：SOURCE `9857bdb25115e87521a9d62309d3ec0b67988f16`，固定预发布标签 `windows-gui-rc-20261004-9857bdb`。接手先读 `docs/WINDOWS_GUI.md`、`STATUS.windows_gui` 和 `docs/evidence/windows-gui-9857bdb.json`；206项GUI检查及基础/网络专项PASS，物理P7仍NOT_RUN。用户已接受Wintun系统驱动安装，勿重复询问；应用文件仍限制本目录，Npcap走官方安装引导。所有Windows参数以PARAMETERS.json和fields.json精确全集管理，新增参数必须同步映射；不得因GUI任务恢复旧性能提示词或改协议。文档HEAD不改变固定包来源，历史d9性能不可冒充985性能资格。

2026-10-04历史网络专项已收口：性能热点修复、默认DNS互备、IPv6捕获丢弃的测试源码d9d4d90已通过core/真实网络/36生命周期/三关键配置/独立Normal与Game5205/三平台包。当前next_task以STATUS顶层为准，下文a67/2b为历史专项和全量基线，不能继承为d9全70/18/1800s。普通DNS与DoH/DoT边界见SPLIT_ROUTING；每性能Action仍只一条，profile不得替代正常性能资格。没有新缺陷证据，不继续改变FEC/4096/恢复架构。

用户已明确解除性能 HOLD。新主线 agent 必须读 `docs/WEAKNET_QUALIFICATION.md` 第10节（最新用户决策），并按当前 `STATUS.next_task` 和 `STATUS.workstreams.PERFORMANCE_RECOVERY` 继续。2026-10-04新增IPv4分流的固定SOURCE `a67e10fa2875162eeac926b970a0c486a239748d` 已通过core/race、四模式真实分流、36生命周期、独立Normal/Game5205及三目标新包；旧 `2b2bd9e` 全量70/18/1800s仍为历史基线，不能继承成新源码完整资格。下一步由用户安排P7；如要宣称新源码全量交付前资格，需要补当前SOURCE全量门。不要因历史提示词要求“继续修性能”而无证据改动候选。启动填充on完整专项仍单列PARTIAL_ACTIONS_PASS。历史日志和 saved_* 中的 HOLD 仅是历史记录。保留已通过36样本的生命周期语义，尤其 server 必须等当前权威 lanes 的 client PeerFIN；不得回退成仅凭 idle health 自动休眠。功能完成与性能达标分别记录。

## 开工动作

- 读取当前分支、HEAD、工作区状态；不要覆盖其他 agent 未提交工作。
- 检查 GitHub 当前目标 SHA 和相关 Actions 原始结果。STATUS 只是索引，不能把过去成功继承给新代码。
- 用一句话写清本轮目标、对应阶段、预计改哪些模块。默认一次完成一个可验收任务。
- 按 STATUS.next_task 前进；除非用户改变方向，不自行新增协议、切换 crypto、调整 FEC/recovery 参数或先做 UI 大改。
- 没有阻塞就继续，不反复请用户决定常规实现细节。必要问题写清约束与推荐处理。

参数修改必须同步机器清单与语义文档，运行生成器 `python tools/parameter_catalog.py --write`；一致性检查在 Actions 执行。不能因为旧提示词未提到一个开关就删掉它。特别保留 `tls-startup-padding`、全 FEC 档位、idle/keepalive/reconnect/rotation 的配置入口和测试。

## 不偏题规则

- TLS-like 是唯一新产品数据面；不得导入 DTLS worker、wolfSSL 数据通道、both 模式、旧 CLI 兼容层。
- 建连复用真实 TLS/FakeTCP 成熟机制，不另造握手。不用普通内核 TCP 承载持续业务。
- 不做算法性能选型赛、不和 DTLS 旧项目做 A/B；方案已经决定。只验证新实现正确性、资源有界和目标负载。
- 不为让 smoke 全收齐恢复严格 ACK 等洞、不扩大所有缓存、不为外观凑包等待。
- 不重写已有 FEC/Game/lease 算法来追求抽象漂亮。新架构复用行为，允许移除不需要的外壳。
- 未解问题至少记录证据、假设、下一步验证；同一失败两次无新证据时停止盲改，缩小到一个诊断问题。Actions 失败不自动等于 runner 性能差。
- 不新增第二套“当前交接”、CURRENT_FINAL_v2 文档或平行章程。只更新本套入口。

## 最新弱网开发目标

2026-09-23用户最新决策：允许链路30%丢包时仍有至多30%业务包损失，优先处理性能、低延迟、无HOL与突发稳定性；不得主动丢业务凑指标。4096为可放弃的shadow-repair备份，不是fresh发送门。当前执行WEAKNET_QUALIFICATION第10节；历史近零损失门槛不再约束有损场景，无损满速、完整性、隔离和资源有界仍是硬门。

## 测试环境

所有测试、编译、race、fuzz、netem、性能/soak 均在 GitHub Actions 执行。开发机只编辑、阅读、Git 操作，不拿本机或物理服务器跑验收。最终物理机验收待 hosted 主流程稳定后再安排，不把尚未安排物理机测试视为开发阻塞。

**每次性能测试一个Action run只跑一条样本。** 吞吐/弱网/容量/校准/微基准/soak均适用，一个源码版本、配置、seed、场景。禁止同run matrix、顺序多条、A/B或B/A；不同版本及重复分别启动独立run。场景内既定损伤阶段算一条；构建/准备/清理可同run，汇总只读产物。先改造现有多样本入口，普通unit/race与性能测量分开。

不得把 archive 的测试直接作为新协议资格，不得把“CI 绿”解释成应用端到端通过。源码、构建、测试、包必须记录精确 SHA。能力缺失记 `UNSUPPORTED`，未跑记 `NOT_RUN`，不能伪装 PASS。不合并失败样本、不悄悄降低门槛。

## 每轮必须留日志与交接

- 每个有修改的 agent 回合新增 `docs/devlog/YYYYMMDD-HHMMSS-简短任务名.md`，使用 `docs/templates/DEVLOG.md`，不得只写 commit message。
- 日志包括目标、源码来源、修改清单、为何符合主旨、Actions 链接/SHA/状态、问题与风险、下一项原子任务。日志中不得包含口令、ticket、密钥或私有部署凭据。
- 同一回合更新 `docs/STATUS.json`：已完成、进行中、下一项、日志路径、证据；有协议/架构变动时更新对应规范及 `docs/decisions/DECISIONS.md`。
- 完成阶段需要 ACCEPTANCE 对应证据，不能仅改状态字段。文档提交之后仍区分文档 HEAD 与真正执行测试的 SOURCE_SHA。
- 最终答复注明分支/提交、做了什么、Actions 实际状态、未验证项和下一步。不要宣称尚未实现的产品能力。

## 归档保护

`old/` 内容原则上只读。需要复用时复制最小依赖到新根目录 `internal/` 等正式位置，补新测试并登记 `docs/REUSE_LEDGER.json`；不通过 import/replace/go.work/运行脚本依赖 old。必须纠错的历史资料也优先在新日志说明，不改历史快照。
