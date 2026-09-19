# 2026-09-20 P2 TCP新增测试静态修正

## 基线与问题证据

基线为 `5c074f1f2b3610370cae3f851218ff0be6c86699`。推送后静态复核新增Go测试时发现两处无需运行即可确定的编译语义错误：`Segment` 含 `[]byte` 不能直接用 `!=` 比较；循环中的 `time.Duration(i) * 2 * time.Second` 会形成两个defined duration值相乘。

## 修改

仅修正 `internal/faketcp/association_test.go`：逐字段比较SYN-ACK并用 `bytes.Equal` 比较payload；循环时间表达式改为 `time.Duration(i*2) * time.Second`。产品代码、P3目录、协议和验收门槛均不变。

## Actions / SOURCE_SHA

该修正提交创建前未在本地运行任何编译/测试。前一SHA若Actions已启动，其结果仍按原SHA保留；本轮资格只绑定承载本日志的新SOURCE_SHA。等待GitHub Actions原始结果。

## 下一步

先取得本SHA unit/build/race/fuzz结果；若通过再进入realityfront外观/票据任务，若失败按Actions错误继续收敛。
