# 20260923-211500 Game4共享短黑洞单样本入口

新增独立 `next-shared-blackhole`，不改产品/wire/FEC。规范已有Game4、3Mbps/方向、FEC20:20、300ms单向、四lane共同100ms/500ms短黑洞；黑洞内允许业务loss，恢复必须有界。

规范未固定事件时刻/背景loss，因此本harness显式新增并写入manifest：背景loss=0，黑洞约t=60s。100ms与500ms分别独立Action run。

恢复门只组合已有门槛：blackout解除后加300ms路径传播，从预计恢复到达时刻起3s内必须出现一个完整1s wall unique goodput >= 3Mbps×99%的窗口。CORRECTNESS/INPUT_VALIDITY/CAPTURE/ENVIRONMENT仍必须PASS，socket/link local drop=0，drain不得持续积压。

新增：
- `.github/workflows/next-shared-blackhole.yml`
- `tools/strict_blackhole_stage.py`
- `tools/check_strict_blackhole.py`
- `tools/test_blackhole_analysis.py`
- `.github/workflows/next-blackhole-analysis-unit.yml`

复用正式 `strict_weaknet_sample.sh` 路径，只新增blackhole scenario分支；performance static policy纳入新workflow；dispatch relay白名单扩展到新workflow并继续按head SHA去重。

首请求只武装100ms/seed601；unit/contract通过后才dispatch。
