# FEC SIMD S1 — 单片编码/恢复共用后端提交（待 Actions 验证）

- 工作线：`next/fec-simd-20261010`；本提交父 HEAD：`f666adcd738445f1eb8aaf11092f0a107fdf30ef`；helper 起点：`543ac2cd2920e9f0fb59fdb38ee6aa3d9a65e56f`。
- 精确旧版 A SOURCE：`a2db258b436a41fdee98c6c53abec9bab6ce600f`。本提交产生的产品 SOURCE 以 GitHub commit SHA 为准，不能提前在自身文件里填写自指 SHA。
- 代码审计：`internal/fec/fastcodec.go` 的 `xorMul` 同时服务 EncodeActive 和 Reconstruct；现有 `fastblock_encoder.go` 的 concrete fast codec 仍跳过 inactive 清零。此阶段不改 wire、partial、32ms/3s、systematic ownership、解码时间线。
- 依赖：`github.com/klauspost/reedsolomon v1.12.6` MIT（tag 4916c9cd17081aa4f43f36639502e86f5787b40e）；`github.com/klauspost/cpuid/v2 v2.3.0`，原 `x/sys v0.33.0` 不降级；go 1.23.12 不变。go.sum SHA 使用 sumdb 已公开的校验值。
- 热路径：平台初始化能力按 amd64 AVX2/SSSE3 与 ARM64 ASIMD；单片长度 ≥32 才使用零值 `LowLevel.GalMulSliceXor`，否则 WBD 原 64KiB 乘法表标量。非 SIMD 平台和 `wbd_fec_scalar/noasm/appengine/gccgo/nopshufb` 强制标量，避免库 scalar 大型 16-bit 惰性表。库 v1.12.6 的 LowLevel.WithOptions 丢弃局部 options，故没有调用。没有声称 AVX512/GFNI 或 ARM native 性能。
- 正确性新增：GF 256×256 全对照、0/1/255 系数、实际短片及尾部、非对齐与输入输出 sentinel、网络片长微基准；先执行原 active/恢复/ownership/core/race 验证。
- Actions：**本日志提交时 NOT_RUN**；不存在此 SOURCE 的编译、unit/race、SIMD native、微基准、真实业务、CPU 优势证据。S1 验收须记录精确 workflow run/job/artifact，若失败保留 FAIL，不把代码提交视为 PASS。
- 后续：先查看 foundation Actions；补标量/noasm/native ARM64 证据，S2 仅在收益可测的前提下启用整块融合；再正式 paired ABBA。
- 保留历史：11 项 RTT、Game4 socket/ready overflow、300ms TCP-off 收尾、MTU/PMTU、约 80s S2C 中断保持原 FAIL/OPEN；physical=NOT_RUN，新候选不继承 A 的 PASS。
