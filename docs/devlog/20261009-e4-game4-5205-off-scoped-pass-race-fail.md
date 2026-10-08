# E4 raw recv16 Game4 真5205单样本证据回填；foundation race失败继续开放（2026-10-09）

工作范围仅 `next/performance-efficiency-20261008`。核验远端父HEAD `a1ce9ab92eed39a04f651931111ceb2a3e5e0106`，不动规范主线 `next/tlslike-dataplane`，无物理机器/新性能产品候选。本提交仅把已完成运行的原始结果独立核实并回填 STATUS/evidence，不能重复dispatch旧seed假装新测量。

## 原始身份、业务与分析器

- [Game4 Actions 37853468730](https://github.com/lly8666/wobuzhidao/actions/runs/37853468730)，job113571933555，artifact11582763655，run success；原始 analyzer **PASS_SCOPED_ACTIONS, issues=[]**，不是全产品PASS。真实SOURCE `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，helper `32db36d7f3a5ef8177372e76effc775cf070dbb0`，seed1840，独立一run一case。
- 真实 socket→Linux OpenWrt TPROXY 正式客户端→raw FakeTCP/TLS-like→router两向真实netem→正式server shared TUN→真实socket。300秒业务+3秒drain；Game4配置逻辑4 lane（OFF未采集实际活动lane数），每方向逻辑3Mbps，300ms单向，FEC20:20，padding/profile OFF，outer MTU1400，auto record cap0，**实测 server TUN MTU1273**。
- 固定前75秒5%/中150秒20%/后75秒5%，按**原发送时钟**归因：C2S qdisc实际5.0340/20.0530/4.9803%，S2C 5.0331/20.0976/4.9928%，而非平均12%代替各阶段。
- 双向每向 UDP pre **25792/25792**、stress **51503/51503**、post **25767/25767**，尺寸96/256/512/1000/1372/4068B均完整，无malformed/corrupt；各阶段UDP 1秒/3秒超期都0。独立probe C2S **1500/1500**、S2C **1495/1495**，未回/1秒或3秒超期均0；返回项p99 **621.309656/623.435425ms**，两向样本全返回因而未隐藏timeout。持续10ms窗口统计最大活动空桶60ms，只是诊断非HOL根因。
- 双向 TCP各304独立流字节/hash一致，HTTP(S)20/20，HTTPS验证10/10。两向goodput各2.97231648Mbps，受控业务注入约2.976Mbps/向；额外 socket/interface drop **0**，server/client packet socket容量读回各1048576B。
- runner EPYC **9V74/4vCPU**，cgroup quota UNKNOWN，host busy max72.80%、CPU PSI some avg10 max32.22、softirq max16.80%、steal max0；client132.39 + server132.63 = **265.02 CPU-s**，合计1276.24 CPU-s/有效GiB，peak RSS约98.50/98.12MiB。Go malloc/alloc总账**NOT_COLLECTED**，不是0。

独立核对 artifact内 `manifest.json` SHA256 `7adb31294428f9a9fdb97e21bdb068183a45922c18dcd0f7d61a170de3ac3dd1`，`summary.json` `41179df998130b24f856aa7ba3880af93bfcf57747eba33a66c14ee42e587c07`，`efficiency-ledger.json` `8ca906e459081e7d95713c57557651975b3a029b7a819a514274dda52caf742f`。小证据见 [evidence](../evidence/performance-efficiency-e4-game4-5205-off-run37853468730.json)，不提交raw。

## 保留失败与下一保护门

- 同 helper commit 的 [foundation37853468530](https://github.com/lly8666/wobuzhidao/actions/runs/37853468530) Linux race job113571983626 **FAIL**：`TestACKFeedbackBlockedWriteKeepsNoHOLAndOnlyLatestACK` 见 `ACKWorkerRunning=true, Attempts=1, calls=0`。测试在native emit callback中先 `acks <- seg` 后 `calls.Add(1)`，收到ACK不等于calls已更新；未见data-race detector报告，**断言/同步竞态待核验，不标race PASS**。下一提交对该测试加明确阻塞写entered握手及同步关闭断言；不sleep/放宽门，不触产品行为除非确认产品错。
- Game4旧source [37817466498](https://github.com/lly8666/wobuzhidao/actions/runs/37817466498) S2C stress446个UDP缺仍FAIL；[37851028041](https://github.com/lly8666/wobuzhidao/actions/runs/37851028041) server AF_PACKET drops140仍FAIL；旧TCP-only 37762364098 注入不足/S2C hash错仍OPEN；最大UDP 8937/65507B未验，E7约80秒S2C中断OPEN_DEFERRED。
- 接下来**先修test的确定性同步并在Actions跑定向重复+race**；固定产品SOURCE/helper后补 Game4 lossless同seed1840、Normal1 0loss/5205 seed配对、Game2 0loss/5205 seed配对，均独立Action，一次一测量job。必要Game诊断ON/低RTT稀疏保护另run。新helper commit不得偷偷改变产品SOURCE。
- CPU收益需父/候选每至少3条同CPU/资源层、交付一致的profile OFF独立样本；本条9V74不能与7763的383.92横比。未经门槛不继续新增性能优化。E6/P6/物理NOT_RUN。
