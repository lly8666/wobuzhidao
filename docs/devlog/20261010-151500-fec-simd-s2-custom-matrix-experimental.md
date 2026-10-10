# FEC SIMD S2 — 自定义矩阵整块多输出（仅实验选项）

- 本步父 SOURCE：`94834550aeef01e2720a2904c5bb9f5a13e7b287`；S1 foundation run [38032702483](https://github.com/lly8666/wobuzhidao/actions/runs/38032702483) 已触发，写此日志时 Windows amd64 unit/build job 成功，Linux与race尚未核完。没有全面 S1 PASS 或 CPU gain 声明。
- S2 保持默认后端为 S1 span；仅 Actions 的 `-tags=wbd_fec_fused` 可打开完整组(20 shards)的融合引擎，且需要 CPU 矢量能力。未证实真实收益前绝不把融合当默认。该选项非 GUI/用户配置参数。
- 每个 20:P（P=4/8/10/12/16/20）分别只传 **P 行** 原 WBD systematic generator parity 矩阵给 `reedsolomon.New(20,P, WithCustomMatrix, WithMaxGoroutines(1), WithInversionCache(false))`；P 行不可错误传20行进较小实例。采用 sync.Once 进程内每档一个共享实例，无块内 new/goroutine/timer，理论上最多六实例且无无限求逆缓存。partial N<20 沿用S1只读活跃source/写 min(N,P) parity。
- 将`FastBlockEncoder`对具体 `*FastReedSolomon20x20` 的判断改为显式 `IgnoresInactiveSources` 读集合能力；不声明即按原generic语义清零，已有inactive clear/owned负例和新增未知active codec负例保护。systematic立即送、流控、固定密文、期限/退役、LINK MTU 全不触及。
- Golden：六档多网络片长的旧参考 parity 全等、未用parity不得被写、partial与整块混合、active清零负例。新增六档真实片长的span/fused benchmark，仅解释内核，正式收益待真实 ABBA。
- 新 scoped Actions：`.github/workflows/next-fec-simd-core.yml` Windows native+x86 native+Linux ARM64 native（若runner不支持如实写NOT_RUN），测试normal/scalar/noasm/fused、部分race与ARM32/ARM64 cross-compile。此处仍 **NOT_RUN/NOT_MEASURED**，不得用构建证明NEON执行。
- 仍保留全部历史FAIL、NOT_RUN；Game4/旧300ms TCP-off/约80秒下行OPEN，物理NOT_RUN；下一步S1/S2结果复核、微基准、按方案S3–S5新独立真实业务paired fixture/资格。
