# Deterministic lifecycle transition observation

## 本轮目标和阶段
Pre-delivery P5/P6 qualification, next/tlslike-dataplane starting b13e2c88. Unblock foundation without changing production retirement, reliability or timing.

## 修改与原因
The replacement test polls a transient server A+B state after RotateOldest returns. The replacement's first automatic authenticated health record may qualify B and complete A's FIN retirement before the polling goroutine observes it. Add a test-only transport barrier for replacement payload once the real server association has prepared its transition; bootstrap and pure ACK remain free. Observe both endpoints' unchanged overlap assertions, release the barrier, then execute the unchanged delivery/retirement/dormant/wake assertions. Cleanup releases blocked emitters before closing the client. Production code, grace, deadlines and counters unchanged. Add an Actions job repeating this test 30 times with -race.

## 复用来源
Current runtimeentry test's in-memory transport and actual Association.TransitionState. No old modules or new production hooks.

## Actions证据
b13e2c88 foundation37118772749 FAIL: Linux race test timeout at lifecycle_test.go:177, no race-detector conflict report. Pre-delivery controller37118869828 correctly failed before dispatch; no soak workloads launched. b13 targeted37118772735 and lifecycle fullstack37118772730 PASS. Harness tools37118772759 PASS. New barrier and repeated race NOT_RUN until this commit's Actions. All tests/builds remain Actions only.

## 问题、排查与风险
Transient-state timing is the hypothesis supported by the immediate health/qualification/FIN code path; repeat-race and foundation must prove the change. Do not label the original failure a runner capacity failure or claim a production race fix. The barrier must not block bootstrap or teardown. Preserve the original failed run and controller gate result.

## 下一项原子任务
Require new-source foundation and repeated race success before independent 180s soak canaries. Continue actual configuration coverage preparation while gates execute. Formal1800s, final18, packages and physical remain unqualified.
