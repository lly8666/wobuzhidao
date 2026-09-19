# 20260919-190600 P2 Bootstrap Actions回执

## 本轮目标和阶段

收口P2的第一个原子子任务，只写回BootstrapStream最小提取的真实Actions证据并推进STATUS.next_task。真正执行测试的SOURCE_SHA为23cef50c0f50a5ca2c8ab7b752cadfe1ac9acdc3；本回执文档HEAD会晚于该SOURCE_SHA。

## 修改与原因

不修改产品代码。更新STATUS，将BootstrapStream最小闭包标记为已完成子任务，并把下一项限定为Sender/Pending的bootstrap retransmit/ACK wait最小闭包与prepare/detach阶段边界。

继续保持范围：
- 不整包迁移old/internal/faketcp/arq.go。
- 不提前迁移steady-state SACK/RACK/repair pressure。
- 不接FEC。
- 不把基础Go测试解释成P2真实握手资格。

## 复用来源

本回执不新增代码复用。上一提交已登记：
- old/internal/faketcp/bootstrap_stream.go -> internal/faketcp/bootstrap_stream.go
- old/internal/faketcp/arq.go -> internal/faketcp/sequence.go，仅seqLT

REUSE_LEDGER保持上述最小闭包记录。

## Actions证据

SOURCE_SHA：23cef50c0f50a5ca2c8ab7b752cadfe1ac9acdc3

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35436393242

顶层结果：completed / success。

实际job：
- repository-contract：PASS
- Windows 2022 active-go-tests：go list / go test ./... / go build ./... PASS
- Ubuntu 24.04 active-go-tests：go list / go test ./... / go build ./... PASS
- Ubuntu race：PASS
- Ubuntu既有tlsrecord directed fuzz：PASS
- independent P1 reference generator与artifact上传：PASS

新增internal/faketcp/bootstrap_stream_test.go属于go test ./...范围，因此其乱序重组、ACK-gated chunk、deadline、chunk/byte上限、uint32 wrap、marker scope、Close/EOF断言均实际执行并通过。

## 问题、排查与风险

本子任务没有出现Actions失败。成功仅说明最小BootstrapStream提取在当前根module的基础构建、unit和race中成立。

仍未验证bootstrap retransmit与Sender的2秒ceiling集成；old/bootstrap_retransmit_test.go依赖Sender/Pending路径，已明确留给下一原子任务。也未实现TLS reader预读边界、prepare/detach、最终应答ACK或首条新record阶段切换。

## 下一项原子任务

定向审查并提取old/internal/faketcp/arq.go中仅服务bootstrap的Pending/Sender路径与必要调用方，补bootstrap retransmit/ACK wait测试；同时设计prepare/detach边界API。退出条件是Windows/Linux unit/build和Linux race在Actions通过，且不带入steady-state完整ARQ/FEC。
