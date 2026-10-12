# 2026-10-12 N2 M3：历史窗口32→16的唯一软参数候选，尚未通过Actions

真实产品 SOURCE `08e63965dd9b2910e2e8ea073a22bbc3d0032db6` C03 Action38212621418与 `c835c6d9986a1a5ce7838c4bb63e1dbcb169c8e0` C04 Action38217154364 两个**独立配置**的同类恢复5%持续过度保护证据：C03 C2S末60s在冻结R8–R10范围仅0%、C04 C2S58.3%、S2C41.7%，均小于90%工程screen。两项均能在20%损伤实际维持R16–R20、最终干净阶段分别降R4/R0，且无>0秒持续首次交付空洞；C04初始1%丢330/497包如实保留。确认C04 Action绿色和模型结果不能直接叫N2整体效率PASS；C03旧41/46秒业务中断 historical FAIL未覆盖。编码器active FEC source字节不等于全程包括off的业务，parity成本与总wire分别报告。STATUS与 `docs/evidence/adaptive-n2-c04-offstart-scoped-pass-recover5-efficiency-fail-20261012.json` 持有精确run/job/artifact/hash。

定位：C03/C04两端真实反馈序号~0.5–0.6 qualified PN window/s，旧`confidenceHistory=32`条可保留上个20%强损约55–64s墙钟；在120s的恢复5%段，Wilson upper在500～560秒仍接近15–17%，而系统还需source资格、512新记录、30s稳定、slow EWMA15s和每档10s冷却，导致实际5%阶段未能及时进入R8–R10。不能只取最终一个HOLD，也不能假设真实线路失包由R推算为0，不能使用netem测试阶段为产品切档。当前纯软计数窗口长期墙龄还不够可辨，变短会对突发有风险，所以只减**历史已接受窗口保留数**，不更改最少source样本、M/R可信上下界、payload组k、Wilson和慢速估计、稳定期或任何认证/密文/Generation/MTU/资源/3s/4096的硬门。

修改`internal/adaptivefec/controller.go`的`confidenceHistory=32`到16，预期密集输入下最多约16*256=4096个旧PN而非8192个，旧20%记录约28–35s后完全退出。现有`controller_test.go`的历史数量断言改为引用命名常量而非不可变32，并新增`controller_history_transition_test.go`直接用authenticated qualified、互不重叠有实际M=R界的20%16窗口→5%16窗口→clean16窗口验证累积历史精确淘汰；replay/capacity/未资格化不能伪增zero样本。尤其不降低`MinDecisionRecords=512`或趁机调`offStable=60s`，无新CLI/JSON/catalog/GUI参数需要同步。独立数学参考阈值和冻结曲线不变，也不改N0/N1/N3代码。

**当前只是代码候选，不能断言修复成功。** 先GitHub next-foundation全Go unit/race、Windows/Linux、N0 sameSeq及lifecycle通过后，再单独新SOURCE新性能Action做最小20%120s→5%120s持续双向10Mbps、各300ms真实路径one config/one scenario/one measurement job，报告实际20%损伤完整交付+missing、R档、恢复时间、真实source量/窗口资格、M/R和parity总bytes/p99/CPU/队列、老新结果不跨VM CPU相减。应将理论B层时间带在测前引用并对“最后60秒90%”解释不得测后放宽；如果反例不支持则缩小定位不反复选seed。验证基本通过后再原C03/C04、GE burst、非对称、反馈丢失/稀疏/大小包/Game fixed。模型 `n2-independent-ge-ctmc-v1` GitHub Math Action38217981601已单独PASS（非性能），GE突发潜在0.1%不可达不能强制升R20。

N2未完成自动省资源/CPU安全资格，N3–N6未开始；physical NOT_RUN，历史2.73s探针尾和80s S2C中断OPEN；不部署不动qualification ref不合并主线。
