# 修复 ACK 重复测试的 Foundation Actions YAML 无作业失败（2026-10-09）

仅 `next/performance-efficiency-20261008`，父HEAD `ffd1239f2399fb04361f02803d55e6225a890178`，产品源码冻结 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，无产品、协议、性能参数或物理设备改动。

[Foundation run37856284837](https://github.com/lly8666/wobuzhidao/actions/runs/37856284837) 在commit `ffd1239f2399fb04361f02803d55e6225a890178` 后 immediately completed **failure**，Jobs API 返回 `jobs=[]`。独立读取Git对象证实前一提交由插入含长正则的步骤破坏了 `.github/workflows/next-foundation.yml`：Go命令引号末尾和第二次定向-race/原full Race步骤遗失，工作流无有效job。此为 **WORKFLOW_INVALID / TESTS_NOT_RUN**，不是测试没通过，也不是产品race detector错误。原2026-10-08 run37853468530的ACK断言FAIL继续保留，不将最新坏工作流冒充修复已验。

本次从先前 **确切 `3c2336dee4509d994d6e1b685f0cbacc3ccddada` 版**读取完整基础workflow，仅在Linux full race之前插入短 `-run='^TestACKFeedback'` 步骤：`go test ./internal/runtimeowner -count=100`、`go test -race ./internal/runtimeowner -count=30`，然后继续原本的`go test -race ./... -count=1`；其余workflow与原文件字节一致。该表达式覆盖全部ACKFeedback测试，包括阻塞写入最新ACK、不HOL、close、piggyback和错误处理。旧改动的产品测试 `internal/runtimeowner/ack_feedback_test.go` 使用 entered握手仍保留，native Emit真实阻塞和统计严格断言未放松，不使用sleep。

本提交自动触发的Foundation原生job完成之前状态 **NOT_RUN/PENDING**；严禁绕过基础测试开启E4下一个性能样本。旧Game4 run37817466498/37851028041 FAIL、纯TCP37762364098、超大UDP边界未覆盖、E7延后下行中断与CPU收益UNPROVEN不变。