# 新分支：Normal自动FEC / AES / TUN分流 / 质量显示方案

## 目标和精确来源

用户要求从FEC优化分支新建工作分支，归档精简旧计划，制定完整方案并提供新agent提示词。实际新分支 next/adaptive-fec-aes-tun-20261010，父文档HEAD a8913e3b651413b6d37c3a7732a0baa375009da0，继承产品SOURCE 7fb98fab79834a351a1dbe04eebb207f66bea28b。本提交只文档/STATUS，不改任何产品源码/Go依赖/工作流/试用部署。

## 本轮确定

N0一次受保护V3协商per-client策略/密码，N1双向低频质量，N2 Normal普通/激进自动FEC，N3标准库AES128/256与实际TLS/decoy值，N4Windows少量routes/TUN内分流/native direct/幂等取消清理，N5中文GUI两秒指标与catalog，N6真实工况/配置/合理门/P6。

保留systematic立即/无HOL、partial min(k,R)/32ms/3s、fixed Game、有限shadow same-wire、MTU/身份隔离/owned状态。WebRTC只借控制骨架；AES用标准库跨平台硬件/generic；sing-tun参考system-stack，不搬完整媒体/代理栈。

## 归档和防污染

父STATUS及17个入口/方案快照完整字节归档，旧devlog/evidence/source/workflow未删；当前STATUS从194KB精简为当前任务+来源/OPEN索引。历史计划与旧模板替换成只读导航，根入口/章程/路线/连续性/验收同步；V2 wire/模块/分流/夹具加明确新计划与现有能力边界。未改PARAMETERS实际目录，新参数只是提案，开发时再同步真实生效。

## 验证与限制

本地仅文档JSON/路径/归档字节和Git diff审计，无编译/Go/产品测试。本分支产品/性能/physical均NOT_RUN，旧SIMD Q2与native失败原样保留，80秒S2C继续延期。push基础Actions自动触发如实核验，不主动dispatch性能或部署。新参数未实现，不假装生效。

## 下一步

新agent按STATUS N0开工：审admission/Tunnel FEC接线，先补V3字节协议/KDF向量和多客户绑定，再Actions unit/race；逐原子步骤留devlog+STATUS。所有性能每Action单样本，本分支无历史串行例外。
