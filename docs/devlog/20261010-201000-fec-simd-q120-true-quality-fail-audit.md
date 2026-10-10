# FEC SIMD S4 Q120 正式 ABBA：Q1 配对通过，但 Q2 旧版真实质量失败导致批次中止

- 冻结旧 A `a2db258b436a41fdee98c6c53abec9bab6ce600f` 与新 B `7fb98fab79834a351a1dbe04eebb207f66bea28b`，本次 Actions helper `5a3406fc249cbbfba28cd537cbc43f0110a2713e`。产品源码不动。独立单job [run 38039151704](https://github.com/lly8666/wobuzhidao/actions/runs/38039151704)、[artifact 11665571189](https://github.com/lly8666/wobuzhidao/actions/runs/38039151704/artifacts/11665571189)，原始 ZIP SHA256 `46c80b718c2a389e9de21a267570d0b3346f6a5a2dae7936e317ee6dedc52170`。所有状态详见 `docs/evidence/fec-simd-q120-38039151704-audited.json`。

## Q1 四段完整 ABBA
- `s01=A,s02=B,s03=B,s04=A` 各120s业务+3s drain，Normal1、20:20、300ms单向、0% loss、双向10Mbps、同seed；**四段均 VALID_OBSERVATION**，每段有效交付恰为299213826字节、UDP缺失0、TCP hash与length mismatch 0、探针c2s 600/600、s2c 595/595，HTTP(S)20/20，socket/raw/link drops 0，严格owned无泄漏。
- 按原主指标CPU-s/有效交付GiB：A两段 `359.284973,363.806538`，均值 **361.545755**；B两段 `319.990422,324.117247`，均值 **322.053834**，**单host配对B降低10.923%**。产品CPU-s均值 A100.75、B89.745。两个版本4段真实业务注入/交付同量；未靠少交付获得表面改善。
- 探针返回者p99 C2S/S2C：A段603.432/603.670、604.541/604.689ms；B段602.869/603.476、604.103/603.670ms；全部探针皆返回，无失踪幸存者偏差，未见>=10ms或>=5%的恶化。最大active业务空桶各段不超过20ms；仅是连续性诊断，**不能证明所有场景无HOL**。
- HWM MiB A client40.79/server41.66, client40.78/server41.49；B client41.52/server43.99, client43.39/server44.04。B RSS/HWM相对增加但远低于64MiB/进程审计线；PSI10峰值 A9.0/9.05%，B34.96/8.38%，quota UNKNOWN；不得脱离本机分层泛化10.923%。真实完整窗口IP带宽捕获不存在，不能将qdisc PPS直接当精确线速节省。

## Q2 旧版 A 真实质量失败（不是workflow或静态错误）
- `s05=Q2 A` 正常脚本退出0、analyzer退出1且给出 `FAIL`，ledger0、clean true。完整20:20、Normal1/300ms、5→20→5% netem，压力阶段实际 c2s19.9785%/s2c20.0210%，相位时间/路由有效。
- **压力60s，probe发300/方向，c2s 296回、4永不回=1.3333%，s2c 295回、5永不回=1.6667%**；触发 `5205_STAGE_PROBE_LOSS_OVER_1PCT_{c2s,s2c}_stress`。阶段用发送端monotonic时间归类。不能因为全程c2s 4/600与s2c5/595低于1%而忽略压力期阈值。
- 同压力期UDP c2s和s2c各发69104、各缺1（全程也是各缺1）；TCP/HTTP(S)完整、零local/socket/raw drops、PSI10峰10.67%；**真实残余损失不是测量无效，不改成PASS或INFRA_INVALID**。是否为理论FEC方程不足、孤立parity突发、迟到过期或其他路径需进一步有针对性的证明，不推断B的弱网表现。
- 框架按FAIL立即停止，`s06/s07/s08 Q2` 与 `s09-s12 Q3` **共7段NOT_RUN**。整个 Q120 为 `FAIL_OR_NOT_RUN`，不是全部12段ABBA PASS，不允许启动正式L120或300s/跨CPU/ARM性能/P6发布。
- Foundation此前 `TestLifecycleEntryGameThreeAndFourLaneMatrix/lanes-3` 首次timeout FAIL及专门重试SUCCESS均保留；其它旧Game4、300ms TCP-off、MTU/PMTU失败和约80s下行 OPEN_DEFERRED不变，physical `NOT_RUN`。

## 下一步与不可做
先找实际Q2旧A stress probe/UDP残余损失的有界根因或明确该旧版基线在既定阈值下不可满足质量门；产品A不可被偷偷打补丁、质量阈值不可降低、不能让新的B单跑被称为已验证的相对改进。为避免重复盲测，这次**不触发任何其他测量配置**，保留源、job、原始artifact作为继续开发的唯一锚点。
