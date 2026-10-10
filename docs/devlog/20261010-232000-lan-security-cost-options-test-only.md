# 内网极简安全模型：先评估费用，不修改正式协议

用户允许考虑完全受控 LAN，甚至省掉数据包防篡改。本轮探索但未授权直接把有实际账户/地址隔离/多lane/FEC的现有正式产品改为不可鉴权，故仅加入研究性单测基准。

## 准确现状
- WBD 先通过真实 TLS 1.3/uTLS 和 protected admission 获取密钥（新lane/新incarnation一次），稳态是自定义 TLS-like record，ChaCha20-Poly1305 逐包 Seal/Open，12字节nonce、13字节 AAD、16字节tag、8字节ChaCha header-protected PN。没有开放给标准 TLS 解密端的标准 TLS 1.3 稳态记录；仅外观样式。
- FEC/MTU/ownership和 TCP IPv4+TCP checksum 不是纯保密成本。去掉 MAC 后被修改的密文会静默变成被修改的业务数据，FEC可能以错误片重构，account/IP invariant 风险大；SHA256/HKDF和 ClientHello marker MAC 基本是每lane建连成本，不是steady per-packet。
- 过去 native ARM64 synthetic AEAD profile 中 Poly1305 updateGeneric 占 19.57% flat，是 synthetic 占比。包装重写完整AEAD 64B仅快4.665%，1400B慢0.196%，已标为拒绝上线（详见 38052541722）。
- AES-256-GCM 同样 16字节tag，可能走平台 AES/PMULL/PCLMUL 组合，**须 Actions native 数据验证，未声明硬件能力或真实产品收益**。
- 仅测试中的四个模式：ChaCha20-Poly1305（当前）、AES-256-GCM（保持认证）、ChaCha20 encryption-only no MAC（故意不安全的上限对照，不上线）、plaintext copy-only（极限下界，彻底不符合伪TLS密文要求）。保持消息/AAD/nonce、固定预分配输出和动态 nonce，Bench 每尺寸 count=3；arm64结果写 artifact，amd64写 Linux job log。
- 测试 `TestResearchCryptoCostVectors` 必须核两种 AEAD 的 Open/错误tag拒绝、ChaCha-only roundtrip并证明篡改一位会进入明文。报告只比较单核 synthetic Seal，不计 header mask、完整 record 收发、FEC、前后端、锁/syscalls和实际 packets；不得推为实际 CPU 降幅。
- 正式 record.go/hash、go.mod、FEC/WIRE、参数、GUI均不改，不新增用户可选无鉴权开关。若真采用另协议，需要双方显式协商新 record 版本、准入可信环境约束、tag/小包 header sample 结构、FEC 认证前置、无HOL/固定wire重传边界、严格所有权测试及真实 ABBA/弱网/长测确认。仍需用户明确选择实际威胁模型。
- 保留 Q2 原始3/4 FAIL、Game4/TCP/80秒/MTU/PMTU open；300s/three-runner/P6/physical NOT_RUN。全实验 Actions 未完成前标记 NOT_RUN。
