# 20261007-230500 已建立RTT后的repair下限修复候选

## 修复前因果边界

首个MTU9000 fixture run `37638831165` 的FAIL继续保留，已定位为观察器把非目标TUN帧误判fatal。修正后的 SOURCE `be06814c3ba527fbdf1f713e3b37db024ad71618` 在 next-predelivery-tools run `37639166468` 的真实Linux TUN job通过：24轮UDP8972/DF=false均单包；UDP8973/IP9001 DF=false 24/24均为 `[8996,25]` 两个IPv4片；UDP65507 24/24均8片；96B穿插72/72单包；65508及DF超9000均为EMSGSIZE。这里只证明OS分片边界，不冒充产品性能。

修复前 SOURCE `a3ce2f552b04776dbcc2654b9fcef54c7cc3edb8` Foundation run `37638827134` 全绿，包含新增Go诊断和全仓race。LINK/FEC确定性测试使用真实20:20 size class，故意丢25B tiny-tail systematic时，独立96B systematic仍立即交付；100ms owner-tick等价partial parity可恢复tail，证明缺片只等待本数据报，不形成lane-wide HOL。outer transport诊断建立50ms干净RTT后，旧逻辑仍把RTO clamp到InitialRTO=1s；随后无后续payload/SACK的稀疏record在999ms不repair、1000ms才以同Seq/同密文字节repair。该机制与物理M03约1.08/1.13s量级吻合，但不能据此声称每次大包late或r12/5305多秒尾部都只有这一原因。

## 生产修复

只修改 `internal/runtimeowner` timer下限。InitialRTO仍是无可信RTT样本时的启动值，正式路径仍1s；新增内部MinimumRTO默认200ms，不是CLI/JSON。取得未重传的clean RTT后，现有SRTT+4*RTTVAR估计以MinimumRTO而非InitialRTO做下限。3s绝对repair horizon、超时backoff、repair credit、fast repair、4096 shadow语义、FEC、LINK、ACK决策均不变。重传仍直接复用pending record的原Seq和原payload/ciphertext，不重新Seal。

修复后确定性测试保留startup RTO=1s断言；50ms样本得到150ms estimator并由200ms floor限制，199ms无repair、200ms首次repair，字节/Seq完全相同。既有600ms RTT->1.8s行为无需改变，所以高RTT路径不会被强制成200ms。

## 待验

本提交尚未跑修复后Actions或性能，PHYSICAL NOT_RUN。200ms是否保留取决于同SOURCE core/race、MTU9000 fixture、Normal/Game默认观测off、r12/5305旧尾部的p99/probe coverage/repair/wire成本。若明显制造无益重传放大或收益不足则回退，不改验收门。
