# E0：Game4 单条profile-on真实lane/成本诊断助手（2026-10-08）

本轮是单个**诊断场景资格**，不是产品性能优化，也不是无损Game4性能保护PASS。父分支HEAD `54a99f8c05a70ccb2148c789ea9ac053f053d0e8`，产品SOURCE始终 `bf11fbfbe64d518e7ba189d51bfb4512df4df733`，源码FEC/4096/MTU/record/generation/lifecycle未改。现有Game4真实profile-off [run 37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445) 原始 **FAIL**，C2S UDP2154/103062 datagrams missing、C2S/S2C probes 26/33 missing，两方向HTTP/TCP完成但仍不合格，见[原始失败证据](../evidence/performance-efficiency-e0-game4-fail-37766819445.json)。本轮不得悄悄重标。

## 为什么是诊断单样本
上条Game4 HOST AMD EPYC7763,4vCPU，产品合计CPU350.67s，host busy79.83%、CPU PSI35.66，socket/interface drop0、quota未知。既不能按无drop排除调度瓶颈，也不能把没确认的容量问题伪作CAPACITY_LIMITED。普通off诊断缺少运行时Game四条物理incarnation是否持续有效、实际PacketID竞速/去重、每lane源/校验/repair、raw syscall/batch成本，不能据传输设计自动确认每lane有效。

## 仅助手改动（自己的新helper SOURCE需自己验）
- 单配置增加`diagnostic_mode=on|off`，当前且仅此条选`on`：Game4、逻辑3Mbps/方向、TCP+UDP mixed、300s lossless、300ms单向、seed1814、3s drain、FEC20:20/padding off、client/server formal auto-record0、outer1400；所有profile/off从此显式设off，与诊断不同run，禁止同run A/B。
- Workflow `next-efficiency-e0-single.yml`仍仅一个`one-fullstack-sample`测量job，profile-on单独导出`WBD_STRICT_CPU_PROFILE=1`及`WBD_EFF_DIAGNOSTIC=1`；helper脚本已现成按此开关保留两端diagnostic-jsonl，并启用既有产品可选CPU/block/mutex profiling。只上传小`*-cpu-top.txt/*-block-top.txt/*-mutex-top.txt`数值摘要、摘要JSON、MTU、resources/netem和pcap hash删除回执；不上传raw pcap和敏感正文。
- Analyzer新必选`--diagnostic-mode 0|1`，manifest on/off必须与真实环境一致。off出现diag立即FAIL；on缺客户端/服务端JSONL、解析错误或真正4-lane GameLogicalOutbound/GameLaneCopies=0时明确FAIL；不能只靠CLI或四条TCP flow冒充Game竞速。输出`runtime_game_evidence`仅数值/引用，不含正文。
- cost ledger诊断分支从已存在的`product.owner/lanes`或`product.tunnel.owner/lanes`抽真正Desired/Active/Physical/GameLogicalOutbound/Copies/Delivered/Duplicates和各lane generator、FEC source/parity/decoder/reassembly/repair等有界摘要；GC/alloc计数delta、raw收发和batch仍有来源标识。**诊断on CPU与off绝不作为AB收益**。

所有代码在本轮GitHub push后的Actions上py_compile/build/real fullstack执行，不在物理机或本地跑新测试。新source SHA以workflow checkout的完整`GITHUB_SHA`为准，push前尚无诊断结果/不可写PASS。上一条Game4 FAIL绝不删除。下一步读取Actions run/job/artifact，若诊断本身失败区分fixture/host/product与原Game4业务FAIL；根据热点和失包发生边界择一定位。当前 E0尚未退出，原TCP-only10Mbps FAIL未改；E1–E6、同源P6/manifest/长测未做，E7约80秒下行断点仍OPEN延期，不写PHYSICAL_PASS。机器证据：[本轮诊断候选](../evidence/performance-efficiency-e0-game4-profileon-candidate-20261008.json)。
