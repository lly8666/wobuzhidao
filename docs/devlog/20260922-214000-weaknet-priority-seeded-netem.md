# 20260922-214000 弱网优先级与 seeded-netem preflight

## 目标与依据

基线 SOURCE_SHA:
`c300898182f08895d282418a278fcb172375f2a1`

用户本轮明确：高丢包验收首先回答业务是否持续、救回多少、延迟/排队与资源是否有界；严格TCP外观和单次lane candidate成功不是最高目标。低丢包仍应减少无必要repair/gap forgiveness。FEC20:20为主冗余但不是万能保证。任何外观让步不能取消同Seq同密文、MTU/checksum、nonce唯一、隔离、no-HOL和有界状态等硬正确性。

该修正只改变解释/轮换门控，不降低 WEAKNET_QUALIFICATION 既定业务loss/goodput/RTT/post5/resource门。

generation2:
https://github.com/lly8666/wobuzhidao/actions/runs/35729779643

同SHA基础：
- targeted https://github.com/lly8666/wobuzhidao/actions/runs/35729779654 — PASS
- foundation https://github.com/lly8666/wobuzhidao/actions/runs/35729779693 — PASS

## generation2 原始事实继续保留

6个lossless样本完成固定120s+10s并有完整pcap/stage/resources/diag；INPUT_VALIDITY/CAPTURE通过，但全部未满足目标速率性能。典型：
- Normal seed202：socket skmem drop max=92882；client/server CPU 131.04/113.06s；C2S约0.36–0.52Mbps、S2C约6.73–7.04Mbps；artifact 10694503751 sha256 `a95ebc55dc98daaef7d7257a3ee77cea7900f6d111e53fe07db33cd567a1da64`。
- Game seed202：socket drop=17507；C2S约0.59–0.88Mbps、S2C约3.0Mbps；artifact 10695006650 sha256 `5268553c0b928c9535c5d064d059db395b8978e2d782bbc7a9b179863d090145`。

这些仍是FAIL/CAPACITY_LIMITED，不因本轮口径修正变绿。

12个5205/5305样本未产生完整stage事件且旧validator因缺pre_start traceback。stager使用 `netem ... loss random X% seed N`；本轮先将注入器兼容性作为harness问题独立preflight，不把这12个解释为产品弱网FAIL，也不择优拼接。

## 文档同步

- DEVELOPMENT_PLAN：明确低丢包TCP-like hygiene与高丢包业务优先；保留fresh优先、有限gap forgiveness/retirement；禁止新固定loss阈值模式；lane candidate失败时旧lane连续性优先。
- WEAKNET_QUALIFICATION：新增1.1节，明确FEC/repair/最终业务收益不能相加，先查重复repair/queue/local drop；rotation按多次candidate与最终恢复验收。
- ACCEPTANCE：单次candidate失败、TCP乱序/duplicate ACK/同密文有限repair不再直接等价业务失败；硬正确性与业务性能门保持。

## active strict analyzer同步

`tools/check_strict_weaknet.py`：
- application duplicate/corrupt、lane/source mismatch、decoder failure、同Seq不同cipher仍为硬CORRECTNESS；
- decoder已见重复、transport duplicate、ForgivenGaps、Abandoned、repair与late-first-arrival进入非门控 `transport_hygiene`；
- pcap按阶段新增duplicate ACK和fresh seq regression计数，仅解释不门控；
- 新增 `recovery_accounting` 分阶段拆FEC reconstruction/recovered、repair selected/attempt/failure/deferred、fast/RTO、forgiven/abandoned/duplicate；
- socket skmem drop按namespace/socket归属；
- stage事件不完整时写结构化HARNESS_INVALID，不再KeyError。

`aggregate_strict_weaknet.py`：
- 无效/失败样本不强行做RTT比较；
- 汇总transport_hygiene REVIEW但不将其作为第六个隐形PASS门；
- 18主测原有五类classifications和RTT门不降低。

## seeded-netem预检

本候选不自动启动18主测。严格workflow临时改为workflow_dispatch-only。

新增测试专用固定tc：
- iproute2 7.2.0；
- kernel.org官方sha256: `4c2fa124c2cf0afd7ca34d1eeacba6ba048a56f6374e2aab93dafbdbd4eea9c0`；
- 只构建tc到runner临时目录，不进入产品发布物；
- strict stager显式 `--tc-bin`，失败时写harness_error原始命令/stdout/stderr。

`next-strict-harness-preflight.yml` 只创建临时veth并分别用seed 123456/654321配置20% random loss，保存tc版本/kernel/qdisc JSON。只有exact-SHA preflight PASS后才恢复generation3自动触发。

## 不变项

- 不改产品FEC/repair/padding/MTU/socket buffer/default。
- 不跑旧31分钟soak。
- 不用降速结果替代10/3Mbps主资格。
- 不把candidate一次失败、TCP抓包乱序或duplicate ACK美化成PASS；它们造成的实际业务loss/latency/resource失败仍由硬门如实失败。

## 本提交后的测试

只等待GitHub Actions：
1. next-strict-harness-preflight；
2. next-p4-steady-targeted；
3. next-foundation。

三者闭合前不启动generation3。
