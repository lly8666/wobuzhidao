# 2026-10-12 N2 M1 首轮 Actions FAIL：partial k=1/k=2 反例更正

严禁把工作流绿当产品好。SOURCE `6e0d6aea1214033b164fda495d4cdff977cfb0b3`，Actions `next-adaptive-n2-marginal-core` [38202927503](https://github.com/lly8666/wobuzhidao/actions/runs/38202927503) job `114660056352`，**FAIL in focused Go unit; race NOT_RUN**。从GitHub原始job日志确认，20项测试里两项FAIL：

- 新 `TestN2UnattainableDoesNotBlindlyRaiseTo20` 把 k1, p5%, floor0 的理想最低档错当0。实际 `q(k1,off,p)=p=.05`、`q(k1,R>=4,p)=p^2=.0025`，仍不可达到目标0.001，但第一个开启parity确有巨大收益。合理结果4而非0；k1 floor4以后都不变。
- 旧 `TestBoundedNormalAndAggressiveEventuallyMoveAndRaise` 假定高真实首发损伤必然从k2组20:4加档。实际发送`r=min(k,R)=2`，不论配置4/8/20都只能发两块parity；提升名义档位对k2没有额外保护。旧期望一见高损就raise20与用户“理论不可达不能无意义升满”冲突。原测试保留高loss gate意义：现在先断言k2在高损不可盲升，再输入有真正20-shard的可保护闭合组高损，必须观察合理RAISE。

本次只修上述两个**逻辑错误测试预期**及补充k20反例，没有改动已经提交的生产软参数、原N2硬安全门，未降低模型目标0.001或改变冻结C03/C04/C08/C09；保持首次FAIL收据永久存在。新 Actions 再运行核心unit/race；若仍失败继续定位，不能先跑netem或用单seed证明优化。原540 C2S恢复效率不合格、2.73s/80s历史问题仍OPEN，physical NOT_RUN、N3-N6 NOT_STARTED。
