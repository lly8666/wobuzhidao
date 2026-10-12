# 2026-10-12 M3 32→16全局历史单测 FAIL；两层已资格化历史更正候选

当前工作分支HEAD首轮失败SOURCE `b5f6047be2b1cc2f52c400f8defba3d73915ecdf`， [Foundation38219612120](https://github.com/lly8666/wobuzhidao/actions/runs/38219612120) 的Linux job114712470162与Windows job114712470204均在active-go-tests中 FAIL；[N2 marginal-core38219612192](https://github.com/lly8666/wobuzhidao/actions/runs/38219612192) job114712415227 的focused unit FAIL，同一实际失败 `TestOffNeedsLongEvidence`：`controller_test.go:128: off entered without 60sec evidence: -1`；[Lifecycle38219612039](https://github.com/lly8666/wobuzhidao/actions/runs/38219612039) PASS。**这些原始红灯永久留在 `docs/evidence/adaptive-n2-m3-first-off-wilson-unit-fail-20261012.json`，不能写成性能证明或抹掉。**无新performance Action。

原因是数学冲突，不是runner/方向/seed：可信零loss的16个互不重叠256PN窗口共4096样本，Wilson 95% 上界约0.000937，高于原 `decreaseMargin=0.0005`，所以 off 连在完全干净条件下都不可达到；32个同规模窗口8192样本 Wilson上界约0.000468，满足。缩短所有历史会改坏进off硬门。不能为了测试绿色放松 `decreaseMargin`、冒充0 loss，或把`netem loss`传给控制器。

这次更正保留`confidenceHistory=16`作为普通**已经启用FEC**档位的快速可信窗口，新增独立 `offConfidenceHistory=32`只有已被现有source资格门确认的不重叠M/R/k和cropped计数（不是业务payload/队列）的固定存储；减少到off之前以32份真实受理样本组合保守Wilson、真实组k/R、FEC-off/cropped形状，从而保留旧0.0005上界和60s稳定/10s冷却。off起步pilot后同时清理这份profile历史，防止之前无k的旧off组污染未来模型。**off候选不能靠微小边际收益跳过绝对置信度**；非off已启用档位仍可在有界且真正无额外保护收益时逐级降档。没有新监控探针/业务流量、fresh等待、额外数据包缓冲，也没有改M/R界、Wilson公式、512最少记录、慢滤15s、认证/完整性、generation/同Seq同密文、SIMD/immediate systematic/partial min(k,R)、32ms/3s、MTU/4096资源硬门。长连接2³¹准入修复保持。

新增 `controller_off_proof_test.go` 明确测试前16个可信干净组**不能**越过off的Wilson门、32个才可、post-repair证据不能进入off证明、只有新的32个真实干净窗口淘汰旧20%损伤；已有`TestOffNeedsLongEvidence`也必须通过，而其他M3测试确保正常已启用档16窗口老损伤淘汰。候选还未在GitHub Actions验证，不称PASS。新ring只保存计数+闭合k组小数组，保持有界内存换有限聚合CPU（在考虑off时重新求和<=32组），CPU优化资格还需真实诊断OFF同机控制。

下一步**先** exact SOURCE GitHub next-foundation Linux/Windows全Go单位/race + focused N2和N0 samewire/lifecycle；一项FAIL立即留证修复，不能先启动性能；全过才规划一条最小20%→5%单场景、双向10Mbps、300ms和提前冻结时带/开销screen反例。进一步跨C03/C04/GE相关突发验证16历史没有过快 underprotection，再考虑N3–N6。原C03/ C04 recover5过度保护失败及旧2.73s/80s OPEN；N2未完成，physical NOT_RUN，禁止部署、移动qualification ref或合主线。
