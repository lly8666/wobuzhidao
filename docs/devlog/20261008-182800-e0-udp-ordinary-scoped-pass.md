# 2026-10-08 E0 首条正式UDP scoped PASS 与普通基线拒绝

起点目标分支精确 helper HEAD `863e4e4552696cc11e3bcf2b2fd5438f2f9cb28d`，产品 SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733`；该轮仅独立 Actions 性能 [run 37755799764](https://github.com/lly8666/wobuzhidao/actions/runs/37755799764)，job 113239907700，artifact 11539464746，attempt1，`sample-claim`严格单案例；没有物理机与主线操作。完整原摘要在 Actions artifact，原分析器判 `PASS_SCOPED_ACTIONS, issues=[]`，Action success；不能修改原判定。

## 真实业务测量
正式client OpenWrt TPROXY入口、server shared real TUN wbdg0、IPv4 真实socket 双向主动注入、外层加密FakeTCP与两个方向root netem，`--mtu 1400`、正式自动record cap 0、20:20、padding off、默认100ms tick（源runtimeentry代码）。服务端TUN**实读1273**；inner biz/client/target veth9000是试验链路容纳较大IP包，不能当产品派生TUN值。Normal1，UDP ordinary 96/256/512/1000/1372/4068B，seed1801，300s，300ms单向，零netem loss，双向按总业务字节约10.000Mbps已发、约9.996Mbps交付；各普通档大小包双方均0 missing/0 corrupt，send errors0；小probe C2S 1500/1500、p99=602.136ms，S2C 1495/1495、p99=602.108ms，missing0，连续业务10ms未出现主动期零窗。高大UDP8972/8973/65507本条**完全没有注入**，旧12条原FAIL仍OPEN。

runner Linux Azure ubuntu-24.04, 4 vCPU Xeon Platinum 8370C 2.8GHz，配额未证实为有限，host steal最大0，CPU PSI some avg10最大32.97，host busy最大32.02%，packet-socket/socket/interface drop0。本条client进程CPU118.26s/server118.02s，峰值RSS约32.73/33.21MiB；它们是**诊断开启时**的数据，不能与用户实机b4/其它runner比较为CPU收益。已观察raw send_multi>0、FEC partial和修复/ACK计数；定向账本见本轮JSON，非off正式成本。同期Foundation [run 37755799645](https://github.com/lly8666/wobuzhidao/actions/runs/37755799645) PASS含Linux/Windows Go基础、shared TUN和OpenWrt TPROXY模块；它也不替代性能。

## 首次原始PASS与E0有效性不等价
审到两个原助手沿用偏差：(1) `tools/large_mtu_mixed_business.py` 实际 `DRAIN_S=15`、manifest `drain_s=15`，E0/E6规范要3s，15s尾收不能冒充3s；(2) strict脚本始终传 `--diagnostic-jsonl`，当前server源码 `cmd/wbd-server/main_linux.go` 令 `ObserveTiming=diagnosticJSONL非空`，原始server diag `lanes[0].lane.TimingEnabled=true` 且累计1239803次inbound timing，client false。显式 CPU_PROFILE=0 不能证明server真正无逐记录计时。故这条保留 **ACTIONS PASS_SCOPED**，但正式普通 profile-off baseline **NOT_QUALIFIED**。未声称CPU降低、延迟改善或容量上限；PSI竞争仍需分层，0 socket drop也不等于无host争用。

第三个报告问题：旧 `large_mtu_resource_report._diag` 查顶层 `owner/raw_io/lanes`，实际JSON在 `product/tunnel` 等下，summary diagnostics.first_state/last_state 空；数值原jsonl仍在artifact，不能用空字段写0。只读抽样报告malloc/总alloc/GC、收发batch与FEC及repair均作为诊断探索，不用于off成本评价。新helper必须改为正确解析并在单独诊断Action收集，不改变失败事实。

下一个原子任务：将业务生成器 drain 调到3s、manifest同步；**普通off不启用会打开server逐记录计时的diagnostic-jsonl**，以proc/cgroup/links侧车采样off CPU/PPS/drop；单独profile-on run取Go alloc/batch/repair细账，校验on/off本身差异不能当收益。重新以自身精确helper SHA跑一条Normal10 UDP ordinary lossless seed1801（受控纠偏再资格，不是把旧12重发），然后独立TCP/mixed/Game4和大UDP边界。无需E1代码前，不动FEC/4096/MTU。80秒下行OPEN留E7，物理 NOT_RUN。证据详见 [run JSON](../evidence/performance-efficiency-e0-run37755799764.json)。
