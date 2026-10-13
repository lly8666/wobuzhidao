# 2026-10-13 正式混合TCP+UDP独立10ms首达无HOL Action 登记（结果尚未产生）

唯一性能SOURCE `014177a20e48bccf7d8bab8090b774f92dfb9847`，[Action38326041291](https://github.com/lly8666/wobuzhidao/actions/runs/38326041291) job115064818885。本次不启动另一个性能任务；其GitHub Actions前置repo/performance policy/sample claim、Go N2 unit/race、三个Python定向单元（派生真实五netns配置、独立10ms UDP packet+CRC坏包拒绝、10ms空桶检测）已经PASS，Linux依赖已通过，正式client/server二进制build正在进行。**尚无120秒产品负载及首达/CPU/queue/额外资源结论，不能写成无HOL通过或N2 PASS。**

同一绝对源码以正式Go protected client/server、Linux 五netns原本TPROXY/netlink/raw路由、Normal auto initialR20/floor4/max20 ChaCha MTU1400，双向各10Mbps（真实混合持续bulk UDP、4条长TCP、每秒独立短TCP、单独100Hz 96B UDP），双向300ms单程，独立tc随机netem1% each-way（内核RNG UNSEEDED），一次120秒活跃业务。对每个业务方向独立记录短TCP首发和首收时间，fast UDP精确sequence、source monotonic_send/target receive时间的单向首次收达p99、每10ms接收桶、source调度晚到/坏包/重复/连接失败；在有界范围核真正no HOL，不能将UDP聚合持续交付或TCP字节累计当作所有flow无HOL。所有capture原始pcap只核hash/kernel drop，上传前删除，凭据/业务原文不上Github artifact。原case和screen参数均测前注册，任何证据缺失/容量/实际执行FAIL原样保留。

用户要求的Windows GUI配置与传输选项已经原生Windows WinForms273项Actions验收；N3 AES128/256分别100秒真实无损client/server功能限定通过。仍未解决N4 Win原生Direct TCP/UDP透明转发系统栈和真实Wintun，以及N6三目标包；不能用Go N4逻辑测试的绿色宣称Win原生数据通。C04/S2C恢复效率78.3%及C05/S2C10%效率70%原始FAIL、C10 GE one-way首达p99 1.833s/1.9475s与历史2.73s探针/80秒下行仍OPEN。physical NOT_RUN、不得自动部署/移动qualification ref/合并主线。
