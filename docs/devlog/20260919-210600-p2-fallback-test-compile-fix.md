# 20260919-210600 P2 Fallback测试编译修复

SOURCE_SHA：e98ee2cd40d22f133b57af39e875574975e878d4

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35443074386

## 精确失败

repository-contract：PASS。

Windows/Linux均在go test ./...编译internal/realityfront测试时失败：

internal/realityfront/fallback_test.go:273:6: assoc.Close() (no value) used as value

因此：
- fallback产品实现尚未进入运行期测试；
- Linux race/fuzz/reference均未运行；
- 不能把该SHA视为fallback行为失败，也不能宣称通过。

## 修复

只修改fallback_test.go：
- 将 "_ = assoc.Close()" 改为 "assoc.Close()"。

ServerAssociation.Close本来就无返回值。产品Go代码、fallback分支逻辑、raw ClientHello replay、decoy dial/splice和recognized admission路径全部不改。

## 下一步

新精确SOURCE_SHA重新跑完整Actions。只有repository-contract、Windows/Linux unit/build、Linux race及既有fuzz/reference全部通过，才把real decoy fallback标记完成。
