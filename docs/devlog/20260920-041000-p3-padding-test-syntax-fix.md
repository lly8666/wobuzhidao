# 20260920-041000 P3 padding 测试语法修复

## 失败证据

第一阶段 padding 实现 SOURCE_SHA：

`046d84e11561aef74578b7c1ede3b6e05306b001`

Actions：

https://github.com/lly8666/wobuzhidao/actions/runs/35461114443

顶层：`completed / failure`。

jobs：
- repository-contract：PASS
- p2-kernel-fallback：PASS
- active-go-tests (ubuntu-24.04)：FAIL
- active-go-tests (windows-2022)：FAIL

Linux job 105945053296 日志明确：

```text
internal/datapath/padding_test.go:58:14: string literal not terminated
FAIL github.com/lly8666/wobuzhidao/internal/datapath [setup failed]
```

该失败发生在整仓 unit 编译阶段，因此 Linux race、tlsrecord directed fuzz、independent reference generator 和 reference artifact 都没有运行。该 SHA 不能作为 padding 产品资格，也不能用于静态 padded vector 来源。

## 修复

只修 `internal/datapath/padding_test.go` 的测试诊断字符串，将源码里的真实多行 string 改为 Go 字符串中的 `\n` 转义。

没有修改：
- padding wire 格式；
- tlsrecord Seal/SealWithPadding；
- unified MTU headroom；
- FEC/LINK；
- datapath owner 语义；
- P2 faketcp/realityfront；
- 默认 padding=0。

## 资格计划

本修复提交仍不是最终 padding PASS 候选。

1. 新 SHA 完整 Actions 先证明实现/专项测试/race/fuzz 通过，并由独立 `tools/tlsrecordvector` 生成 `pn7_padded_tailzero_p9`。
2. 从该精确 SHA 的 job output/artifact 读取 padded wire hex。
3. 把该 hex 钉入 `internal/tlsrecord/vector_test.go` 静态 expected bytes。
4. 再生成一个最终候选 SHA，跑完整 Actions。
5. 只有最终 SHA 顶层 `completed / success` 后才写 padding PASS。

P2 CLOSED 与此前 P3 LINK/FEC/MTU/session-owner PASS 证据均保持不变。
