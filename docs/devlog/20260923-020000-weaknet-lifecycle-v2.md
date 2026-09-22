# 20260923-020000 弱网生命周期移植与参数入口

## 本轮目标和阶段

用户要求暂停主线，将旧弱网自适应退役、keepalive 容错、双边黑洞恢复、idle/wake 防误判移入 NEXT，并使所有新 agent 都能发现现有参数。起点 next/tlslike-dataplane 的 a045a28，先提交 1523ce5 主线 HOLD。用户随后确认另一开发 agent 已暂停。其最新 a045a28/c9a9691 等只改资格框架和 AF_PACKET 观测，无产品代码直接冲突，全部保留。

## 修改与原因

- runtimeowner 移植 rate/RTT 接收压力退役；保持 4096、3s repair、新流量 1/5 credit、no HOL/late-first-arrival 与不可变密文重传。
- tlsrecord/datapath 新增独立 40B health，绕过 LINK/FEC/padding，不分配业务 flow、不赚 repair credit、不加入 repair 队列；用新 PN 与认证接收判断健康。服务端须先收到对端有效 steady record 才开始保活。
- runtimeentry 客户端一个候选、超时失活、有限 jitter backoff、可重试错误诊断；候选失败不终止进程。纯下行刷新业务活动，本地需求在发送前登记；missing keepalive 不等于 idle；自动休眠做活动快照复核。唤醒失败也退避。部分 Game 唤醒允许缺失 lane 在别的 lane retiring 时补齐，仍有同 ID 排他和既有物理上限。
- 新控制类型通过 admission V2 明确版本边界，两端同时升级；真实 TLS/decoy/稳态 record 外观未更换。历史 V1 通过证据不继承给 V2。
- Linux/Windows 正式 CLI 新增同名 JSON 配置。PARAMETERS.md/json 为必读，源码清单自动一致性 gate；tls-startup-padding 默认 off 且进入 JSON。每个平台参数差异明确记录。
- 新增 core Actions workflow、health/pressure/config/blackhole+失败候选/idle竞态测试；既有 closure 测试使用显式 1s keepalive，保留双端休眠、FIN、lease、no-HOL 原断言。详细真实进程矩阵见 LIFECYCLE_ACCEPTANCE。

## 复用来源

old 冻结源 b5c848f；读取 old/internal/faketcp/adaptive_pressure.go，适配到 internal/runtimeowner；old/internal/windowsruntime/controller_idle.go 的活动快照复核原则适配到当前进程内生命周期。REUSE_LEDGER 已登记。不导入 old 包/旧 controller/DTLS，不用旧提示词恢复任务。

## Actions 证据

本提交时 NOT_RUN；push 会触发 next-lifecycle 定向核心和既有 foundation。不得将脚本/用例已写成视为通过。所有测试/编译/race/fuzz 留在 Actions，本机仅编辑、格式化、参数清单生成和 Git。完整真实弱网/生命周期与物理环境仍 NOT_RUN，由用户指定的新 agent 执行。后续结果必须写精确 SOURCE_SHA/run/job/artifact。

## 问题、排查与风险

旧主线上行吞吐问题未解决，a045a28 AF_PACKET 诊断上下文保留。不能因新增生命周期功能就宣称性能修复。V2 要求端点成对升级；peer hint 是有时效证据，丢失会保守延后 idle，并非分布式可靠关闭事务。已 promote 后不回滚旧 generation，后续异常再建新 incarnation。休眠后没有服务端 out-of-band 唤醒客户端机制；需要持续被动接收则禁用 auto idle。

核心与 race 尚未运行，尤其关注生命周期/timer/关闭竞态、partial wake 的 server state 收敛、health 对抓包成本统计的影响及严格负载退化。未达到验收门槛前保持 IMPLEMENTED_PENDING_ACTIONS。

## 下一项原子任务

新 agent 按 LIFECYCLE_ACCEPTANCE 依次验收 core、真实进程生命周期、10M 单 lane/3M 四 lane 弱网质量与开销，修复本专项缺陷并精确 SHA 重跑；完成后直接回写 DEVELOPMENT_PLAN、ACCEPTANCE 和 STATUS 的专项 COMPLETE。原主线 HOLD 不自动解除。每轮记录详细开发日志，参数变更同步清单，不扩大缓存、不恢复 HOL、不以 runner 性能为无证据借口。
