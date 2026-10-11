# FEC off 弱网性能考核：off-weaknet-v2

用户于2026-10-11明确修订。适用于当前 adaptive 分支今后 FEC off 的弱网性能考核，不重判任何历史 run；fixed FEC20:20 formal20、其他档位和 lossless 原有分析结果保持原归属。本文件优先于历史文档中套用到 off 的“业务 loss 不超过线路 loss / goodput >= (1-p)*99%”条款。

## 生效与迁移

此节初稿在政策提交 d2ddbd9 时是 docs-only，随后已实施独立 off-weaknet-v2 分析器并完成定向 Actions。初稿的“尚未实施”是历史当时状态，最新结论仅以唯一 STATUS.json 和对应精确 SOURCE、harness 的原始 receipt 为准。冻结的历史 run 和旧 FAIL 不改写。

当前开发 agent 先结束这波工作，再独立修改 off profile 的分析器与定向验证，输出版本 `off-weaknet-v2`，并继续原 N1→N6 任务。旧参考分析、原 FAIL 和原错误列表必须保留；新版本用新 receipt/独立字段记录。非 off、formal20 和 lossless 不能无意改门。未实施写 NOT_IMPLEMENTED，未跑写 NOT_RUN。

## 1. 仍然必须考核性能

FEC off 不是“任意损失都能过”。真实业务首次交付、低延迟、无跨业务 HOL、连续性、资源有界和弱网恢复仍是目标。禁止主动丢 fresh、加强重传或等待洞来满足吞吐数字；有限外层修复保持既有语义。

永久硬门：payload/hash完整性、认证/账号地址隔离、same Seq same wire、generation/PN、合法 MTU/checksum、配置生效、owned 清理与资源上限。超MTU业务自身重组可以失败，但不能堵住后续独立业务。

输入/环境独立判定：真实发送量、注入波形、send failure/lag、runner型号/指令集/quota/PSI/steal、产品与发生器CPU、raw/socket/内部queue drop、有效观测覆盖。注入不足或环境容量受限分别标 INVALID/CAPACITY_LIMITED，不能写 PASS，也不能把系统drop归为FEC理论损失。

## 2. 无损与弱网分开

**用户补充的设计预期**：FEC off 时，在有真实随机线路丢包的情况下，业务包丢包率与线路 p 同数量级、甚至大致相近，是完全正常的预期行为，不是需要加强重传、等待 ACK 或阻塞 fresh 来修好的缺陷。业务包率不保证精确等于 p；有效 LINK 分片可能增加业务包/字节损失，已有的有限修复和重排也可能降低最终缺失。**不能因为 off 业务丢包≈p、或某些尺寸的业务字节损失高于 p 就单项判产品退化，更不要求弱网 off 零丢包。** 仍严格评价 payload 完整性、真实首次唯一交付、长尾 p99/missing、无跨业务 HOL、连续交付、CPU/GiB、队列年龄/排空和 wire 开销；无有效可比对照时仍为 MEASURED_NOT_QUALIFIED/REVIEW_REQUIRED，不自动性能 PASS。flat1/flat5 三段损伤恒定，末段是稳定性观察，不是20%降到5%的恢复。


**off + lossless**：保持现有无损保护和旧分析器行为。健康容量下实际已发送、合法、充分drain的UDP应正确完整交付；吞吐目标和旧 FAIL 不借本次修改放宽。跨版本CPU/p99/goodput变化结合重复性与环境比较。

**off + 5205/5305及其它受损场景**：不再把以下条款作为 off 的绝对性能通过门：

- 业务包损失必须 <= 配置外层丢包率 p。
- 首次唯一交付字节吞吐必须 >= target * (1-p) * 0.99。
- post5必须在10秒内连续三个1秒窗口同时 loss<=5%、goodput>=target*0.95*0.99。

以上旧结果继续输出为 `legacy_reference`，不能删除、改写为 PASS 或隐藏。

## 3. 分片、字节与修复必须解释

包损失、字节损失、及时 goodput、drain后的最终交付分开记录。不要把最终迟到交付计入当期吞吐，不用 returned-only p99 隐藏 missing/timeouts。

