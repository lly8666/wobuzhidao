# B短TCP探针发送结构修复＋C 5M TCP/5M UDP 混合业务候选（仅助手资格）

固定产品SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`，前助手独立分支HEAD `a729d030f8ad9a301ec8cdd01b0874e49381d7aa`。原始B0 https://github.com/lly8666/wobuzhidao/actions/runs/37732512345 **FAIL**：实际10Mbps目标两方向只发/收5.63689472Mbps、56.37%注入，有Socket背压与真实TCP重传；每方向三流hash一致，短事务250/300，50个MISSED_SCHEDULE约每6s重复，因为串行短TCP事务的三次握手+请求往返RTT约1.2秒而计划周期1秒。不得把50次标为网络丢包、被大UDP阻塞，也不能把低注入说成容量环境PASS。

在候选助手 `tools/longmix_tcp_business.py` 只改短连接生成器：每秒按绝对monotonic启动**独立线程/真实TCP socket**；有界最多8个在途、超过100ms起始延迟明确计MISSED_SCHEDULE、超过8上限明确计MISSED_CONCURRENCY_BUDGET，绝不等待前一连接往返后才能安排下一连接。固定同一seed/96B短事务，300s＋10s drain、每向96B/s预留，不把短包添加到10/5Mbps业务目标外。加入Actions专用真实loopback TCP fixture：目标刻意服务等待1.25s（超过1s发送周期），三条计划请求仍应全部及时启动和返回；此测试仅验证助手，不冒充产品性能。

新增C真实双业务助手：`tools/longmix_mixed_business.py` 只是一个**工作负载**的子进程launcher，每侧独立TCP业务5Mbps（三长TCP+短事务）与UDP业务5Mbps（96/512/1372/8972/8973/65507B，byte比例10/10/10/15/25/30）。UDP96B独立探针10Hz算UDP预算，ACK反向预留4096B/s，主UDP负载619944B/s；TCP short 96B/s也算在TCP 5Mbps内。遇TCP backpressure不将未用TCP配额补给UDP、源保证同seed/300s/只一对正式product endpoints。C利用真实socket + Linux隔离netns/客户端TPROXY/产品AF_PACKET underlay/服务端共享TUN/目标socket，同机mono时间；一个runner只跑一条C样本，**不**执行另一套产品端点。单一脚本转换和无损/TCP+UDP流完整性、双向netem实际丢包、独立探针+短TCP缺失分列的检查器 `tools/check_longmix_mixed_compact.py`，保留窗口与返回大包ACK数值；仍须helper preflight先成功才能跑C。

同时补A/ C UDP大包8972的ACK RTT账本（原只记8973/65507），有界ACK账本cap32768；保持A已跑原始F A0证据与helper SHA不可重新赋PASS。固定A ACK预留8192B/s，C为4096B/s，在`longmix_profile`同步配额及单元验证。补C相关unit、脚本生成、静态sample guard/workflow互斥的Actions预检。此提交不是性能测量、不能称B/C PASS、无产品修改/物理操作，freeze ref不变。独立no-truncation product `2f7bb59e...`功能Actions已PASS，但未改变b4下Linux合法大UDP无法全链路交付。原M03/1554 80s S2C中断与65507 1.225s迟到独立OPEN。
