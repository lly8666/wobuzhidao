# N2 M0 独立模型/旧540s只读分析 Actions 成功收据（非产品资格）

2026-10-12。接上轮候选 SOURCE `8b5b08918189e42816d82ef8661bc17704c3604b`（本轮产品 runtime N2 核心完全未更动，历史实际产品 SOURCE 仍 `90925f81cfb16e8435c238b060c46492eef017ea`）。

## 实际验证（Actions only）

模型/只读分析工作流 `next-adaptive-n2-independent-reference-audit`：Action [38202228800](https://github.com/lly8666/wobuzhidao/actions/runs/38202228800)，job `114657908330`，`success`，产物 `11726191066`，archive SHA256 `0f1ceda0b43a4a1c0f13b4ab2f2dd89c03e72c413e8c6f599ab7ad942325f858`。模型输出SHA256 `bf71f41d420d96c3c46f97b7370cbcfab3bd7d521176d09671773ae83cc0f1bd`；只读诊断输出SHA256 `98c24d5df18705acc0eb38708b703fa166b6abc77d133d77d51a00c93691c292`；产物 receipt SHA256 `3db625657ff83fdb9135854a7b1850f500ba75f6684222cc571e8189b790883c`。下载的是**前一Action** `38161148715` job `114534148614` artifact `11710956407` 的原始 summary/双端有界诊断，而不是新起性能测试。原 summary SHA256 `85163b36ba051a76adf33fef5d748e8a224e0e8d5fac2003270d5aa52bfc8450` 由工作流校验，删除临时原样本后只上传汇总。

数学侧不调用产品 Controller.Step：枚举独立 binomial MDS partial k/r 验证通过。预冻结完整 k20 IID 档，C03=4→4→8→16→8→4；C04=0→4→8→16→8→0；C08 1%为4；C09 5%为8。B层输出报告/成熟/RTT/有效报告率/历史/稳定/冷却的独立时间带是**假设模型**，不同真实k分布/突发必须另外计算且不能测后修改阈值。模型输入为 `docs/evidence/adaptive-n2-curve-freeze-v1.json`，模型/分析器版本分别 `n2-independent-iid-timeband-v1`/`n2-evidence-audit-v1`；未发任何新性能Action，产品不能读取参考。

`next-foundation` 同SOURCE [38202228724](https://github.com/lly8666/wobuzhidao/actions/runs/38202228724) 已核验其七项实际启用job success（包含 Linux/Windows Go unit/race 等），skipped 不记PASS；它不代表新理论曲线产品已经通过。

## 关键证据与限制

最后活跃240s C2S 逐秒全为 `CONFIDENCE_RESIDUAL` 降档gate，但累计监控reason有 `HOLD_UNQUALIFIED_FIRST_ATTEMPT=61`、`HOLD_UNVERIFIED_PARTIAL_K=87`、`HOLD_STABILITY_OR_CONFIDENCE=92`。不能只看一次末尾HOLD；这里last-qualified gate持续是旧已接受source评价，UNKNOWN时间并未推动控制器。C2S在428.058/429.058/451.058/507.058s有已完整source报告 `M=0, M+R=1/8/5/2`；仅是同wire repair**上界**，非实际线路p。32历史窗口与256 PN截断组source的保守处理会使Wilson残余持续较高。C2S最后Wilson upper仍0.3019%，S2C为0.04687%，S2C降到off但C2S仍20；额外保护的业务收益/CPU不足以从此前结果证明。逐秒采样的 source seq 增量近似0.5报告/s，不表示实际全部回声可靠。

成因尚非单一可证：无法判断哪些相同Seq repair来自真实原始loss与ACK/调度重试，也不能把边缘部分组一律假设实际k20。现有观察不授权把R从可信上界删掉、netem真值直通生产、强行清零估计或直接硬降。因此核心生产修改要通过低丢包/真实突发及不对称等工况验证；目前N2保护和省资源效果保持OPEN。诊断ON CPU数字只能描述、不能当普通OFF优化。旧2.73s probe/80s S2C OPEN，physical NOT_RUN，N3–N6未起步。

## 本轮动作

把 `docs/evidence/adaptive-n2-m0-actions-receipt-20261012.json`、此devlog和唯一STATUS在单次提交同步到分支。没有新增产品配置选项，不需修改CLI/catalog/GUI。本轮不补跑大netem矩阵，不动主线、不移动qualification ref、不物理部署。后续严格先最小纯功能N2反例、再独立一条真实场景，每性能Action严格单SOURCE/config/scenario/measurement；必要小改参数必须有跨工况证据，不能把M0 Green写成N2 PASS。
