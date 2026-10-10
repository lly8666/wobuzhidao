# ARM64 Poly1305 加速可行性：先测数据打包，生产加密代码不变

- 工作分支 `next/crypto-packet-efficiency-20261010`。严格产品基线 A `7fb98fab79834a351a1dbe04eebb207f66bea28b`。此前 one-block headerMask B 在真实 Q1 A/B/B/A CPU +2.02%，已回滚；不恢复。
- 实际使用 `golang.org/x/crypto v0.38.0`。其 `internal/poly1305/sum_asm.go` 的汇编构建目标仅 amd64、loong64、ppc64、ppc64le，没有 ARM64；ARM64 使用 `sum_generic.go` / `updateGeneric` 的 bits.Mul64/Add64 算法。
- 先前原生 ARM64 synthetic AEAD pprof `38049793709`：`chacha20.xorKeyStreamVX` 58.89% flat，`poly1305.updateGeneric` 19.57% flat，后者不是整个隧道真实业务占比，不可按 19.57% 推断产品收益。
- 本提交**仅增加测试** `internal/tlsrecord/poly1305_research_test.go`，比较 (1) 与 x/crypto generic AEAD 相同的 Poly1305 MAC 分段 feed (2) 单块 MAC + AAD/密文每包拷贝 (3) 已预组装 MAC 输入（不计组装成本，严格只是理论上界）；交叉以公开 `poly1305.Sum` 检查完整 MAC 输入；覆盖 0/1/15/16/17/短包/近 MTU/65535，AAD 0/1/13/16/17/32 及伪随机 key。
- native ARM64 workflow 增加目标 `poly1305-packaging-bench.txt` artifact，Go1.23.12，同来源同平台，0 产品修改。新源是否 PASS、CPU 改善和最终采纳判定 **本提交时均 NOT_RUN**。若拷贝成本抵消调用减少收益则拒绝此路径，**不通过修改密文、tag、nonce 或绕过认证来换速度**。
- 后续如考虑汇编加速，必须验证公开、成熟许可实现并做相同 crypto vectors、Go arm64/purego/noasm、失败 tag 清零、别名、并发与整机 profile-off A→B→B→A；仅 microbench 不可上线。原 Q2 3/4 FAIL、Game4/TCP/80 秒/MTU/PMTU 仍 OPEN；300s/三个独立 runner/P6/physical 均 NOT_RUN。
