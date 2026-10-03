# Stage4b initial server batch wiring fix

## 本轮目标和阶段
Resource sequence4b. Candidate6351a82b044d3de34695dec3bcea8f7fe27f8322, next/tlslike-dataplane. First actual5205 runs exposed missing native server TX activation; stop progression and fix.

## 修改与原因
LifecycleServer first-admission call omitted the optional EmitBatch argument, while replacement config and legacyServer already supplied it. Pass s.cfg.IO.EmitBatch in initial AttachServerAdmission call. No workload/parser/threshold/FEC/window change. Existing strict native-path guard remains mandatory and is the end-to-end regression for bothNormal andGame initial lanes; do not relax it toclient-only.

## 复用来源
Current runtimeowner.AttachServerAdmission optional batch API and lifecycle first-admission path, no old code.

## Actions证据
6351a82 foundation37101785003 andtargeted37101785044 PASS. Parent07ad1e5 foundation37101733199 and36sample lifecycle+aggregate37101733181 SUCCESS. Parenttargeted37101733217 FAILED: lifecycle_test.go:177 wait for transient replacement overlap timedout3.04s inLinuxrace job111142456959; no DATA RACE detector report. Same historical timeout documented20260924-124500; preserve failure, not diagnosed asrunner orfixed here. Latesttargeted passes do not erase it.
Actualperformance runsNormal37102098167 andGame37102099835 bothoriginal5categoriesPASS, butworkflowFAILED mandatorynative-path guard: server send_multi=0 (all1packet syscalls), nofallback. Collector37102054999 thereforeFAIL, correctdecision; nostage4bPASS. NormalserverRX506884calls/1361662messages,273453multi;TX1551706calls/1551706messages. GameserverRX568121calls/1597193messages,319535multi;TX1819973calls/1819973messages. This is a realwiring omission, notVMcapacity. Failedoriginalruns/artifacts retained.
This newsource must pass ownfoundation andtwoindependent5205; NOT_RUN. No localbuild/test.

## 问题、排查与风险
Synthetic tests had no native batch activation assertion onserverinitiallane; strictfeatureguard caught bypass. Partial-send/native wire tests alreadypass. Keep callbacks synchronous/bounded andoldper-packetfallback available. No result inherited fromfailed6351 samples.

## 下一项原子任务
Freeze correctedexactSHA; foundation thenoneindependentNormal10M/Game3M5205, compare same4a baseline, requirebothRX/TX native multi>0 andno fallback, originalfivegates/RTT/loss unchanged. Onlythenclose targeted optimization sequence.
