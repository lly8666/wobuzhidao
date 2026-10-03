# Stage4a ready-only receive batch

## 本轮目标和阶段
User resource optimization stage4 split RX/TX into separate atomic gates. Start2f0086cc304dbc145c4aec3d6713ebebfd1fb623 onnext/tlslike-dataplane.

## 修改与原因
Linux raw AF_PACKET uses recvmmsg(MSG_WAITFORONE), at most8 packets already available. Never waits for additional batch members after first packet. Fixed8 full65536+64 byte scratch slots per endpoint (extra~449KiB), preserves exact-sized owned packet returned to caller, packet order/parser/checksum/MTU. ENOSYS/EINVAL fall back to existing recvfrom; EINTR/EAGAIN retry/close behavior unchanged. OptionalPACKET_IGNORE_OUTGOING kernel filter plus existing user filter, unsupported option ignored. Diagnostic-only syscall/message/multi/fallback counters avoid per-packet production statistics and blocking read-lock snapshots. Client/server existing diagnosticJSONL gains raw_io. x/sysv0.33 alreadypresent is promoted direct; no new dependency/version. Foundation adds Linuxarm64 crosscompile. Unit socketpair exercises native batch ABI, actual3packet/1syscall, retained ownership on refill, sparse1packet no fill-wait, truncated-frame discard and legacyreceive path.

## 复用来源
Current raw endpoint parser/serializer/ownership. Native mmsghdr uses architecture-specific x/sys/unix.Msghdr/Iovec, no hardcoded64bit layout. Primaryreferences https://man7.org/linux/man-pages/man2/recvmmsg.2.html and https://raw.githubusercontent.com/golang/sys/v0.33.0/unix/ztypes_linux_arm64.go . No archived code.

## Actions证据
Stage3 source2f0086cc: foundation37100253684 PASS all build/unit/race/fuzz; recovery37100253722 andlifecycle37100253751 PASS. Independent5205 normal37100447253/game37100449153,collector37100329568 PASS. Normal9.998993/9.999524M, stressloss0.006741%/0%;Game2.999872/3.000032M and0%loss; socketdrop0. p95 maxdelta6.01ms,p99max4.25ms. NormalCPU89.25/91.38 vs53.72/54.02;Game75.17/70.52 vs74.78/70.21. WeaknetSACK feedback almostunchanged ACKbytes/PPS, so no CPU/bandwidth gain claimed forACKstage. HostedCPU heterogeneity unresolved; performance input/capacity gates PASS andnoqueuesocketdrop. Receipt docs/evidence/resource-stage3-5205.json. Stage4a thiscommit NOT_RUN; no local tests/build.

## 问题、排查与风险
Additional fixed scratch is userspace receive workspace, not expanded SO_RCVBUF or repair queue. NativeLinuxamd64 execution andarm64 compilation do not establisharm64physical runtime qualification. Older kernel fallback remains functional. I/O stats explicitly enable only withexisting diagnostics.

## 下一项原子任务
Gate exact-source foundation thenone Normal10M/1lane andoneGame3M/4lane5205. Only then implement/send teststage4b. Fullmatrix/longsoak/P6/P7 remainNOT_RUN fornewproduct.
