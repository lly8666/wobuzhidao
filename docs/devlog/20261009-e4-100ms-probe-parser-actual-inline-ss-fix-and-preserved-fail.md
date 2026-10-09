# E4 100ms probe 真实失败: `ss` 同行skmem解析器缺陷；原始Analyzer PASS独立保留（2026-10-09）

只在 `next/performance-efficiency-20261008`，父 HEAD `4d958ccea9af82fe8dd0c67f38a1b8de409c905a`，产品冻结 `ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`，本次只修只读helper解析，不改变网络/队列/产品/MTU、物理或主线。

单一新性能[run37865738583](https://github.com/lly8666/wobuzhidao/actions/runs/37865738583) job113611907874 artifact11588092908 的**工作流最终FAIL**。严格分层：真实300s+3s全套业务成功，原始业务analyzer **PASS_SCOPED_ACTIONS issues=[]**，ledger成功；新增100ms数值探针audit **failure**，最后的fail-closed步骤 FAIL。必须保留此双层结论，不能把本次workflow称PASS。

原有1s采样共316，client/server各316次`ss_packet`返回成功，AF_PACKET socket-drop累计d0/d0、接口drop0，Game双端各4条active physical、双向UDP各103062/103062，probe1500/1500和1495/1495，p99 614.879274/617.022575ms，TCP各304完整、HTTPS20/20证书10/10。runner EPYC9V74 4vCPU；profile ON + probe 自带CPU负担，所有CPU-s比较 **禁止**。这不推翻先前9V45 OFF 0loss原始FAIL client33/server86。

问题根源通过与此run的两个artifact交叉对照已确认：每端100ms探针恰好输出3150个JSONL，但是字段`ss_ok=false`、`packet_socket=null` **所有样本都如此**，workflow最后显示`PROBE_AUDIT=failure`；与此同时1s `resources.jsonl` `ss_packet.stdout`稳定显示一行 `p_raw UNCONN ... *     skmem:(r0,rb1048576,...,d0)`。原 `RAW_RE` 只识别`p_raw`下一行`skmem`，没有识别真实iproute2同行格式；先前合成测试恰好只造两行，所以旧repo-contract虽然绿，却是假验收缺陷。此次精确将regex容许同行或下一行，扩展两个synthetic tests包含真实格式，仍禁止多packet socket混合、保留缺失和drop counter回退fail-closed。该修复**不改变已上传的3150个null**，原始ss文本按隐私契约没有上传，不能事后捏造100ms drop计数。不因此无条件重跑新300s样本。

证据链接 [结构化结果](../evidence/e4-game4-100ms-probe-run37865738583-failure-analysis.json)。**下一步**GitHub Foundation新合成测试通过后再判断是否有足够证据走更窄的receiver/kernel drop因果测试；Game4其他protector/Normal1/Game2、大UDP/TCP-only和E7约80秒下行 OPEN，CPU收益UNPROVEN，E6/P6/physical NOT_RUN。
