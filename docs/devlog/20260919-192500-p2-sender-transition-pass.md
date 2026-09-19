# 20260919-192500 P2 Sender/Transition Actions回执

## 目标

收口bootstrap Sender/Pending、ACK wait与StageTransition prepare/detach原子任务的真实Actions证据。真正执行产品代码测试的SOURCE_SHA为525175a19165d392b825a96e95594b0507aa2878。

## 失败与修复链

第一次产品提交cb6000400ce8fa5c90af68a8ee4910e8ab5fbbc4触发run 35437027533，但repository-contract在Go jobs前FAIL，原始错误为Missing reuse source。原因是REUSE_LEDGER source字段写成两个路径拼接说明，不是单一存在文件。

随后525175a19165d392b825a96e95594b0507aa2878只修复REUSE_LEDGER/STATUS/devlog，不修改Sender/Transition产品代码，因此成功run验证的产品代码与cb600040相同。

## Actions证据

SOURCE_SHA：525175a19165d392b825a96e95594b0507aa2878

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35437066295

顶层结果：completed / success。

实际结果：
- repository-contract：PASS
- Windows 2022：go list / go test ./... / go build ./... PASS
- Ubuntu 24.04：go list / go test ./... / go build ./... PASS
- Ubuntu race：PASS
- 既有tlsrecord directed fuzz：PASS
- independent P1 reference generator及artifact上传：PASS

因此本轮新增的bootstrap_sender_test.go和transition_test.go已在普通unit与Linux race中实际执行。

## 已验证行为

- bootstrap Pending重传使用2秒ceiling。
- bootstrap重传不扩大shared RTO；后续普通payload仍保持普通指数backoff。
- WaitAck可与并发ACK推进配合并遵守deadline。
- 重传保持原Seq与payload字节不变。
- cumulative ACK支持uint32 wrap。
- prepare后提前新record不会回Feed TLS，而进入64条/总wire字节有界队列。
- pure ACK不进入payload所有权路径。
- detach转交提前record；晚到旧bootstrap与新record分开分类。
- 同Seq同payload提前重传去重；同Seq不同payload、boundary overlap、queue overflow均abort候选并清buffer。

## 边界

这仍不是P2完成。真实SYN lineage、packet/persona、同association TLS、exporter、fallback、最终TLS应答/首record真实竞态尚未接线。

## 下一原子任务

定向读取packet.go、tcp_persona.go、server_mux.go及直接测试/import，只提取SYN/ACK/同四元组/sequence-space association最小闭包，并把现有BootstrapStream/Sender/StageTransition挂到该association。继续不迁移steady-state完整Receiver/SACK/RACK/FEC。
