# E1 16ms大组源码首测失败：大UDP短尾片仍落进8ms小组，已定位并纠正（2026-10-09）

仅工作分支 next/performance-efficiency-20261008，父HEAD 6bde35e43c31d12665cf49e3e580ee2b387f77a6。首个只加大组16ms候选的 [Foundation37918866355](https://github.com/lly8666/wobuzhidao/actions/runs/37918866355) 整体 **FAIL**，Linux `TestE1Largest...`在8ms得到2个校验片不是预期1个；历史 `TestFECSizeClassesFragmentRecoveryOwnershipAndNoHOL`和1372B有界20%擦除试验也失败。对应 [Lifecycle37918866377](https://github.com/lly8666/wobuzhidao/actions/runs/37918866377) SUCCESS，但不能覆盖Foundation失败。本提交保留原失败及因果，不把它称为CI小毛病。

真正原因是LINK分片后FEC选组按**碎片自己的字节数**判断：1372B UDP的尾片可能只有约200B，所以它单独进入<=256B的8ms parity组；只把第一片所在>512B组调为16ms，**整个UDP仍有尾片在8ms掉进独立的一片一校验脆弱组**，因此原先测试中1372B最初仍不能恢复。4068B同样可能有短尾；为改善可恢复性必须让同一大UDP全部分片在同一个大组内聚合，而不是仅改最长定时。

纠正改动：保留`SizeClassEncoder.Add`向后兼容的按碎片尺寸选组；新`AddFragmentOfLargeDatagram`由`FECPath.Encode`仅在20:20+默认8ms+原始数据报需要多个LINK分片时调用，强制其**所有分片，包括短尾**进入>512B同一组，该组仍最多16ms才发partial校验。无关的96/256B完整游戏数据报仍在小组8ms走，systematic立即发送，满20组即时parity，existing owner timer/ticker、SACK/ACK/repair、线格式和队列不变。

真实代价也不能隐瞒：以前尾片在小组产生较短parity，现在跟1200B第一分片一起编码，同块校验片可能都接近1200B，增加实际FEC wire bytes与CPU；非丢失帧的首次交付依旧即时，但丢失后的FEC校验可能最多晚8ms。新测试检查大UDP第一片与短尾BlockID一致、16ms，独立小包8ms；历史FEC大小组无HOL/owned-wire测试更新为8ms小校验及16ms大校验两期；保留真实构造下全包恢复而非单个shard恢复。

已有 E2 源 fc92076b 的 20%真实弱网 [Action37915859759](https://github.com/lly8666/wobuzhidao/actions/runs/37915859759) **FAIL** 原样保留（1372+4068占严重stress丢UDP大头，skmem.d两端0）。本次只修刚暴露的源码问题，不运行新的300s。下一步先完整Go Linux/Windows/race/Lifecycle，然后严格一次一条 Normal1无损、再弱网，遇失败不挑宿主/不继续Game、不调到24/100ms冲绿，CPU获益未证实。
