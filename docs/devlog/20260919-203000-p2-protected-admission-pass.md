# 20260919-203000 P2 Protected Admission Actions回执

SOURCE_SHA：d4acf7ae1389920c67a192a4359ade4120675387

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35442811807

顶层结果：completed / success。

通过：
- repository-contract
- Windows go list / go test ./... / go build ./...
- Ubuntu go list / go test ./... / go build ./...
- Ubuntu race
- 既有tlsrecord directed fuzz
- independent P1 reference vector generator
- artifact upload

本SHA固定并验证：
- TLS内单次username/password认证；
- record_version只接受1，未知版本明确拒绝且不降级；
- TunnelID按既有规范固定16-byte raw；
- client/server record wire limit受tlsrecord实际构造器边界约束；
- server生成16-byte incarnation nonce；
- client校验version/client_limit/TunnelID回显；
- 双方从各自原始TLS/uTLS ConnectionState，用协商参数派生同一KeyPair；
- server在最终TLS admission reply前PrepareTransition；
- 最终reply的FakeTCP ACK完成后DetachTransition；
- wrong password不会prepare；
- Linux race覆盖该阶段编排。

P2仍未完成：ACCEPTANCE明确要求真实fallback。下一原子任务只补unrecognized ClientHello -> decoy target的raw replay/bidirectional splice，不进入P3。
