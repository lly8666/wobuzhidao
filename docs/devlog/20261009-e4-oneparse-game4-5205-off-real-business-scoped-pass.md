# E4 单遍收包解析：真实 Game4 5205 混合业务保护通过，但CPU收益未证实（2026-10-09）

唯一工作分支 next/performance-efficiency-20261008。此提交父HEAD 9bd2b69f7d0ad7609aa5fa9864e26f07b79a9bae；测量源码 SOURCE aa931d5822a5378c48bf0763216f0446a5af8683，唯一帮助脚本HEAD 9bd2b69f7d0ad7609aa5fa9864e26f07b79a9bae。不能使用旧源码 ba8ed1 的PASS代替新源码，测试的Git worktree已按`PRODUCT_SOURCE`锁定候选。前置 [Foundation37886445830](https://github.com/lly8666/wobuzhidao/actions/runs/37886445830)和同一配置提交 [Foundation37886694668](https://github.com/lly8666/wobuzhidao/actions/runs/37886694668)全绿，真实单测量job [Game4 Actions37886694720](https://github.com/lly8666/wobuzhidao/actions/runs/37886694720) **SUCCESS**。原版`summary.json` **`PASS_SCOPED_ACTIONS`, `issues=[]`**，`efficiency-ledger.json`无issue；artifact11597305253，四份原始JSON SHA256见 [证据](../evidence/e4-linux-oneparse-real-game4-5205-off-one-protection-run37886694720.json)。这不是基于日志猜测。

固定唯一样本：真正biz socket→client TPROXY→4-lane TLS-like/FakeTCP→router netem→server shared TUN→target socket，mixed UDP/TCP/HTTP/HTTPS，300秒 profileOFF+3秒drain，seed1840，每向3Mbps，FEC20:20，outer MTU1400、实际TUN MTU1273，默认tick100ms。75s 5%、150s 20%、75s 5%三段实际双向丢包比例约5.0/20.1/5.0%。

核心业务硬门：双向各103,062 UDP（pre25,792 / stress51,503 / post25,767）全部交付，缺失、错误和>1秒业务UDP均为0；游戏探针上行1500/1500、下行1495/1495、无丢包，往返p99上行620.32278ms、下行621.112374ms；HTTP/HTTPS请求20/20，HTTPS证书验证10，错误0；活跃时最大10ms粒度空档c2s70ms/s2c80ms（非完整HOL因果证明）。各端AF_PACKET skmem.d=0、socket额外drop=0、接口额外drop=0；original `strict_resource.errors=[]`，所有检查由原不可跳过的summary与ledger完成。

CPU/资源只陈述事实：本条runner为AMD EPYC9V74、4vCPU、cgroup CPU quota未知，最大CPU PSI some avg10=34.77%、host busy72.4%；客户端131.34+服务端132.24 = **263.58 CPU秒**。旧源码 ba8ed1在另一GitHub-hosted VM上的同类Game4 5205 [37853468730](https://github.com/lly8666/wobuzhidao/actions/runs/37853468730)为**265.02 CPU秒**、9V74 CPU型号、最大PSI32.22%。两条虽是同型号4vCPU，仍非同一虚拟机且无法验相同quota/实际CPU干扰，仅差1.44CPU秒（≈0.54%），**不能**推导候选提高CPU效率的因果收益，更不能以此做升级宣发。

结论严格分层：只把 `aa931d` 的**这条Game4 true5205 profileOFF**列为PASS_SCOPED_ACTIONS，不能等同Game4 0loss、Normal/Game2、容量/多MTU或物理通过，更不等同收包丢包根因解决。旧9V45真正无人工丢包Game4原始OFF [37857040784](https://github.com/lly8666/wobuzhidao/actions/runs/37857040784) socket client33/server86与同型号ON [37871243581](https://github.com/lly8666/wobuzhidao/actions/runs/37871243581) client42 **全部保持正式FAIL**。

用户要求不要钻牛角尖已落实：不追加BPF/cgroup遥测框架、不盲重跑弱网/抽跑快宿主、单轮只派发本条唯一300秒性能案例；不修改socket buffers、游戏inbox/shard队列、协议/FEC/MTU/main/规范主线或物理设备。当前CPU gain UNPROVEN，E4 lossless drop root OPEN，E6/P6/physical NOT_RUN，E7约80s S2C停顿 OPEN_DEFERRED。下一工作明确选择有用户价值的一条业务保护或小产品热点检查，先过硬门而不是堆测量。
