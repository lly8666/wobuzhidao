# 20260922-190000 有界 TLS 启动填充实现

## 本轮目标和阶段

用户要求实现仅对识别出的内部新 TLS 流有限填充，再交全新 agent 在 Actions 测试/修复，通过后直接更新主线进度。用户已暂停原主线任务。本轮从 next/tlslike-dataplane 的 896ae94bea1aa1e532b2a14ed9d3e707a72f5286 开始，P4/P5 小功能独立交付。

## 修改与原因

datapath 新增有界、纯旁观 startup detector；支持 leased IPv4 TCP 与 platformflow v1 数据 envelope，有限 ClientHello 结构解析/分段重排，绝对时间与流/字节/record 上限。接入 Normal/BusinessFlow/Game outbound 和 source-valid、去重后的 inbound；原始业务不等识别。现有 padding policy 新增 TLSStartupOnly，保留旧固定每 record API，随机有限请求共享既有 tunnel reserve/commit 预算；FEC parity 无额外填充。Tick 到期、Close 清理，rotation/DORMANT 不重置额度。

runtimeentry 四个 owner 创建入口接默认关闭策略，Linux/Windows client、Linux server CLI 增加开关。新增专项 Actions、识别/所有固定 FEC/双向/no-HOL/Game/concurrency/rollback/fuzz 测试，以及由 Go crypto/tls 生成真实 ClientHello 经 production platformflow serializer 的交叉模块测试。规范及详细验收见 TLS_STARTUP_PADDING.md。

## 复用来源

无新的 old 源码提取。直接复用当前根 internal/datapath/padding_policy.go、Lane seal、LINK/FEC、runtimeentry owner 创建与现有平台入口。REUSE_LEDGER 不变；握手/密码/wire/FEC/4096/recovery 不变。

## Actions 证据

基线 896ae94：targeted 35698760883 已 completed/success；foundation 35698760856 查询时 in_progress。仅作为开工审计，不属于此功能资格。

本次实现 SOURCE_SHA 为包含本日志的产品提交，最终以 GitHub 精确 commit 为准。本机仅编辑/格式化/Git diff 检查；所有本功能构建、unit、race、fuzz、fullstack 在记录时均 NOT_RUN，push 后自动触发专项和原有 workflow。新 agent 必须补写实际结果，不能继承基线 PASS。新 workflow 只承诺 core 资格，真实网络专项尚需补跑。

## 问题、排查与风险

本功能不消除方向/时序指纹。4KiB cap、首 record 内 CH、2s绝对期限会有有意漏识别；不能为此等待业务。每 tunnel 最坏 prefix 内存约4MiB，表满只旁路；需测多 tunnel 和 churn。source/headroom不足可能零填充，真实稀疏效果/开销未测。已有主线回归和真实路径高负载资格不得因本功能被宣称已完成。

## 下一项原子任务

全新 agent 依 TLS_STARTUP_PADDING.md 的关闭清单完成 exact-SHA Actions 测试、定向修复、记录原始证据；全部达到门槛后直接标小功能 COMPLETE。原主线任务保留暂停，不另建第二套进度文档。
