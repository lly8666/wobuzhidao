# 20261007-223000 大包迟到：修复前确定性分层诊断

## 当前SOURCE与分线

本轮算法诊断分支 `qualification/large-datagram-rto-20261007` 从 raw-buffer 候选 SOURCE `3d3e24f1271ddf960aa57035a6217476f1826bbc` 开出。raw socket 配置仍是独立前一提交；本提交不改任何生产算法、FEC参数、4096库存、业务速率或恢复期限。

raw候选的 exact-source Foundation run 37632989732 与 Linux server run 37632993604 已成功；六个独立普通性能 run 也全部 workflow success，默认请求524288在Actions实际读回1048576、limited=false、socket/link drop0。该事实只证明默认路径可运行；同SOURCE `raw-recv-buffer=0` 对照还没跑，不能把“1MiB”直接记成物理性能修复。ARM已知rmem_max=212992，仍需物理读回limited/effective。

## r12旧尾部仍复现，排除raw buffer为统一根因

同SOURCE `3d3e24f...` 的 r12 Normal/5305/seed1508 run 37633598715 在 raw buffer 实效1MiB且socket/link drop0时，stress probe 60发56收、p99=2167.779ms；业务最终goodput约9.174/9.181Mbps。它与旧 run37602563270 的约2168ms尾部同量级，因此 raw receive buffer 没有消掉该类尾部。M03/1539物理两次约1.08/1.13s late本来也是server rawdrop0；两问题继续分开。

## 新增的修复前确定性诊断

1. `TestSparseLowRTTLossWaitsInitialRTOPreFix`：建立50ms RTT样本后，当前 `observeRTTLocked` 得到的估计仍由 `clampRTOLocked` 下限到 `InitialRTO=1s`。随后发送一个没有任何后续payload/SACK证据的稀疏record，999ms不repair、1000ms才同Seq同密文字节repair。该测试先固定当前慢路径，不把相关性直接写成已修根因。
2. `TestCrossInnerMTUTinyTailFECRecoveryDoesNotHOLSmallDatagram`：把MTU9000的OS结果建模为8996B near-MTU IP fragment + 25B tiny tail，走真实LINK/FEC 20:20 size-class。故意丢tail systematic时，随后96B systematic仍立即first-deliver；100ms owner-tick等价flush后的同block parity可恢复tiny tail。证明缺tail不会制造lane-wide HOL，也明确FEC能在parity存在时先于outer RTO恢复。
3. `tools/test_inner_mtu9000_fragment_kernel.py`：privileged Actions真实Linux TUN，MTU=9000，24轮持续覆盖UDP8972/8973/65507/非法65508与DF边界，并在大包间穿插96B。只记录每数据报send-call、TUN首片/完整片时刻、IP长度/offset/片数/errno，不保存正文。8973/DF=false必须实际得到[8996,25]两片；65507必须8片；8973/65507 DF=true与65508必须EMSGSIZE。

上述三个证据层故意分开：真实内核只证明OS分片边界，LINK/FEC测试只证明产品分片/恢复/no-HOL，outer transport测试只证明低RTT稀疏loss仍被1s floor卡住。只有它们和修复后同场景改善一起成立，才把1s floor写成大包迟到的可修因果链。

## 下一步

本提交测试尚NOT_RUN。先用Actions验证修复前诊断；若通过，再做窄修复：保留startup初始RTO=1s与3s绝对horizon，但允许已有RTT样本后的base RTO使用独立保守下限，而不是永远以InitialRTO做floor。随后同一候选必须跑core/race、Linux生命周期网络门、上述三项回归，以及每run单一样本的Normal20/Game4 lossless+weaknet和r12/5305旧尾部。独立小包、repair开销、重复发送同Seq同密文字节、fresh不受4096门约束都保持硬门。
