# E1 uniform32 FEC20:20：真实15ms整条FakeTCP/TPROXY/TUN低RTT稀疏业务通过（2026-10-09）

仅工作分支 next/performance-efficiency-20261008，父HEAD 63ab6e83b1e4087172fda1c5e5e191779bfabd50；产品已使用统一32ms源 `a2db258b436a41fdee98c6c53abec9bab6ce600f`，此回执只追加 evidence/devlog/STATUS，产品和维护/FEC热路径未修改。专用 [全栈Action37937452545](https://github.com/lly8666/wobuzhidao/actions/runs/37937452545) **完整SUCCESS**，配套 [Foundation37937452458](https://github.com/lly8666/wobuzhidao/actions/runs/37937452458) 也**完整SUCCESS**（仓库契约、Linux/Windows Go/race、TPROXY、iptables+nft TUN、P2 fallback及其它已有烟雾）。

从独立真实artifact11619545001下载ZIP，SHA256 ccece3c9281e5edcd1ccd08a3f0a64e93da32c63a43466a21335e5ee3403fc3e；正式client TPROXY、server真实TUN与目标UDP、5个Linux netns，source确认为a2db258...，FEC20:20窗口32ms，双向各native netem delay0.015秒，120秒无人工丢包，seed1846，每45ms轮发96/1372/4068B。

**真实独立业务而非单元测试**：C2S 2667发/2667收、S2C 2667发/2667收；三种尺寸每个方向各889包，无损坏、重复、跳过发送或stage非法丢包。C2S 96/1372/4068 p99分别15.505412/15.552994/15.750874ms；S2C 96/1372/4068 p99分别15.441736/15.550428/15.753568ms。Router rsrv/rcli qdisc分别真实转发19727/19726包、delay15ms、内核qdisc drop0；限定的4个underlay TCP pcap捕获计数约19705/19706包，kernel tcpdump drop0，原始pcap先生成hash receipt再清理。正式analyzer结果 `PASS_SCOPED_FULLSTACK_15MS_SPARSE_LOSSLESS`，摘要SHA256 20238d5beb81743850ad06b3bb826fc4ffa8c0cfc8fde4f34edcf4a10467d4b7。

**边界不许越界**：这一轮没有故意丢一片源，所以不能说全栈恢复FEC丢片PASS；旧 [真UDP socket+LINK/FEC 37933861380](https://github.com/lly8666/wobuzhidao/actions/runs/37933861380)的1片损失后repair和独立96B不HOL是另一有限范围的证据。此前Source32 Normal1/Game2各lossless/5205四条300秒业务已单独scoped PASS，但Game2真实同时存活lane数profileOFF未证明。本次低RTT normal1单场景不代表Game4。FEC20:20下8ms→32ms稀疏30包partial blocks从30降10，parity shard/字节不变，CPU gain UNPROVEN；不能凭成功体验测试推出成本节约。

下一步若要扩展低RTT弱网保真，只准定义一个可确认身份的源LINK shard在真实全栈中丢失、保证有parity并测一次修复/不阻塞后到96B的受控功能样本；如果做不到针对性丢片，不得凭随机netem 1% loss冒充精确修复。否则E1 lossless闭环即结束，转向已有证据的维护/FEC真正CPU热点，不再生新诊断平台。历史9V45 Game4 lossless OFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) packet socket client33/server86，ON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client42资源FAIL仍OPEN；E7约80秒下行断流继续OPEN_DEFERRED、物理NOT_RUN。
