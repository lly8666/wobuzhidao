# 20260919 P0 Actions回执与默认搜索隔离

## 本轮目标和阶段

P0收口。首个提交3335b13a913cbd4b1adc689d75605b159280afac已经推送到next/tlslike-dataplane，本次只写回真实证据并增加通用搜索排除。

## 修改与原因

增加根.ignore排除old，降低全新agent通用rg搜索误读旧指令的概率。AGENTS保留指定模块的精确读取入口。README不再硬编码某一旧日志，统一指向STATUS.latest_log。

## 复用来源

无产品代码迁移，REUSE_LEDGER仍为空。旧快照已经由Actions逐Git对象比对原始b5c848f确认，非运行测试。

## Actions证据

- SOURCE_SHA：3335b13a913cbd4b1adc689d75605b159280afac。
- Run：https://github.com/lly8666/wobuzhidao/actions/runs/35433878751。
- repository-contract：PASS，包含归档完整性/交接入口检查及foundation receipt上传。
- active-go-tests：SKIPPED，因为根目录尚无新产品module；不是产品测试PASS。
- artifact：foundation-3335b13a913cbd4b1adc689d75605b159280afac。
- 本提交将再次触发foundation。STATUS保留已验证SOURCE_SHA，不提前声称本提交已测。

## 问题、排查与风险

P0只证明仓库组织可继续开发。新产品尚未实现，网络/no-HOL/性能/物理验收均NOT_RUN。old不是安全沙箱；隔离依靠目录、入口指令、默认搜索排除和Actions防止重新依赖归档，不能声称agent不可能误读。

## 下一项原子任务

P1创建根Go module并实现WIRE_SPEC的keys/seal/open向量。新agent先读AGENTS和STATUS，不进入old交接，不执行本地测试，不恢复DTLS兼容或算法选型。
