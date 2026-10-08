# E0 Game4混合无损正式业务样本FAIL；从首条失败缩小诊断（2026-10-08）

## 正式sample与不可修改的原判定
目标分支 `next/performance-efficiency-20261008`、提交父HEAD `aa2200c833dd557d2c926e5ea4fd5316897b610a`，产品固定SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733`，助手源 `aa2200c833dd557d2c926e5ea4fd5316897b610a`。独立[Actions run 37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445) job113276399837/artifact11545736422，单300s真实client/server+TPROXY加密raw+real Linux TUN，attempt1，Game4/每向逻辑3Mbps/mixed TCP+UDP/HTTP(S)、FEC20:20、padding-off、lossless300ms one-way、outer1400/自动record0/serverTUN实际1273、seed1804、3s drain、diagnostic off。**原样本步骤success、ledger success、analyzer FAIL、Actions failure**，原issues三个：`LOSSLESS_UDP_MISSING_c2s`、`LOSSLESS_PROBE_MISSING_c2s`、`LOSSLESS_PROBE_MISSING_s2c`。不能让另一个healthy mode、TCP/HTTPS PASS覆盖Game失败。

## 逐方向业务/探针/延迟
- C2S总逻辑发出(含独立HTTP)2.976187Mbps、bulk goodput2.941329Mbps，普通UDP按96/256/512/1000/1372/4068B累计**103062份发出，2154份未到**(1257/319/156/90/208/124)，不属于外层预设netem loss；TCP304/304条hash与长度一致、无损坏证据。小UDP探针1500发1474回，**26 missing**，仅返回项p99=815.494ms、最大2524.923ms，最长主动recv gap=320ms（代理诊断，不是已经证实跨业务HOL）。缺失不可填0，不可仅给幸存者p99当全部业务p99。
- S2C总逻辑2.977328Mbps、bulk goodput2.972316Mbps，UDP 103062份0 missing、TCP304/304相符，但小probe1495发1462回，**33 missing**，仅返回项p99=792.261ms、最大2056.827ms，主动gap30ms。C2S/S2C探针相对于旧Normal mixed约604ms有真实尾部恶化，环境CPU型号不同所以不直接作为优化对比。 HTTP/HTTPS20/20成功、10个HTTPS证书+内容验证，返回p99=2694.734ms/未回0。
- Linux hosted AMD EPYC 7763、4vCPU；client/server产品CPU186.15/164.52s，总350.67s/300s，不可直接与Normal混合的AMD EPYC9V74对比。host busy峰79.83%、softirq峰12.03%、steal0、CPU PSI avg10 some峰35.66；实际可用cgroup quota字段未解出。socket与interface drops均0，client/server packet recv buffer峰占用约14.45%/7.15%。既不能简单说无drop=宿主有余量，也**没有充分证据按CAPACITY_LIMITED自动改判**。Game4 4lane计数仅CLI/manifest同源锁定，真实竞速和每lane计时细节需独立诊断回执。

## 下一项，不做盲测与不抢跑产品优化
首先冻结本次原始FAIL、小summary/ledger/manifest SHA256在 [机器证据](../evidence/performance-efficiency-e0-game4-fail-37766819445.json)。另开单条明确诊断开关的Game4/profile-on Actions，采实际lane generation/PacketID、FEC partial/source/parity、ACK/repair、raw send+receive batch及队列延迟；profile-on引入成本不与profile-off直接比较CPU。也可定点离线解析原pacing10ms桶，区别源漏发、平台交付、late/丢失/本地drop；不能新造真实本机物理测量。Game4本次未通过，不能按计划E1守护门PASS，也不放宽lossless门、扩4096/FEC缓冲或删独立业务以求绿。旧TCP-only run37762364098仍FAIL，旧Mixed超预算run37763345793 raw PASS但NOT_QUALIFIED，Normal mixed严格修后run37764510941仅scoped PASS。

最终E6/P6/同源manifest/hash未开始、物理NOT_RUN；原80秒下行FAIL保持OPEN_DEFERRED到E7。以上只是E0证据，CPU优化收益仍**NONE**。
