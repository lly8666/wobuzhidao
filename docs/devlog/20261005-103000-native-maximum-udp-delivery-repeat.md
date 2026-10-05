# 最大 UDP 边界：API正确，回程1包缺失待重复

## 本轮目标和阶段

产品仍SOURCE660b370，同源配套部署不变，文档起点1dd9184。M03 seed1365完整300.0926076s，正重复seed1366；剩余idle/DNS/IP/配置继续推进。未改产品恢复策略。

## 修改与原因

只读外部分析器坚持合法包全收齐门，不因只缺1包而降低：65507 UDP/DF=false发171、target收到171且echo_send_errors0，客户端精确收到170，Timeout1/Late0。因此缺失位于target收到请求之后的回程，尚未定位到产品、Windows IP重组或真实WAN；这一条严格delivery记FAIL，不把helper正常exit0当PASS。

8972/DF=false172/172、DF=true171/171，8973/DF=false171/171；穿插96B1368/1368。8973和65507/DF=true每档171次均MessageSize，65508/两DF各171次均MessageSize。BadPayload/OtherSendError/InvalidResponses/ReceiveSocketErrors0；客户端一直存活、退出0，owned网络状态/NRPT/firewall0。API与MTU拒绝/后续小包可用边界正确，但最大合法分片UDP全收齐未通过。

## Actions和复用证据

原有MTU/helpers Actions资格未改变。使用现成next-strict-weaknet与next-config-effective，固定tag qualification/660b370-20261005，一次性18独立严格lossless/5205/5305×Normal/Game×seed1381/1382/1383，以及70独立功能配置，共88runs已dispatch。每性能run仅一条，不改变门；实时回执只读核对head_sha/identity/attempt/duplicate，每种配置普通UDP/DNS/TCP/HTTPS和实际record/FEC/lane效果由Actions判定，不能CI绿替代实际回执。

## 问题、排查与风险

该轮末尾client RxPath recovered_sources579、reconstruction228，ExpiredMissingSources0/pressure_retirements0/record/path errors0；仅末尾计数不能定位缺失哪个IP fragment。server raw socket末尾drop0，真实WAN损伤未刻意注入也不代表丢包恒0。重复前不改FEC或扩大缓存；若第二次同样缺失，下一步缩小到这一个回程IP重组诊断，不能盲改协议。Windows/Linux全局IP计数mid-window另存，只作观察，不能归属于WBD。

## 下一项原子任务

收seed1366逐尺寸/DF回执，保持seed1365 FAIL。随后S18/S19/S20按无助手probe/DNS的真实静默/唤醒与持续下行验收；收88独立Actions及两1800s。当前历史有效完整17样本、8唯一case，35项NOT_RUN；不是17个PASS或当前源码全矩阵通过。原始pcap按有界窗口分析删除。
