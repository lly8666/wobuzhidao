# E0 Normal mixed原始PASS的超额业务字节审计，修正限速资格（2026-10-08）

源: 产品 `bf11fbfbe64d518e7ba189d51bfb4512df4df733`，既有混合helper `ab9fa01253581063817d6b4d443cf86a3c8017fc`；独立 [run 37763345793](https://github.com/lly8666/wobuzhidao/actions/runs/37763345793)，job 113264879151，artifact 11543666374，attempt1，**Actions及原分析器原始 `PASS_SCOPED_ACTIONS`，issues=[]**，绝不追溯删除或改写。单job/单300秒Normal mixed 5M TCP+5M UDP每方向、seed1803、300ms单向、loss0、FEC20:20/padding-off、3s drain、client/server诊断都未开启，真实服务端TUN1273、auto record cap0；真实客户端TPROXY/加密raw/server共享TUN/目标业务socket。

## 业务完整性和局部通过
C2S/S2C两向：345673个普通UDP datagram，所有尺寸missing0，四持续TCP+300短framed流发送304条与接收304条hash/长度一致；20次HTTP(S)请求全返回、10次真实HTTPS证书和正文验证，missing0、返回p99=1837.955ms。小UDP probe C2S1500/1500 p99=631.889ms，S2C1495/1495 p99=627.008ms；active gap最长170/50ms（仅检测指标，非正式跨流HOL证明），socket/interface drop0。CPU client173.75s/server172.79s，AMD EPYC7763 4vCPU，RSS42.59/43.12MiB，steal0，host busy max57.02%、CPU PSI some avg10 max34.69；配额未充分获取，不能据此声明容量尚有余量或CPU优化收益。

## 关键资格限制：输入上界未按总业务字节封顶
原摘要C2S总已发bulk+probe **10.078025Mbps**，S2C **10.078013Mbps**（加上独立HTTP短业务只会更高）；目标严格为**双向每向逻辑业务总10Mbps**。因此**保留原始scope PASS，同时E0严格业务预算资格NOT_QUALIFIED**，不得把多发的约0.078Mbps视为交付优势/改善。旧分析器只拦低于0.99倍的输入，没有验证上限；长TCP最后一个1MiB块在预算临界点整体发出最多可能超限，每方向4条流的理论尾部超额上界接近0.112Mbps，与本次量级相符，但这只是合理的发生机制而非已经得到包级因果。没有通过放宽用户速率、修改正式产品或丢弃业务来修。

## 本次新增助手补丁（新SHA需自己资格）
1. `tools/large_mtu_mixed_business.py`：TCP按 `rate*300s−已发字节` 决定末尾stream write长度，并按实际长度CRC/sha验收；UDP只有完整datagram能放进剩余额度才发，不能把UDP单包截成两条应用消息。绝不插入排队/凑批/忙轮询，不改正式产品。HTTP/HTTPS短业务继续预留0.02Mbps总配额。
2. `tools/efficiency_http_https.py` 记录实际HTTP响应整段应用字节长度（含header），不仅payload；分析器将两方向单独短请求/response字节加到bulk+probe，形成`total_logical_sent_mbps_including_http`，要求不高于10Mbps+单包8KiB测量余量（余量不是合法故意超发配额）；原goodput和故障原始字段均保留。
3. 新 `tools/efficiency_cost_ledger.py`为只读分析/不发流量；在同一测量Action后计算合计产品CPU-s/有效交付GiB、外层qdisc PPS/非NIC PPS、主机型号/CPU配额/PSI/steal/drop/RSS；没开诊断则alloc/batch/repair写NOT_COLLECTED，决不伪造0。若后续单独显式profile-on则记录Go内存/GC与raw_io/repair数值，但不与off当A/B优化收益。工作流唯一测量job不变，分析步骤在原样本后，加入原子helper哈希与compact artifact，任何脚本/summary失败仍fail closed。

此次改动的唯一下一性能run仍是Normal mixed 5+5Mbps、seed1803、lossless300s/3s drain、profile-off，不在同run做校准/A-B或第二条。只有新helper确实总字节≤10Mbps、完整性/输入/探针/CPU/queue有效时才将E0 mixed基线由NOT_QUALIFIED改为PASS。独立Game4、jumbo 8936边界、诊断on账本、E1-E6及P6仍未完成。80秒下行原FAIL留待E7，物理不碰且不写PHYSICAL_PASS。证据见 [JSON](../evidence/performance-efficiency-e0-mixed-37763345793-overbudget.json)。
