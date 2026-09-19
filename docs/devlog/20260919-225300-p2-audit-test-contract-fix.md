# 20260919-225300 P2 Audit测试契约修复

SOURCE_SHA：589994182bc6216f6468475a085ec8d9ed165bfc

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35449863782

## 精确失败

repository-contract PASS。

Windows/Linux unit均在新审计运行期测试真正执行前失败：

1. internal/faketcp/association_test.go
   TestServerAssociationRejectsNonWBDAndCrossFlowHandshake 仍断言 MSS=1460 的非WBD固定指纹SYN必须返回 ErrBadServerSYN。
   这与本轮审计要求“普通合法SYN必须先进入连接，身份到ClientHello再判断”直接冲突。
   新产品实现接受该SYN是预期行为，因此修测试契约，不回退产品代码。

2. internal/realityfront/audit_closure_test.go
   成功handoff测试保留了未使用 client 局部变量，Go编译失败。

race/fuzz/reference因unit失败未运行。

## 修复

- 将旧测试改名为 TestServerAssociationRejectsCrossFlowHandshake。
- 明确证明MSS=1460的合法普通SYN不是WBD presentation，但NewServerAssociation应接受。
- 继续保留cross-flow ACK必须ErrHandshakeState的原断言。
- 成功handoff审计测试不再保留未使用session变量。

产品SYN准入、peer MSS协商和candidate deadline实现不变。

## 下一步

新SOURCE_SHA重新跑完整Actions。下一轮若失败，才视为进入普通SYN fallback或绝对deadline的真实运行期问题。
