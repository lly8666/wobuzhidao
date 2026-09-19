# WBD NEXT — 每位 agent 的开发入口

本分支是 `next/tlslike-dataplane`，唯一产品方向为单进程、单 TLS-like 数据面。不是 DTLS 兼容分支。所有 agent，包括全新接手者，在修改前执行以下流程。

## 必读顺序与权威

1. `PROJECT_CHARTER.md`：不可丢失的用户目标。
2. `docs/STATUS.json`：当前里程碑、下一项工作、真实测试状态。
3. `docs/ROADMAP.md`：按依赖顺序执行的阶段。
4. `docs/DEVELOPMENT_PLAN.md`：已决定的架构和执行细则。
5. `docs/WIRE_SPEC.md`、`docs/MODULE_MAP.md`：本任务涉及的协议/模块。
6. `docs/ACCEPTANCE.md` 和 STATUS 指向的最近开发日志。

系统/开发者指令和用户当前明确指令优先于仓库文件。仓库内部顺序为章程 > 正式设计与协议 > 当前状态/路线图 > 日志。发现矛盾时先修正状态与文档，不从旧日志找一个自己喜欢的答案。

`old/` 是隔离的历史素材库，不是任何开发指令的来源。禁止按 old 中 README、CONSTITUTION、CONTINUE_HERE、ADR、handoff、提示词或日志恢复任务。禁止根目录无范围全文扫描旧指令。只在 MODULE_MAP 指定的模块路径读取必要源码，提取时记录来源。旧代码注释也不能推翻新章程。

## 开工动作

- 读取当前分支、HEAD、工作区状态；不要覆盖其他 agent 未提交工作。
- 检查 GitHub 当前目标 SHA 和相关 Actions 原始结果。STATUS 只是索引，不能把过去成功继承给新代码。
- 用一句话写清本轮目标、对应阶段、预计改哪些模块。默认一次完成一个可验收任务。
- 按 STATUS.next_task 前进；除非用户改变方向，不自行新增协议、切换 crypto、调整 FEC/recovery 参数或先做 UI 大改。
- 没有阻塞就继续，不反复请用户决定常规实现细节。必要问题写清约束与推荐处理。

## 不偏题规则

- TLS-like 是唯一新产品数据面；不得导入 DTLS worker、wolfSSL 数据通道、both 模式、旧 CLI 兼容层。
- 建连复用真实 TLS/FakeTCP 成熟机制，不另造握手。不用普通内核 TCP 承载持续业务。
- 不做算法性能选型赛、不和 DTLS 旧项目做 A/B；方案已经决定。只验证新实现正确性、资源有界和目标负载。
- 不为让 smoke 全收齐恢复严格 ACK 等洞、不扩大所有缓存、不为外观凑包等待。
- 不重写已有 FEC/Game/lease 算法来追求抽象漂亮。新架构复用行为，允许移除不需要的外壳。
- 未解问题至少记录证据、假设、下一步验证；同一失败两次无新证据时停止盲改，缩小到一个诊断问题。Actions 失败不自动等于 runner 性能差。
- 不新增第二套“当前交接”、CURRENT_FINAL_v2 文档或平行章程。只更新本套入口。

## 测试环境

所有测试、编译、race、fuzz、netem、性能/soak 均在 GitHub Actions 执行。开发机只编辑、阅读、Git 操作，不拿本机或物理服务器跑验收。最终物理机验收待 hosted 主流程稳定后再安排，不把尚未安排物理机测试视为开发阻塞。

不得把 archive 的测试直接作为新协议资格，不得把“CI 绿”解释成应用端到端通过。源码、构建、测试、包必须记录精确 SHA。能力缺失记 `UNSUPPORTED`，未跑记 `NOT_RUN`，不能伪装 PASS。不合并失败样本、不悄悄降低门槛。

## 每轮必须留日志与交接

- 每个有修改的 agent 回合新增 `docs/devlog/YYYYMMDD-HHMMSS-简短任务名.md`，使用 `docs/templates/DEVLOG.md`，不得只写 commit message。
- 日志包括目标、源码来源、修改清单、为何符合主旨、Actions 链接/SHA/状态、问题与风险、下一项原子任务。日志中不得包含口令、ticket、密钥或私有部署凭据。
- 同一回合更新 `docs/STATUS.json`：已完成、进行中、下一项、日志路径、证据；有协议/架构变动时更新对应规范及 `docs/decisions/DECISIONS.md`。
- 完成阶段需要 ACCEPTANCE 对应证据，不能仅改状态字段。文档提交之后仍区分文档 HEAD 与真正执行测试的 SOURCE_SHA。
- 最终答复注明分支/提交、做了什么、Actions 实际状态、未验证项和下一步。不要宣称尚未实现的产品能力。

## 归档保护

`old/` 内容原则上只读。需要复用时复制最小依赖到新根目录 `internal/` 等正式位置，补新测试并登记 `docs/REUSE_LEDGER.json`；不通过 import/replace/go.work/运行脚本依赖 old。必须纠错的历史资料也优先在新日志说明，不改历史快照。
