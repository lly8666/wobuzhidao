# 内网安全成本初筛：AES-GCM 保留完整认证却快于不认证 ChaCha

- 研究代码 SOURCE `7ebb59bb29749ed9d368613c8ed35a1963fb069c`，Actions [38055895924](https://github.com/lly8666/wobuzhidao/actions/runs/38055895924) native ARM64/Linux amd64/Windows 全通过；[foundation38055895925](https://github.com/lly8666/wobuzhidao/actions/runs/38055895925) 与 lifecycle38055895927 均通过。
- ARM64 artifact `11671461373` digest `3b43fe76dfc88df19e5ea4117e96ec0a8fdbb8f537abd739246592be1ea584ea`，文件 research-security-floor-arm64.txt sha256 `8f44ae5b5b74505673dedc590d456972e2e4e5f473f475561f406b72743f2d5d`。原生 benchmark median:
  - ARM64 64B: ChaChaAEAD354.9ns / AES256GCM89.27ns / ChaCha-only without tag243.6ns / copy5.908ns；
  - ARM64 256B: 708.9/119.3/237.1/7.467；
  - ARM64 1400B: 2051/481.3/1325/30.66。
- x86 Actions linux job log 114224273145 same source:
  - 64B: ChaChaAEAD171.0ns / AES256GCM128.6ns / ChaCha-only(noauth)137.9ns / copy4.067ns；
  - 256B: 250.8/135.5/463.6/7.179；
  - 1400B: 728.3/523.0/2469/16.24。x86 standalone ChaCha-only经 generic cipher 路径可能比成熟完整 AEAD 更慢，不能推断撤 tag 一定提速。
- AES256GCM 与现有 AEAD 都提供 16字节 tag、12字节 nonce、AAD 关联认证、检测 tag 错误；只是算法不同，并非原 wire 可以不协商直接兼容。测试 Vec/篡改失败 PASS，copy/no-auth 只作理论速度上限，绝不可默认为安全产品。
- **新提交追加仅测试的整个 WBD record 编码/解码实验**，使用既有 `Sealer.seal`、`Opener.OpenRecord`、ChaCha headerMask、已分配 wire、outer type/version/length、PN、AAD、padding，两种 cipher.AEAD。验证相同 wire 长度、每种分别合法 decode、篡改拒绝、跨 suite 拒绝，native ARM64和Linux amd64读实际 record Seal/Open 性能。生产源码/参数/WIRE、headerMask、FEC全不变。新测试未执行，明确 NOT_RUN。
- 实际产品若迁移 AES256-GCM，必须新协商 record cipher ID/version，保持client/server对应、FEC 保护与 retransmission同wire、原核心 noHOL 语义及旧版本显式拒绝；在原生 ARM64完整业务按相同 runner ABBA 与 300s验证，不能继承这里只测 Go AEAD 的收益。
- 历史 headerMask B fullstack CPU+2.02% 已 REVERT；FEC 旧 Q2 原始3/4 FAIL，Game4/TCP/80s/MTU/PMTU OPEN，300s/3 runner/P6/physical NOT_RUN。
