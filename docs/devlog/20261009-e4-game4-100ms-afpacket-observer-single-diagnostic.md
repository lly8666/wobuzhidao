# E4 profileON 100ms AF_PACKET 独立socket时钟诊断（2026-10-09）

唯一工作分支 `next/performance-efficiency-20261008`，父HEAD `debf470e6875705e9a94ac0efe9523a598039246`，产品 SOURCE `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072` 固定，**无产品代码改动，无缓冲、队列、wire、MTU、socket或物理操作**。上一原子测试 [Foundation37865267829](https://github.com/lly8666/wobuzhidao/actions/runs/37865267829) 完整SUCCESS，含原失败artifact资源时钟相对于business_start前置11.947秒的校正、synthetic零/缺失/多socket/重置clock门。

新增仅用于 profileON 一条 Game4 lossless mixed seed1840 300s+3s、FEC20:20、paddingOFF、3Mbps各向、300ms单向的 **两只独立in-netns AF_PACKET数值探针**：分别每100ms执行一次 `ss -0 -a -m -n`，以真实monotonic调用前后时标界定socket kernel `skmem(r/rb/d)`，每次命令400ms有界超时、303s deadline、每端≤4096样本。只上传数值JSONL，不上传原始ss文本（含地址可能泄露）、raw payload/秘密。保持现有1秒资源、原来的原始业务分析器、所有业务配置、单job单case、严格pass/fail不改变；生成器仅在diagnostic ON插入sidecar且cleanup负责回收；本次 helper改动受控自动触发一次性能Action。预检对probe样本数量、时钟、ss可用率fail-closed；所有business/fec/throughput/probe原分析器仍独立成立。

**局限：** 100ms附加采样本身有CPU/进程调度开销，不能拿这条sample的CPU-s和OFF比较、不能将新的干净runner当作修复9V45 OFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client33/server86的证据；以前profileON 9V74 [37863612374](https://github.com/lly8666/wobuzhidao/actions/runs/37863612374) scoped PASS也仍然独立。旧失败时间轴server业务86.052515–87.052582s/client142.052521–143.052518s仅1s sample row时间，因namespace串行轮询，需此次每`ss`准确bracket。无drop重现应标`INCONCLUSIVE_REPRODUCTION`。CPU获益UNPROVEN，Normal/Game2保护、E7约80s下行中断、E6/P6/physical仍OPEN/NOT_RUN。

**下一项：**读取这一个诊断Actions原始artifact、原始analyzer和sidecar数值，单独更新evidence/devlog/STATUS，不增开第二个run或无界调缓冲。
