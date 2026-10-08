# ACK callback entered 握手消除测试断言竞态，新增 Actions 定向重复保护（2026-10-09）

只在 `next/performance-efficiency-20261008`，父HEAD `3c2336dee4509d994d6e1b685f0cbacc3ccddada`；规范主线未动，无物理机，无性能或产品运行时变更。产品SOURCE仍固定 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，新helper准确身份为本次GitHub commit GITHUB_SHA（不得写成源码性能收益）。前一证据归档 [E4 Game4 run37853468730](../evidence/performance-efficiency-e4-game4-5205-off-run37853468730.json) PASS_SCOPED_ACTIONS，不变更旧结果。

[原foundation37853468530](https://github.com/lly8666/wobuzhidao/actions/runs/37853468530) 的`go test -race ./...`明确失败测试为`TestACKFeedbackBlockedWriteKeepsNoHOLAndOnlyLatestACK`。原测试 `acks <- seg` 早于 `calls.Add(1)`，消费端读到 channel 消息不保证计数已更新。日志`ACKWorkerAttempts=1 ACKWorkerRunning=true ACKWorkerPending=true calls=0`与此时序一致；未见 race detector 的 data race stack，故只能认定**断言竞态待证实**，不可当作产品已无错或race PASS。

本提交只改 `internal/runtimeowner/ack_feedback_test.go`：首个emit callback先记录 `calls.Add`，再发第一个ACK，关闭独立 `entered` channel 后才阻塞在 `release`；测试显式等待 `entered` 后再断言业务0、2、1首次无HOL交付、100次重复ACK合并/一个在途任务/latest-only以及close后不会重放第三个ACK。仍保留bounded worker、真实阻塞emit、latest ACK精确seq、关闭和失败的硬断言；不靠sleep、不松门。不动`internal/runtimeowner/ack_feedback.go`产品实现。

`.github/workflows/next-foundation.yml`在 Linux full race 前新增：`go test ./internal/runtimeowner -count=100` 同组ACK五测试以及 `go test -race ... -count=30` 同组，保持原`go test -race ./... -count=1`；全部只在 Actions 执行，非性能样本，属于普通unit/race功能CI。已有Windows、Linux TUN/TPROXY及lifecycle触发不被当作真实性能PASS。

**尚待Actions结论**：此提交后的新run必须查看原始作业/失败日志。如果发现仍有产品问题，改产品并重跑；未通过不写PASS。下一步在race过门后才对固定产品 ba8ed1 以每run一case进行 Game4 lossless 同seed1840，Normal1 0loss/5205、Game2 0loss/5205，diagnostic ON/低RTT单独证明。所有CPU收益UNPROVEN，旧 Game4 UDP446/AF_PACKET140 drop及TCP-only失败、E7下行80秒OPEN、E6/P6/实机NOT_RUN。
