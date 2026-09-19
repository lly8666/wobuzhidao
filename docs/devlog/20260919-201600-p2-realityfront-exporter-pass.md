# 20260919-201600 P2 Realityfront Exporter Actions回执

## 精确证据

SOURCE_SHA：f26b88427c20dd960fbc70661fd4ea951b8a29ae

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35441797398

顶层结果：completed / success。

实际通过：
- repository-contract：PASS
- Windows 2022 go list / go test ./... / go build ./...：PASS
- Ubuntu 24.04 go list / go test ./... / go build ./...：PASS
- Ubuntu race：PASS
- 既有internal/tlsrecord directed fuzz：PASS
- independent P1 reference vector generator：PASS
- reference artifact upload：PASS

## 本轮闭环的关键行为

- uTLS Firefox120 ClientHello仍保留原renegotiation_info扩展类型、位置和序列化字节。
- 仅内部renegotiation接受策略从OnceAsClient改为Never，因此ConnectionState exporter可用。
- uTLS client和crypto/tls server在同一FakeTCP association的BootstrapConn上完成真实TLS 1.3。
- 双方直接从各自原始ConnectionState调用ExportKeyingMaterial，未手工复制ConnectionState。
- exporter context绑定version/incarnation nonce/TunnelID/client_limit/server_limit；相同参数得到相同tlsrecord KeyPair，改变TunnelID后keys变化。
- server握手后prepare；提前到达的首个TLS-like record进入StageTransition queue并在detach后原Seq/原payload移交，没有被TLS reader预读吞掉。
- wrong route key的ClientHello保留raw/SNI且Recognized=false，为后续真实fallback保留字节。

## 失败链说明

此前三个失败均已保留证据且根因不同：
- 88c3efa6：go.mod x/text MVS元数据错误。
- f176d687：代码提交漏STATUS/devlog，被repository-contract挡住。
- edecd3d3：真实unit首次暴露Firefox120 preset把Renegotiation设为OnceAsClient，导致exporter禁用。
- f26b8842：保留wire extension但内部policy改为Never后全套PASS。

## 下一原子任务

定向提取simple_auth相关最小认证和受保护admission参数：
- record_version固定1，未知版本拒绝
- server生成16字节incarnation nonce
- TunnelID
- client/server record limits
- username/password最小认证

这些字段必须在TLS内交换，并直接成为exporter context/prepare边界的输入。ticket/lease/CLI与steady-state FEC继续不进入本任务。
