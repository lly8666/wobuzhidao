# 20260923-200000 drop-trace 改用无regex token parser

## 状态

5205 canonical `loss-tolerant-v1` 仍明确报告 `server/ss_packet drops=164`，因此 ENVIRONMENT FAIL / PERFORMANCE CAPACITY_LIMITED 不变。

第二次零采样reader run `35847893659` / job `107138359924` 仍输出timeline=0，说明仅修正正则文本还不足以解释诊断reader差异。

## 改动

本提交只修改 `next-performance-artifact-reader`：

- 不再用正则解析 `skmem:(...)`；
- 对每行用 `find('skmem:(')` 找起点；
- 截取首个 `)` 前的body；
- 按逗号拆分；
- 按固定key顺序读取 `rb/tb/bl/r/t/f/w/o/d` 数字。

这样避开JavaScript生成字符串、YAML、shell heredoc、Python raw-regex之间的多层转义。

仍读取同一immutable artifact `10744205038`，不产生新性能样本，不改产品或验收门槛。

## 下一步

第三次reader必须给出164 drop的transition。如果仍不一致，下一步只输出原始server ss_packet文本样本，不再继续猜解析逻辑。
