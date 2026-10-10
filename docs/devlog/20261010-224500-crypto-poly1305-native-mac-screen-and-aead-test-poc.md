# Poly1305 原生 ARM64 MAC 筛查有微收益；再建完整 AEAD 实验

- 测试工作流先于 run `38052140743` 发生 YAML_PARSE_FAIL_NO_JOBS，已在 `965778f9` 修复并保留。
- [Actions run 38052216364](https://github.com/lly8666/wobuzhidao/actions/runs/38052216364) 于代码测试 source `965778f9c03bf96fab18b322e9d458d9a9dab8e2` 原生 ARM64 + Linux x86 + Windows 测试全部成功。ARM64 ZIP artifact `11670300227`，digest `65fa9680bdb634c27f45f0793bd78aa1fb5725b60ee1c766d92ac2488f7624c8`，原始数字见 `docs/evidence/crypto-packet-poly1305-packaging-native-38052216364.json`。
- MAC-only `stream / contiguous_copy / prebuilt` 中位 ns/op：64B `77.97/53.22/47.07`，128B `103.0/79.87/71.48`，256B `153.1/132.1/120.5`，512B `253.2/239.6/220.5`，1200B `525.9/515.3/485.7`，1400B `617.8/609.9/565.3`。contiguous_copy 包含 AAD/密文复制但复用预分配 scratch；0 B/op,0 alloc/op。包装方案 64B 对 MAC 部分快 31.74%，1400B 仅快 1.28%，**不是整个 AEAD 或隧道收益**。
- 本提交添加**仅在测试目录**的完整 Seal PoC，将标准 ChaCha20 + Poly1305 MAC 连续打包。与成熟 x/crypto 输出逐字节相等、ref.Open/篡改失败、各种 AAD/长度/密钥与有界复用 scratch 均要求严格通过。PoC **不是完整 cipher.AEAD 实现**，不满足所有别名/错误接口，不可部署；只量是否完整加密收益值得继续。
- 生产源码 `internal/tlsrecord/record.go` 完全不改；本提交时新 PoC Actions 尚未执行，故全 AEAD 提速、native ARM64 fullstack、three-runner和300s/physical维持 NOT_RUN。即使全 AEAD 微增益亦须正式 120s profile-off ABBA 和额外验收才可宣布可采用。
