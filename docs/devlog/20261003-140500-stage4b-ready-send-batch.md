# Stage4b already-generated send batch

## 本轮目标和阶段
User sequential resource optimization finalatomic4b; startingfda4f08ec808855a4451344fef4df7a080b26f2a onnext/tlslike-dataplane.

## 修改与原因
Optional SegmentBatchEmitter added alongside ordinaryEmit. Current record list processed synchronously in<=8chunks; no persistent queue/deadline/batch-fill wait. Controls/health/ACK/FIN/bootstrap still individual. Factor exactexisting fresh sequence/shadow preparation and completion, retain original one-record path when noBatch orsingle. Same record ciphertext clone,4096 backup policy, protectedcontrols, budget andhorizon; exactsentprefix retains repair ownership and refillscredit, knownunsent removesphantombackups/reportsfailures. A partial failure consumes its assigned sequence range (asexisting failedfreshsend) and cannot stall futurefresh. Linuxsendmmsg uses existingMarshalSegment persona/checksum/options, handlesEINTR, exactpartialprefix withoutduplicate retry, ENOSYS fallbackSendto. Chunklock prevents packet-ID interleaving insidebatch; no loss/recovery policy adjustments. SegmentMux validates every flow before emitting anything; allclient/serveradmissions receive optional batch function. Npcap remains singleEmit. Existing raw_io diagnostics gain TX counters; strict workflow verifiesboth nativeRX/TX multi calls and0fallback aftersame workload, uploadscompact feature receipt withsame summary. Analyzer and original five gates unchanged.

## 复用来源
No archived code. Existing runtimeowner send preparation copiedverbatim into helper (only returnshape/indent), shared by bothsend paths. Current rawserializer retained; standardLinuxnative mmsghdr architecture fromx/sysv0.33. Primary reference https://man7.org/linux/man-pages/man2/sendmmsg.2.html .

## Actions证据
Stage4a sourcefda4f08: foundation37100946893 PASS Linux/Windowsunit/build, Linuxfullrace/fuzz, arm64crosscompile andprivilegedrealmodules. SeparateNormal37101084780 andGame371010864715205 pluscollector37101011207 PASS. Normal9.997342/9.999051M, byte loss0.0112%/0.001707%;Game2.999863/3.000075M, loss0%;allsocketdrop0. RTTmaxdelta p956.27ms,p998.93ms. CPU Normal87.79/89.76CPU-s,Game106.0/100.4; independenthostCPU differences not attributed wholly tocode. Stage3 lifecyclefullstack37100253694 alsocompletedSUCCESS, preservesidle/blackhole/wake/generation semantics. Receipt docs/evidence/resource-stage4a-5205.json. Stage4b thiscommitNOT_RUN; no localtests/build.

## 问题、排查与风险
Unit compares15records includingmiddlecontrol single/batch output andshadowaccounting; injected2/4partialfailure testsknownsentownership,unsentretirement/futurefresh. NativeUDPsocket testsserialize10 identicalcompleteIPv4/TCPpackets through2sendmmsgcalls andlegacyfallback. Linux rawIP_HDRINCL fullstack pending. Addedfixedscratch/stackframes bounded8, no tuningbuffers/FECdeadline. Feature receipt alone isnot CPUgain orquality qualification. Batchfallback allowed inproductoldkernels, buthostednativefeaturevalidationexplicitly requiresno fallback.

## 下一项原子任务
FoundationexactSHA thensingleindependentNormal10M/1lane andGame3M/4lane5205. Readfeaturecounters andallqualitygates; ifPASS updateSTATUS/ACCEPTANCE finaltargetedchain,everyactualrunreceipt preserved. Fullmatrix/repeats/longsoak/P6/P7 remainseparate.
