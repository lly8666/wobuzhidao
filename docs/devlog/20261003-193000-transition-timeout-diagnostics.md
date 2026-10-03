# Replacement timeout diagnostic narrowing

## 本轮目标和阶段
P5/P6 pre-delivery; starting f77d31f. Retain first barrier failure and obtain the actual state before another behavior change.

## 修改与原因
Only test diagnostics: atomic count of barrier interceptions and logging changed server overlap counts plus client owner state. Existing assertions and production behavior unchanged. STATUS updated in the same commit; prior split commits omitted one required continuity file and repository gate correctly rejected dd71808/f77d31f. This is an editing error, not a product test failure.

## 复用来源
Existing owner Stats and test transport. No old code.

## Actions证据
dd71808 repeated race37119536977 FAIL: two repetitions timed out at server overlap despite barrier. No race conflict. Foundation37119536945 repository gate FAIL due missing STATUS update, so product checks skipped. New diagnostic run NOT_RUN; no local tests.

## 问题、排查与风险
Immediate client health alone is not sufficient explanation. Need see whether server never publishes overlap, already retires old lane due server-originated health or the barrier misses actual association. Do not add sleeps, increase production grace or weaken assertion. All soak dispatch remains gated.

## 下一项原子任务
Read repeated-race changed-state logs, identify precise event and implement a deterministic test synchronization. Continue configuration harness preparation independently.
