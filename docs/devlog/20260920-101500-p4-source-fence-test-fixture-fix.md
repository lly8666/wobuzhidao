# 20260920-101500 P4 source fence existing-test fixture fix

## 失败证据

SOURCE_SHA：
`d7ca5ce1d393499429048cbd9cb41aed6b46bd9c`

Actions：
https://github.com/lly8666/wobuzhidao/actions/runs/35483439066

顶层：`completed / failure`。

结果：
- repository-contract：PASS
- P2 kernel fallback + continuous pcap：PASS
- Windows unit/build job：FAIL at unit
- Ubuntu unit/build job：FAIL at unit
- race/fuzz/reference：因 unit failure 未运行

Artifacts：
- foundation：10596971243
- p2-kernel-fallback：10597300736

## 具体失败

Linux/Windows 都命中既有测试：

`TestLeasedOwnerPreservesIdentityAcrossReplacementDormantAndWake`

日志：

```text
tunnel_identity_test.go:76: logicaltunnel: invalid IPv4 packet: length=6
```

本轮新增 `internal/logicaltunnel/source_test.go` 所在 package 已 PASS；没有 validator 自身失败。

## 根因

上一轮 identity/lease 原子任务中的 leased client 测试，为了只验证 owner lifecycle，直接使用：
- `[]byte("flow-a")`
- `[]byte("flow-b")`
- `[]byte("awake")`

作为 `BusinessFlow.Outbound` 输入。

本轮产品契约明确要求 leased client 的业务 packet 必须是 source==lease 的合法 IPv4，所以这些旧字符串被新 fence 正确拒绝。该失败说明旧测试夹具与新产品契约冲突，不是应当放宽 anti-spoof 的理由。

## 修复

只更新 `internal/datapath/tunnel_identity_test.go`：
- 从该测试的 `lease.Config.LeaseIPv4()` 取得 lease address；
- 用本轮共享测试 helper 生成 well-formed IPv4；
- source=lease；
- 原有 replacement/DORMANT/wake 断言保持不变。

DORMANT 场景仍保留任意 payload，因为 `normalBinding` 在任何 source parsing 前必须返回 `ErrTunnelDormant`；该断言继续确认 dormant precedence。

产品源文件：
- `internal/logicaltunnel/source.go`
- `internal/datapath/tunnel_owner.go`

在修复提交中不改变。

## 下一步

新提交必须重新跑完整 Actions，不能继承 d7ca5ce 的任何 PASS 作为产品资格。只有新 exact SHA 全绿才做 source anti-spoof evidence closure。
