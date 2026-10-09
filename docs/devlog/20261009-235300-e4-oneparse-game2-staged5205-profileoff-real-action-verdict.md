# E4 oneparse Game2 staged5/20/5% OFF: real Action verdict (2026-10-09)

唯一工作分支 `next/performance-efficiency-20261008`，父 HEAD `d87d6f9bfdbd533e268615374dfd4096a844f841`，本次只更正进度、追加结构化实测证据与开发日志，不改产品Go、内核收包缓冲、任何队列、工作流、主线、协议、物理机器，也没有再派发300秒任务。已核实 [Game2 Action37898379013](https://github.com/lly8666/wobuzhidao/actions/runs/37898379013) 全部SUCCESS， [Foundation37898378989](https://github.com/lly8666/wobuzhidao/actions/runs/37898378989) 全部SUCCESS。真实运行的候选SOURCE是 `aa931d5822a5378c48bf0763216f0446a5af8683`（one-parse），不是旧基线`ba8ed1`；artifact 11600674710 zip SHA256 `b32a1f4c3f09481298a0007b3fecadee4445a09900a1d3eb8d763b3379e5a3cc`。完整单个原始summary与ledger均 `PASS_SCOPED_ACTIONS`、issues `[]`，不以Actions绿色替代业务门。

Game2 `mode=game/lanes=2`（**configured=2**；profileOFF summary未独立记录两条物理socket上均有流量，不把配置硬说成实测active=2）；mix UDP/TCP/HTTP/HTTPS，seed1842，双向3Mbps；300秒+3秒drain，0—75秒5%、75—225秒20%、225—300秒5%双向 qdisc。实际丢包率c2s三段4.993% /20.003%/4.954%，s2c三段4.971%/20.081%/4.990%，而不是照抄输入配比。client TPROXY、server shared TUN 实际MTU1273，FEC20:20。

业务真实性：c2s UDP pre25792/25792、stress51503/51503、post25767/25767全部交付。s2c UDP pre25792/25792、stress**51502/51503（少一条）**、post25767/25767；唯一缺失是20%高损阶段大小**4068字节**的一个UDP应用报文。故原Analyzer按本场景允许容忍范围 `PASS_SCOPED_ACTIONS`，**不得写成所有业务0丢**。双向probe c2s1500/1500 p99 692.319ms，s2c1495/1495 p99 688.297ms，missing=0；双方TCP各304个flows收发哈希/长度无差异，HTTP10/HTTPS10正常且HTTPS10个证书验证。业务方向最长活跃发包但暂未收包间隔100ms（该指标只属diagnostic no proven HOL）。

内核资源门：整个运行client/server AF_PACKET `skmem.d =0`；所有biz/client/router/server/target网卡extra drop=0；strict_resource.errors=[]、300次资源抽样。这里说的是**此源、这台CPU、这次负载**下没有观测到socket drop，不能反证历史ba8ed1的真实socket丢包。服务器相对CPU-s112.77、客户端112.35，总225.12；宿主AMD EPYC7763四vCPU、cgroup quota未观察到、CPU PSI some avg10最高37.94%；不同宿主不能用225.12s跟老源9V74/9V45样本相减来谎称收益。

依旧保留历史旧SOURCE ba8ed1在9V45的Game4 0loss profileOFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) client skmem.d33/server86以及profileON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client42的正式原始FAIL。新的oneparse在Game4 lossless和Game2 lossless等已有**各一条限定通过**，不是跨宿主无条件胜利。下一个合理工作是从这次单个4068字节s2c应用missing开始，仅分析旧artifact及现有路径是否与4068大小、20%阶段关联；无因果就标罕见应用遗失/未归因，优先补low RTT sparse/E7约80秒downstream outage的真正用户保护，而不是重新开发BPF/VM校准系统、放大缓冲或抽runner绿样本。

原始结果不篡改：Game2 staging仍PASS_SCOPED_ACTIONS但有1个UDP missing；CPU gain UNPROVEN，physical NOT_RUN，E7 OPEN_DEFERRED，且**不授权新300s压测**。
