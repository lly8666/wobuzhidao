# 20260919-201000 P2 uTLS Renegotiation/Exporter修复

## 目标

只修复pinned uTLS v1.6.5 Firefox120 persona与TLS exporter之间的确定性冲突，不改变TLS公开ClientHello扩展集合/顺序/字节，不扩展到认证、lease或steady-state数据面。

## 失败证据

SOURCE_SHA：edecd3d37d3c34eb1ff1155b86b1fa6f5d2c99cf

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35441650384

结果：
- repository-contract PASS
- Windows unit/build step FAIL
- Linux unit/build step FAIL
- Linux race/fuzz/reference因unit失败SKIPPED

Linux原始测试错误：

TestRealTLSExporterOverServerAssociationAndTransition:
crypto/tls: ExportKeyingMaterial is unavailable when renegotiation is enabled

Windows为同一测试/同一根因，因此不是runner偶发。

## 根因源码核对

pinned github.com/refraction-networking/utls v1.6.5 的 HelloFirefox_120 preset包含：

RenegotiationInfoExtension{Renegotiation: RenegotiateOnceAsClient}

uTLS BuildHandshakeState -> ApplyConfig -> RenegotiationInfoExtension.writeToUConn 会把底层config.Renegotiation设置为该字段。ConnectionState()在config.Renegotiation != RenegotiateNever时故意安装noEKMBecauseRenegotiation，因此即便协商结果是TLS 1.3，ExportKeyingMaterial仍返回上述错误。

该uTLS extension的源码同时明确：Renegotiation字段设为RenegotiateNever时，extension仍会发送。序列化内容只由RenegotiatedConnection决定，初始握手为空。因此可以只改变内部接受重协商策略，不改变Firefox120 wire persona。

## 修复

internal/realityfront/tls.go：
- 第一次BuildHandshakeState保持原Firefox120 preset。
- 写入WBD compatibility SessionID marker后，遍历uconn.Extensions。
- 对RenegotiationInfoExtension仅把Renegotiation policy改为RenegotiateNever。
- 第二次BuildHandshakeState重新ApplyConfig并marshal。
- 不删除、不移动renegotiation_info extension。

新增internal/realityfront/renegotiation_test.go：
- 要求Firefox120 renegotiation_info仍存在。
- 新客户端内部policy必须为RenegotiateNever。
- 原始Firefox120 reference policy仍为RenegotiateOnceAsClient。
- 两者extension Len/Read的wire bytes必须逐字节相同。

原TestRealTLSExporterOverServerAssociationAndTransition继续作为真正退出条件：真实uTLS client与crypto/tls server必须从原始ConnectionState导出相同tlsrecord KeyPair。

## 复用台账

更新old/internal/realityfront/single_flow.go -> internal/realityfront/tls.go条目，明确此行为差异和专项测试。没有新增archive source，也没有修改old。

## 下一步

提交后只认新精确SOURCE_SHA。必须同时通过repository-contract、Windows/Linux unit/build、Linux race以及既有fuzz/reference，才可将真实TLS/uTLS/exporter子任务标记完成。
