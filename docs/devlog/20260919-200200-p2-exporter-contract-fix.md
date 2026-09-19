# 20260919-200200 P2 Exporter契约文档修复

## 目标

只修复f176d687cb33036b7bba5a7e31e46fc800b77d51缺失的STATUS/devlog配套证据，不修改任何产品Go代码或依赖。目的是让相同realityfront/uTLS/exporter产品代码进入完整Actions验证。

## 上一失败的精确证据

SOURCE_SHA：f176d687cb33036b7bba5a7e31e46fc800b77d51

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35440654089

结果：FAIL。

repository-contract原始错误：
- Each change needs STATUS update
- Each change needs a new development log

该run在repository-contract阶段终止，active-go-tests被SKIPPED。因此不能把f176d687视为产品测试通过，也不能把失败归因runner。

## 产品代码状态

f176d687的产品修改保留原始uTLS对象与真实exporter能力，同时固定Firefox120 persona：
- client直接保留*utls.UConn，并从其真实ConnectionState调用ExportKeyingMaterial。
- server直接保留*tls.Conn，并从其真实ConnectionState调用ExportKeyingMaterial。
- 不手工复制ConnectionState。
- uTLS client显式RenegotiateNever，server crypto/tls也显式RenegotiateNever。
- exporter context继续按WIRE_SPEC绑定version/incarnation nonce/TunnelID/client_limit/server_limit。
- 当前go.mod已经把x/text修正为v0.25.0以满足x/crypto v0.38.0的MVS要求。

本提交不对这些代码做任何改动，只补仓库强制的状态与开发日志。

## 失败链

- 88c3efa6df92f6af24aef9fbede36befdc477fc9 / run 35439009012：repository-contract PASS，但Windows/Linux go list要求tidy；根因x/text版本过低。
- f176d687cb33036b7bba5a7e31e46fc800b77d51 / run 35440654089：module修复后的产品代码没有进入Go测试，因为该代码修复提交漏写STATUS/devlog，被repository-contract正确拒绝。

两次失败原因不同，均有新证据，因此不是重复盲改。

## 下一步

本纯文档修复提交触发完整Actions。退出条件：
- repository-contract PASS
- Windows go list/go test/go build PASS
- Linux go list/go test/go build PASS
- Linux race PASS
- 既有tlsrecord fuzz/reference不回归

只有这些全部通过，才把realityfront真实TLS/uTLS/exporter子任务标记完成并进入受保护admission参数/最小认证。
