# 2026-10-12 N2 M1：目标不可达时有界边际收益与慢降档软门候选（Actions 待测）

基线：`8b5b08918189e42816d82ef8661bc17704c3604b` 参考工具提交；`30ba590298b50551466b66d7bffefbc6cc5f7085` M0 Actions receipt，原实际产品 `90925f81cfb16e8435c238b060c46492eef017ea`。本轮计划修改生产 `internal/adaptivefec/controller.go` 与只含 Go 测试的新文件，专用 Actions `.github/workflows/next-adaptive-n2-marginal-core.yml`；**尚未运行新的netem/performance，改动未验证前不称PASS**。

## 可复现失败的源证据与独立验收依据

M0 独立模型 Action38202228800/job114657908330（artifact11726191066）PASS，计算验证了完整k20 IID的理论档；用户最低0.1%模型残余目标与稳态理论档或高一档的验收范围保持冻结 `docs/evidence/adaptive-n2-curve-freeze-v1.json`，**没有为当前实际产品改动放宽这些范围**。540s旧 Action38161148715 的 C2S最后240s活跃业务中 240/240份逐秒快照的 lower gate 均是CONFIDENCE_RESIDUAL，C2S在clean段有多个 `M=0,R>0` 的正上界尖峰（429s R8/451s R5/507s R2），真实第一发loss不可辨；前32个合格窗口+边界 croppedSource 按未保护计价导致即使k20完整组已有冗余，绝对理论 q_mix 也可能因为边缘未知而对所有档超过目标，旧`requiredProfileWithEdges`此时无条件返回max20。原始跨方向对照 S2C已降off，但不能仅凭此就抹掉 C2S保护需求。

## 最小候选与定量风险

在**有界并已合格的source-correlated闭合k/r证据**下，当保守的 q_mix(R) 绝对目标不可达，逐档计算相对最高保护档的残余差，选首个 `q(R)<=q(best)+0.0001` 的允许档；0.0001是目标0.001的10%（不是线路p，不是实际K/R组形状造假）。这样对稀疏k1在高损5%时所有档只能p²=0.25%，不再因目标不可达无意义地升20；高损有明显真实已闭合组收益时仍可加档。对于下降，若保守 Wilson 上界残余超越旧0.0005慢降门槛，但 prior 与 current 在**同一保守source M+R及真实闭合组/unknown edge证据**下相差<=0.0001，可允许**一次逐档**慢降；不改历史消退、512有效source最少量、30/60s稳定、10s每档冷却、慢速EWMA或认证generation/同Seq同密文/MTU/资源界，决不强制下降。

这不是观测到实际0损失的断言、不把UNKNOWN变0、不把crop视为全k20，也不让外部netem/测试phase触发控制器。**风险**：真实首发loss未被上界准确分辨，低价值模型parity增益可能在相关突发时仍有重要首交付时延益处；纯IID近似不能替代相关突发/重排/partial实测。因此本候选只能在 Actions 通过core/race、低损/高损/突发/不对称以及业务p99/吞吐/CPU/wire扩展验证后判断是否保留，旧540结果不能重分类。

候选内核常数为10%残余目标，没有引入新的**用户可配置参数**，现有 CLI/JSON/catalog/GUI的模式和参数未变化。备选措施如缩短32历史窗口、放宽0.0005绝对margin仍暂**未**修改，以免多变量混淆。

## 各阶段 Actions 验证次序

第一道 `next-adaptive-n2-marginal-core` 仅 Go controller unit + race(runtimeowner/datapath/linkdata) 与 repository policy，**0条性能测量**；若本候选失败按同一 SOURCE精确修复，不能跳过。通过后才基于新的精确SOURCE跑预冻结最小真实场景，每性能Action只一个SOURCE/配置/场景/测量job，先C08/已有540问题同型的定向反例，然后C03/C04/非对称/突发/反馈/稀疏/MTU及fixed/off/Game保护。若连续两次同类无新证据，缩小回单功能定位，绝不挑seed/runner。最终物理仍NOT_RUN、N2 efficiency OPEN、N3-N6 NOT_STARTED、2.73s tail/80s S2C OPEN。本提交仍不动任何用户机器、qualification ref 或主线。
