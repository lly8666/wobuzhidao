# Crypto / packet 计算与资源优化执行方案（2026-10-10）

## 冻结与合作隔离
- 专用分支 `next/crypto-packet-efficiency-20261010`；创建点 `6861c2c94cc6250f51dcc1104b84a42e8526881b`，冻结产品A完整SHA `7fb98fab79834a351a1dbe04eebb207f66bea28b`。两提交之间internal/cmd/go.mod/go.sum无变化，仅实验辅助、工作流和证据变动；继承helper起始SHA `6861c2c94cc6250f51dcc1104b84a42e8526881b`，后续每次测量必须记录实际helper SHA。
- FEC SIMD工作线 `next/fec-simd-20261010` 不触碰；FEC主观收益认可，但Q2 300s真实run38042928701的3/4原始probe质量FAIL，不能视为正式通过。保持原FEC/MTU/repair/套接字缓冲/载荷。Go 1.23.12；physical NOT_RUN。
- 永久硬门：真实首到立即交付，NoHOL与p99优先；完整性/身份和地址隔离/固定wire密文/generation/资源有界。额外安全后置；不等ACK/FEC填块/lane；4096 shadow可放弃。

## 顺序
1. ChaCha20 headerMask：审x/crypto v0.38.0的x86通用路径、ARM64短调用缓冲。优先单块64B、20轮不变、取前8B；每方向初始化后只读8个key words，counter与nonce逐record取样；保留独立旧函数的随机golden及非法短输入、边界并发测试。ARM/x86 SIMD仅凭native执行证据标记；回退可用。
2. ARM64 Poly1305：先取得真实native短包/MTU profile，若没有热点则SKIPPED_NO_BOTTLENECK；不换AEAD接口或直接加小包C FFI，不碰认证。
3. TCP checksum：先审热点；正确的宽累加、进位折叠及奇偶偏移。缓存须绑定immutable payload+寿命+generation；CRC32不替代RFC1071；在没有足够受益前不加SIMD。
4. SHA256/copy/alloc/锁：不删整包冲突SHA256；所有权、关闭和并发必须同等，池有上限。没有热点不得盲改。
5. 检查正式buildinfo/x86 AEAD asm、purego/noasm/Windows/Linux/native ARM64路径。性能负面候选撤销。

## Actions 验收与窄例外
- 核心工作流 `.github/workflows/next-crypto-packet-core.yml` 精确branch，在Go/测试路径改动时触发 Linux amd64 unit/race/fuzz、Windows amd64、native ARM64/标量回退；源/工具链完整SHA必须保存。现有FEC+MTU+ownership+NoHOL等旧测试仍由foundation覆盖。
- 性能工作流 `.github/workflows/next-crypto-packet-abba.yml` 仅该分支 `.github/crypto-packet-abba.json` push；严格单测量job、无matrix/PR/dispatch/并行。同host固定A/B源码各编译一次，GOAMD64=v1/buildinfo/hash，逐leg隔离5 netns/状态/进程并删除owned资源，顺序A→B→B→A；old FEC实验的选择器、SOURCE和脚本不得修改。每次提交以唯一STATUS+新devlog+docs/evidence记录。
- 一个leg 120s业务+3s drain筛查，关键300s：Normal1 10Mbps方向混合20:20/300ms 0及5→20→5%，off/15ms 0和1%，Game2/Game4总3Mbps双向0/weak；稀疏/密集/MTU附近+合法大包、TCP长短流/HTTP(S)并存UDP。工具能力不足的组合保留NOT_RUN而不伪造。
- 每个度量同时保留两端CPU-s、CPU-s/verified GiB和submitted、RSS/HWM、实际注入、丢失/迟到各size、probe未回/p99、最长无交付桶、TCP内容hash、outer source/parity/repair/PPS、socket/raw drops。低CPU不能以少投/少交付替代；非全probe返回的p99是returned-only。
- 三独立runner且按CPU ISA/quota/PSI/steal分层；同runner配对优先。相邻原子parent比较、最后与冻结A比较。任何性能claim须保护质量，Q2原始FAIL不静默降低门。
- P6 manifest/哈希与physical接力在合格源冻结后执行；80秒下行E7与历史其它问题保持OPEN/NOT_RUN；不上传敏感内容/完整业务包。
