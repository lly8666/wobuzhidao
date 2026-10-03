# Stage4b feature receipt envelope correction

## 本轮目标和阶段
Stage4b candidate07ad1e575107fe6ee5dd445d18f9ee261d8d7938 is awaiting foundation; no performance dispatched. Read qualificationdiag.Sample before dispatch and found raw_io is under the product envelope.

## 修改与原因
collect_raw_io_receipt.py reads row.product.raw_io rather than row.raw_io. Product Go code, workload and analyzer unchanged. Corrects a deterministic false missing-diagnostic error before any performance sample; does not relax native multi/fallback checks.

## 复用来源
Current internal/qualificationdiag/jsonl.go Sample schema1.

## Actions证据
Parent07ad1e5 foundation37101733199 pending, performance NOT_RUN. This commit requires its own exact-SHA foundation and5205 receipts; do not inherit parent checks. No local tests/build.

## 问题、排查与风险
Feature receipt is supplementary, not original five-category quality verdict. Missing/disabled counters still fail explicitly.

## 下一项原子任务
Wait exact newfoundation then independentNormal/Game5205; read both receipts and original gates before sequence completion.