仅在单次首发、各外层片独立同概率丢失、业务需全部 m 个外层片才能交付的假设下，无修复业务包损失参考为 `q(m)=1-(1-p)^m`。例：p=20%、m=2时q=36%。这不是生产有限修复或真实TCP业务的精确预测。

按发生器真实业务包/字节分布与实际有效 MTU/LINK 分片路径计算参考：包参考对q(m)按包数加权，字节参考按业务字节加权。m须来自有效分片映射/有界观测或可信同源预算推导，不能只凭应用payload大小猜测，不能用一个平均m代替混合分布。缺映射则标模型不可得，不伪造精确理论门。

区分OS IPv4分片、LINK分片、独立业务和内层TCP自身恢复；记录已确认的有限repair、重排、重复与本地drop。相关/突发损伤不套IID硬门；Game共享路径副本不假定完全独立。不据模型参考单项自动PASS或FAIL。

## 4. 新性能判定口径

至少报告每方向/阶段的业务包与字节损失、及时first-unique goodput、p50/p95/p99与missing/timeout、最长连续零交付、CPU-s和CPU/GiB、RSS、队列年龄/排空、wire放大、修复量与控制开销。

有足量、有效、同配置旧版对照时，采用冻结相同夹具、负载/seed/损伤/MTU、可比runner资源的独立样本筛查退化：及时goodput稳定下降约>5%，或CPU/GiB/p99稳定恶化约>10%且超过噪声带时列REVIEW_REQUIRED；低RTT p99保留至少5ms绝对噪声带。实际缺失增加须单独解释，不能用失踪探针让p99变好。一次跨不同CPU的结果不是优化或退化证明。

筛查不是机械新硬门：重复且可归因产品的退化写FAIL；环境不可比写INCONCLUSIVE/CAPACITY_LIMITED；有效单样本无配对或模型不足写MEASURED_NOT_QUALIFIED/REVIEW_REQUIRED，不因删掉旧阈值就自动写性能PASS。没有证明退化也不等于完成性能资格。

post5恢复改为相对**该样本pre5稳态**评价：用预声明的至少5秒窗口，记录goodput、缺失、queue age回到自身pre5稳定范围的时间。现有10秒作为恢复筛查/观测起点，不用单秒随机波动或任意三个1秒窗口直接判坏。范围与容差在看结果前依据pre5采样波动及参考设定，不结果出来后临时放宽。若到期未恢复，列REVIEW_REQUIRED并检查后续窗口、队列是否持续增长；严重或可复现的产品断流/队列不排空保持FAIL/OPEN。

主连续负载中 >=2个完整秒无业务交付的旧连续性检查继续保留，不能被本次取消吞吐公式顺带删掉。预声明的真黑洞/无注入窗口另行排除；普通随机loss不能默认解释成长断流。无HOL还需要定向缺片/丢旧record期间后续独立包继续交付的证据。

本规则不为FEC on统一规定业务零loss或0.1%；on按原本对应的k/r、partial、有限repair与正式验收处理，不借本次修改重判。

## 5. 分析器迁移要求

仅对 profile-screen 的 parity=0 且受损场景建立新分支评估；保留完整原始参考分析。分析器应独立输出正确性/输入/环境/连续性/质量测量/性能资格，评估不足不得包装成 PASS_SCREEN 或 ACTIONS_PASS。有效测量而尚待性能复核可以有明确MEASURED状态，但必须避免aggregate和STATUS把workflow成功等同性能通过。

新增 meaningful 定向分析器验证：合法分片放大不能因loss>p自动失败；相同原始数据的损坏/配置错/注入不足/严重断流仍被识别；无对照仅测量不资格；非off/formal20/lossless保持原门；新旧receipt并存。全部测试在Actions，每性能Action一个SOURCE/配置/seed/场景/测量job。

新样本记录精确产品SOURCE、harness/analyzer SHA及hash、policy version和实际配置；不修改正在跑的样本，不回写历史FAIL。相关控制质量反馈是否持续成熟仍是独立N1功能门，不能因off吞吐门修订而跳过。
