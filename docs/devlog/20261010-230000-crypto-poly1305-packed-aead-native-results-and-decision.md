# Poly1305 完整 AEAD ARM64 原生测试：小包微益、大包微负，拒绝产品化打包重写

## 可靠结果
- 测试代码 source `ab4510e892db655201cbfc6755c57656ae11ce4f`；正式 `internal/tlsrecord/record.go` blob `1181eee429a2a6025e5544078d3905dee1a54a48`，与冻结产品A相同，无生产行为变化。
- [Actions native core 38052541722](https://github.com/lly8666/wobuzhidao/actions/runs/38052541722) **SUCCESS**（原生 ARM64 / Linux x86 / Windows）；[foundation 38052541804](https://github.com/lly8666/wobuzhidao/actions/runs/38052541804) SUCCESS；[lifecycle 38052541801](https://github.com/lly8666/wobuzhidao/actions/runs/38052541801) SUCCESS。
- 原生 ARM64 artifact `11670116461`，digest `6e6756b3dc12e12941e761e1036765afa69c3238ee705acc18c75bdf27492be4`。完整逐行报告、文件 SHA256 与执行 scope 见 `docs/evidence/crypto-packet-poly1305-full-aead-native-38052541722.json`。
- full AEAD Seal（nonce 每次变动、固定13字节AAD、同 key 和消息、0 alloc）三个样本**中位**，reference→packed ns/op：
  - 64B: `353.7→337.2`，快 **4.665%**；
  - 128B: `406.1→389.0`，快 **4.211%**；
  - 256B: `707.0→695.8`，快 **1.584%**；
  - 512B: `1022→1019`，快 **0.294%**；
  - 1200B: `1734→1743`，**慢0.519%**；
  - 1400B: `2041→2045`，**慢0.196%**。
- `TestPoly1305ResearchPackedAEADReference` 与成熟 x/crypto 整段 wire 输出逐字节一致，ref.Open 能解密且修改 tag 拒绝、对 AAD 长度 0/1/13/16/17/32及边界包/重复 scratch 正确。测试版仅实现 Seal，**不支持完整 cipher.AEAD 的别名/并发/失败状态契约**，不具备部署资格。
- MAC-only 的 64B 包装测试快31.74%是**局部 MAC 操作**，完整 AEAD 只快4.665%，正常大包更慢；再次证实只看微热点容易夸大实际收益。

## 决策与下一步
- **REJECT** 把 contiguous packed Poly1305 重写作为现阶段产品优化。要替换成熟 AEAD 需要新缓冲/布局管理和专门 Open 路径、常数时间审查，且 ARM64 Seal 对1400字节已稍慢；这点有限的小包 micro gain 不足以承担安全与稳定性成本。
- 不排除经许可和质量验证的**真正 ARM64 汇编 Poly1305 后端**可能显著降低计算占比；但必须先拿真正 native ARM64 完整隧道 profile（诊断版单独于普通 profile-off）证明 CPU 热点够大，不能以 synthetic pprof 19.57% flat 冒充产品占比。然后严格同 wire RFC AEAD、tamper/nonce/alias/race/purego/noasm/Windows/Linux/noHOL、正式 A→B→B→A及300s等门。
- 本轮没有任何经过实证的**正式产品性能加速**。Q2 300s 3/4原始 FAIL、Game4/TCP尾包/MTU/PMTU/80秒下行保持 OPEN，native ARM64 fullstack/300s/三独立runner/P6/physical **NOT_RUN**。旧 headerMask fullstack B CPU +2.02% 已撤回，不恢复。
