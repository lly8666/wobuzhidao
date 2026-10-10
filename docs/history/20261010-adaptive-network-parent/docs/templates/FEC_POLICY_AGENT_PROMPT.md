继续 lly8666/wobuzhidao 的FEC-off与FEC20:20低损成本/恢复实验。先读AGENTS、PROJECT_CHARTER、STATUS、docs/REALPATH_TEST_FIXTURE_GUIDE.md、docs/FEC_POLICY_EXPERIMENT.md，再核远端最新HEAD。本文不依赖聊天史，执行细节以这两份方案和当前STATUS为准。

用户最新明确授权本项在一个Actions run、一个测量job中顺序测试不同业务/FEC；这是窄例外，不要被历史“一run一条”拦住，也不能扩成同时多负载/跨VM矩阵。其它正式优化/交付资格仍一run一条。自己建立experiment/fec-policy-sequential-20261009从最新优化分支开发，不改canonical主线、不操作实机、不覆盖其它agent。

产品冻结同一SOURCE，交接时为a2db258b436a41fdee98c6c53abec9bab6ce600f（32ms partial FEC，oneparse和clear优化已在）；核core/race后两端只构建一次，全部段同binary hash。保留认证/隔离/完整性/generation、同Seq同密文、有界shadow/fresh不等ACK、独立业务无HOL。不要改repair强度/缓存/默认FEC，不开发自动切换。

复用上个agent的五netns真实业务夹具：scripts/strict_weaknet_sample.sh、prepare_large_mtu_harness.py、large_mtu_mixed_business.py、efficiency_http_https.py、large_mtu_loss_stage.py、check_large_mtu_mixed.py、strict_resource_sampler.py和efficiency_cost_ledger.py；已有真实TPROXY/FakeTCP/TUN路径、TCP hash/HTTPS证书、UDP各尺寸和独立probe、seeded tc、socket/drop/CPU/RSS、pcap清理。不是只跑LINK/FEC unit，不恢复DTLS。

注意当前正式入口并不直接支持本实验：固定300s/300ms、loss choices不含1、旧workflow限0/5205和FEC20:20、ledger CPU/PPS分母硬编码300。先在独立实验入口做最小adapter，120s/drain3、15/300ms、0/1/5%、off/20:20与批次manifest/串行guard全部真生效。原单样本guard/默认/严格门不放宽；新增参数可选且原默认不变。新workflow固定建议next-fec-policy-sequential.yml，仅实验分支的fec-policy-batch.json配置push触发；不要假定新feature分支workflow_dispatch可用。check_performance_workflow_policy.py给此精确入口增加独立串行校验，保留其它入口规则，不跳过旧政策门。sudo env、两端CLI、实际handshake、MTU/FEC、manifest和分析分母一致。off不强求send_multi，不为测试攒包。所有build/unit/race/业务只在Actions，本地只编辑读Git文档。

先批次A：同一个runner一个job，15ms单向；0%和1%双向固定loss；udp/tcp/mixed；每种off/20:20一对，共12段，每段120s+3s，各方向逻辑10Mbps。配对同seed/速率/尺寸，交替off→on/on→off，off/on配置是变量。每段独立目录/receipt、fresh进程和netns/会话，重置计数/RTO/FEC/shadow/信用；完整stop/wait/owned清理检查后下一段，无并行残留。批次预计30..50分钟，90分钟超时。业务FAIL保留可继续后续；无法隔离/完整性失败停批，其余NOT_RUN。

普通业务沿用96..4068B，TCP长/短和HTTP(S)，mixed总预算10M不可双倍；120s实际流数单列，不虚填304。profile/timing和startup padding都OFF，额外BPF/100ms重型诊断不要启用。outer1400、record自动、实际TUN/MTU/MSS/缓冲每段读取；FEC-off派生MTU可能变化，解释这是完整配置对比。

逐段报告计划/成功注入/交付、UDP按尺寸missing、TCP hash/背压、HTTP(S)、全部probe返回/超时/期限交付与returned-only p99；CPU-s/交付GiB和注入GiB、RSS、实际线上IP字节/PPS放大、socket/接口drop、CPU型号/配额/PSI/steal/softirq和助手CPU。同runner仍可能时间漂移；不同CPU跨run不直接相减。缺计数写NOT_COLLECTED，配额不可见写UNKNOWN。

不要把20:20的有损阈值套off，不要求有损UDP全部恢复，也不靠丢业务宣传省CPU。0%合法业务零损坏/无故丢失仍硬门。区分本机额外drop与人工netem损伤、业务迟到与跨业务HOL。已有TCP-like是有限SACK/RACK修复，有预算/备份过期/稀疏tail限制；不能承诺off可靠TCP。没有record映射证据不能声称每个包由快速修复救回。

先提交adapter及功能Actions，验证单段/批次序列、FEC与loss/delay实生效、动态时间分母、失败/清理和hash，再跑批次A。有效后按方案B补300ms/0、1%，C视必要补两RTT/5%；20:4只补关键折中。首批是筛查，收益结论需另外两批关键mixed配对并反转顺序；不要盲目扩矩阵或抽好VM。

每次原子提交新详细devlog+更新STATUS.active_work.fec_policy_comparison，保留原E1/E7和80秒中断等历史失败。现有实验为PLANNED_NOT_RUN，不能继承旧PASS；不给新源码冒充物理PASS。保留每段summary/ledger/manifest/hash和有界原始输入摘要、aggregate只读，清理owned大pcap，不上传凭据/密钥/正文。

持续做到首批真实结果、成本/恢复解释与适用范围明确。最后提供run/精确SOURCE/helper/每段数字表、失败/限制和下一项，并在GitHub既有文档留痕。仅测试夹具错误可修后按新helper重测，不篡改旧失败；如需改产品策略，先把证据和具体建议留痕，不擅自升级默认或重构。
