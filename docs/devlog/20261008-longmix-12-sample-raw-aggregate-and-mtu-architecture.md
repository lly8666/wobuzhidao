# Longmix首轮12条全部原始完成：无损大UDP硬缺口、TCP有效注入不足；是否该以小包为核心

## 精确资格

冻结产品 SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567` 不变，正式主分支文档82c3c614未覆盖；12条独立workflow_dispatch、每run仅单config/seed/场景、一对正式端点、Linux netns真实TCP/UDP socket→OpenWrt TPROXY→AF_PACKET FakeTCP→Linux共享TUN→真实目标socket；有效业务300s、独立drain10s、单向300ms/约600ms基础RTT、外层1400、内层服务TUN9000、Normal1 FEC20:20 padding off。没有Linux客户端TUN，不能冒充Windows Wintun或物理ARM结果。最终外部Actions workflow失败与本次只读收集的数值摘要全部原始`FAIL`；这不等于12个互不相关的新产品缺陷，B输入能力门失败与A/C代码尺寸边界需要独立归因。

|业务|Loss|Action|C2S / S2C有效Mbps|独立探针returned/offered|返回p99 ms|netem C2S/S2C实丢%|
|---|---:|---|---:|---:|---:|---:|
|A|0%|[37731062203](https://github.com/lly8666/wobuzhidao/actions/runs/37731062203)|3.474 / 3.474|3000/3000|601.15|0.00/0.00|
|A|5%|[37734052485](https://github.com/lly8666/wobuzhidao/actions/runs/37734052485)|3.474 / 3.474|3000/3000|653.06|5.03/5.00|
|A|20%|[37734152474](https://github.com/lly8666/wobuzhidao/actions/runs/37734152474)|3.474 / 3.463|2988/3000|672.64|20.10/20.03|
|A|30%|[37734257494](https://github.com/lly8666/wobuzhidao/actions/runs/37734257494)|3.461 / 3.467|2986/3000|681.34|30.08/30.04|
|B|0%|[37733833093](https://github.com/lly8666/wobuzhidao/actions/runs/37733833093)|5.667 / 5.667|300/300|1228.40|0.00/0.00|
|B|5%|[37734359638](https://github.com/lly8666/wobuzhidao/actions/runs/37734359638)|5.548 / 5.577|300/300|1264.92|5.01/5.01|
|B|20%|[37734464340](https://github.com/lly8666/wobuzhidao/actions/runs/37734464340)|5.488 / 5.458|300/300|1390.11|20.01/19.98|
|B|30%|[37734569538](https://github.com/lly8666/wobuzhidao/actions/runs/37734569538)|5.249 / 5.309|300/300|1896.86|29.97/30.00|
|C|0%|[37733929493](https://github.com/lly8666/wobuzhidao/actions/runs/37733929493)|6.415 / 6.415|3000/3000|627.11|0.00/0.00|
|C|5%|[37734795819](https://github.com/lly8666/wobuzhidao/actions/runs/37734795819)|6.415 / 6.415|3000/3000|636.79|4.97/5.00|
|C|20%|[37734908160](https://github.com/lly8666/wobuzhidao/actions/runs/37734908160)|6.412 / 6.413|3000/3000|698.88|20.02/20.01|
|C|30%|[37735039340](https://github.com/lly8666/wobuzhidao/actions/runs/37735039340)|6.399 / 6.405|2989/3000|698.21|30.03/29.99|

注意：A和C p99为独立96B UDP往返，B p99为96B TCP短连接建连加请求往返，**不能横向用一个数字比较两种协议**。丢失/超时另列，不因只对返回样本求分位数而宣布低延迟已PASS。每样本完整source/helper SHA、artifact ID/sha256及数值在同一提交`docs/evidence/longmix-20261008.json -> first_round_complete.results`；一条好的子项不能抹掉整样本FAIL。

## 明确因果与限制

- A和C正常payload九档(A) / 六档(C)中，8972/8973/65507B三档双方向**所有样本0 valid-first**，包括无损；A仅小档实际~3.47/10Mbps，C UDP~1.47–1.49/5Mbps。A显式将这三档设置为65% UDP业务字节，C设置70% UDP字节（整个C TCP+UDP逻辑业务为35%），是刻意重载边界，**不是现实互联网大包比例估计**。源 b4 的OpenWrt UDP入口/平台flow `MaxPayload=9000-20-44=8936`，C S2C超大UDP有大量invalid header（原服务端UDP socket reads截断）。本地产品防静默截断候选2f7bb59 Actions37732745397**仅功能PASS**、不提供>8936合法UDP全链路交付，不把拒绝计数从合法业务丢失分母拿掉。
- B在0/5/20/30%每向TCP有效仅5.25–5.67/10Mbps，短TCP修正后在所有5/20/30和0重复各300/300返回，三条长连接逐流hash/bytes完整；仍大量TCP socket write timeout/背压和kernel retrans，**INPUT_VALIDITY FAIL**。不是网络质量越差反而有更高业务投入；没有足够最早CPU/队列/PSI/softirq容量饱和因果证据，不能写CAPACITY_LIMITED/PASS。低送入量不能被用作原生系统达标。
- A小UDP96探针0/5/20/30%回3000/3000、3000/3000、2988/3000、2986/3000；C为3000/3000、3000/3000、3000/3000、2989/3000。C独立TCP 3条长流+短事务和小UDP持续到达，未观察到整个样本被丢失的大UDP全局拖死，支持此Linux路径局部无跨业务HOL，但不能覆盖原生Wintun M03/1554 80s S2C失活和恢复后65507 1.225s迟到。A/20单独S2C最长无首次交付10ms窗1020ms仍需要源覆盖、排队与产品边界时间线，不草率宣布无HOL或CPU原因。
- `A/0`此前助手 INVALID 37729338574/37729716707、`B/0`旧 37732512345 FAIL (短TCP原250/300)、修复短连接后重复 37733833093 FAIL (300/300)全部保留于evidence原条目；资源/外层字节/FEC block长尾/系统overhead详见每条原artifact，尚未完整提取就标OPEN，不可用本简表冒充资源资格完结。

## 关于大包重要性的专业边界：不改变已冻结产品合同

互联网接入链路MTU通常近1500；现代TCP大`write`由MSS分段、QUIC遵循RFC9000避免IP分片；RFC8899 DPLPMTUD用于监测路径可用无分片大小及黑洞恢复，RFC8900建议上层协议避免依赖IP分片。历史CAIDA监测早期公网聚合链路一般1–2% IP分片（特定小时高很多）、APNIC研究DNS样本约0.1%分片，都不能据此声称2026整网UDP>9000B的精确百分比。标准：https://www.rfc-editor.org/rfc/rfc9000.html 、https://www.rfc-editor.org/rfc/rfc8899.html 、https://www.rfc-editor.org/rfc/rfc8900.html；测量：https://www.caida.org/catalog/media/2001_myths2002/ 、https://blog.apnic.net/2022/09/21/ip-fragmentation-and-the-dns-the-state-of-ip-fragmentation/。

建议优先默认常规数据报的首次交付/p99/资源与跨流无HOL、在外层维持≤1400的预算并结合实际PMTU信号探测有界变化；不要把单个UDP65507分解成几十个**目标应用**可见的独立UDP小包（语义错误）。TUN MTU下降能帮助TCP自行选择更小MSS，但DF=1时不能无声内层IP分片；对DF=0的合法UDP可由系统IP分片并透明恢复，仍须保证后续片/校验/账户隔离完整。弱网30% loss≠PMTU变小，不能以丢包率立即调大FEC或盲目缩小LINK shard；更小shard可增加PPS/重组失效机会，必须按fresh首达/资源/丢包粒度实际评价。没有用户确认不改变已冻结9000/1400、FEC20:20、4096、3s恢复期限或wire实现。

## 下一步

仅增加有解释价值的独立定位样本：A20 1020ms窗口数值时间线、B低注入时发送→TCP拥塞/MSS/ACK/外层队列的最早背压点，以及Windows原生Wintun真实大包适配/物理复验少量M03工况（物理由原聊天）。独立2f7bb59不是发布SOURCE，不能把功能PASS继承给b4或物理P7。修产品前分别提交测试助手与产品原因链，并同一commit维护STATUS/evidence/devlog。Windows/ARM打包仍NOT_RUN。
