# E3架构候选首条真实Game4无损profile-OFF独立样本（2026-10-08）

工作目标仅`next/performance-efficiency-20261008`，上一架构SOURCE `19e5b19245bed8691bbb5e7053ad27a0268c893d` 已添加Game2/4通过认证后共享PacketID去重并发测试、每Game隧道单个`boundedTickWork`异步TCP周期发射，0后台待办队列、生命周期close join、Normal维持同步。技术决议详见[GAME_MODE_INGRESS_ARCHITECTURE_20261008](../GAME_MODE_INGRESS_ARCHITECTURE_20261008.md)。原版本Game4仍真实FAIL，[profile OFF run37797087655](https://github.com/lly8666/wobuzhidao/actions/runs/37797087655) C2S UDP1906缺、probe23/31缺；[profile ON run37788802499](https://github.com/lly8666/wobuzhidao/actions/runs/37788802499) TCP tick emit18.363s、raw ready队列317739外层丢包，不能以新源码代码注释替代实际修复。

新SOURCE `19e5b19245bed8691bbb5e7053ad27a0268c893d` 先通过[foundation37800870550](https://github.com/lly8666/wobuzhidao/actions/runs/37800870550) **全部实际jobs SUCCESS**：Linux/Windows unit/Go build、Linux race/fuzz、Windows Npcap hosted/Linux arm64、OpenWrt TPROXY+Linux TUN nft/iptables privileged/kernel fallback；[lifecycle37800870646](https://github.com/lly8666/wobuzhidao/actions/runs/37800870646) core/lifecycle+race SUCCESS。它证明这批单测/并发测试通过，但对Game4负载后的实际缺失/p99仍必须测量，不能就此宣布E3成功。

本轮helper新提交为单个独立性能case更新`.github/efficiency-e0-sample.json`：Game4、mixed真TCP/UDP/HTTPS、每方向逻辑**3Mbps而非4×3**、loss=0%、one-way300ms、300s业务+3s real drain、seed1825、outer1400、自动record cap=0、FEC20:20、pad0、diagnostic模式**OFF**；专用workflow的`PRODUCT_SOURCE`改为精确`19e5b19245bed8691bbb5e7053ad27a0268c893d`，新helper取**本轮提交SHA**，Actions内`check_repository.py`提前拒绝无STATUS/新devlog的非法提交。只有1个job/1个业务样本，没有同runAB/matrix或本地物理测试。初始测量状态`NOT_RUN`直至真正出现run/job/artifact；GitHub push事件登记可能延迟，旧临时NO_RUN不可填作永久没有跑。

读取原分析器`PASS_SCOPED_ACTIONS/FAIL`、每方向普通UDP全部大小完整性、独立小探针missing与returned-only p99、4长TCP+300短流、HTTP/HTTPS cert+body、真实TUN MTU、netem/qdisc/socket/if丢失/CPU per delivered GiB和PSI。Profile-OFF无ready queue/tick JSONL，故不能用这条宣称`ready_overflow=0`或worker scheduled数；若想看新增async计数，须另起诊断ON单样本，但不能对比OFF CPU。任何Game4 lossless未合格则改动`E3_BLOCKED`，甚至新包丢更多需要回滚或进一步分lane控制。旧500ms RTO/4096资源/自动MTU/FEC线协议不变。

下一保护仍需Normal1 lossless、**真正分阶段5%→20%→5% 5205**、Game2/4 0loss及Game4 5205各独立Actions；Stage B才可能在总预算恒定下建立per-lane ingress workers，不能再堆第二个4096“锅”。CPU收益要求最少3次同宿主分层的OFF有效交付样本，未归因的一次+/-百分比都不可算成绩。原E7约80秒下行故障OPEN，E6/P6/physical NOT_RUN；没有修改main。

机器证据：[本轮唯一Game4样本配置](../evidence/performance-efficiency-e3-game4-bounded-tick-single-candidate-20261008.json)。
