# 真实业务测试夹具：功能、入口与复用边界

## 2026-10-10 helper能力更新与SIMD新旧比较

下面c149564快照是历史，不代表本分支能力。当前继承helper提交543ac2cd2920e9f0fb59fdb38ee6aa3d9a65e56f，已有prepare_large_mtu_harness.py --fec-experiment、--duration-s 15/120/300及--delay-ms 15/50/100/150/300；业务/采样/分母支持对应串行实验。旧36工况run38021635895是FEC-off UDP有效观察，不是新SIMD资格。核每个adapter真实支持的loss/stage，不从可选参数推定5205已完成适配。

现有tools/fec_policy_batch.py固定SOURCE=a2db和experiment/fec-policy-sequential-20261009，只跑单SOURCE off/on，不能直接启动SIMD两SOURCE比较。新任务另建tools/fec_simd_ab.py与精确branch/config受限单job workflow，复用原拓扑/业务/helpers/资源与清理，逐legSOURCE与binary/hash/receipt。旧第4节“当前不能直接运行”仅描述旧c149快照；新能力以上述代码与最新STATUS为准。

最新用户明确允许本项同Actions串行ABBA两SOURCE。开发与测试路线、上下游配置/CPU/真实交付/p99、ARM native、120s screen/300s确认见[FEC_SIMD_OPTIMIZATION_PLAN](FEC_SIMD_OPTIMIZATION_PLAN.md)。300ms TCP-off收尾失败和Game4容量FAIL保留，不能用省CPU关闭它们。

适用工作分支 `next/performance-efficiency-20261008`。这是使用说明，不是第二套STATUS。核验快照：helper HEAD `c149564c5514f13c8b6f75d70d41d5e9d521656c`，产品SOURCE `a2db258b436a41fdee98c6c53abec9bab6ce600f`。最新源码/路径变动先重新读STATUS、对应工具和manifest，不复制历史PASS。

## 1. 夹具实际测什么

完整路径：`biz真实socket -> OpenWrt/Linux TPROXY正式client -> 加密FakeTCP/AF_PACKET -> router netns双向netem -> 正式server -> 共享TUN -> target真实socket`。共五个隔离netns；它们是测试网络，不是产品新增进程/路由架构。业务目标使用受控地址8.8.8.8，测试route-mode all，不访问真实公共8.8.8.8服务。

- 双向主动发送，不逐包等待echo才继续。Normal单lane双向逻辑各10Mbps；Game配置2/3/4lane双向总逻辑各3Mbps，副本不计有效业务。
- `udp/tcp/mixed`业务；ordinary UDP为96/256/512/1000/1372/4068B，jumbo/boundary另有8936/8937/8972/8973/65507B。大包路径能力不同，名称存在不等于全产品支持或已通过。
- TCP包括长流、短流、长度/hash检查；TCP/mixed带20次HTTP/HTTPS、10次HTTPS证书/body验证。mixed总逻辑预算10M由TCP/UDP共同分配，不能把双方各10M。
- 独立小UDP往返probe、missing/timeout、returned-only p99；阶段统计按发送monotonic时间归因，echo在原发送端归因。
- 正式外层MTU1400、record cap0自动派生；产品server TUN20:20样本实读1273。fixture内侧veth9000只是产生大包的测试边缘，不得固定产品TUN9000。
- 实际netem丢包/延迟、资源采样、AF_PACKET/UDP/raw socket和网卡drop、进程CPU/RSS、CPU型号/PSI/steal/配额证据；不是只看workflow绿色。
- 对原始pcap限量轮转，分析后hash并清理，只上传小证据。profile ON额外记录内部计时/队列/批量/repair等，是独立诊断，不能与OFF直接比较CPU。

## 2. 文件地图

|文件|职责|
|---|---|
|`.github/workflows/next-efficiency-e0-single.yml`|已有正式300s单样本编排、固定SOURCE构建、guard、分析和上传；当前push触发，不是自由参数dispatch|
|`.github/efficiency-e0-sample.json`|该入口唯一case；修改会触发旧性能workflow，复用新实验时不要顺手改它|
|`scripts/strict_weaknet_sample.sh`|五netns、TPROXY/TUN/raw路径、owned进程/规则及清理的权威模板|
|`scripts/build_seeded_tc.sh`|构建可核验带seed的tc，避免系统tc悄悄忽略seed|
|`tools/prepare_large_mtu_harness.py`|按精确模板片段派生一段真实业务脚本；片段计数不符立即失败，输出generated.sh及hash receipt|
|`tools/large_mtu_mixed_business.py`|双向UDP/TCP真实业务源/目标、限速、hash、probe、按尺寸/时间统计|
|`tools/efficiency_http_https.py`|短HTTP(S)、受控证书、正文验证；CPU/时间要与case一起管理|
|`tools/large_mtu_loss_stage.py`|固定损伤或75/150/75s的5/20/5波形与实测receipt|
|`tools/check_large_mtu_mixed.py`|SOURCE/helper/manifest、输入/业务/MTU/损伤/资源检查；读取pcap后hash删除|
|`tools/efficiency_cost_ledger.py`|只读CPU/GiB、交付、PPS、RSS、资源和可得诊断账本，不产生流量|
|`tools/strict_resource_sampler.py`、`tools/large_mtu_resource_report.py`|每段独立CPU/内存/系统压力及socket/drop资源观测|
|`tools/afpacket_socket_probe.py`及schedstat/report/witness工具|诊断ON的有界额外观测；普通性能不要启用|
|`tools/perf_sample_guard.py`|原正式入口的单样本claim约束；本实验不得全局删除或绕过旧门|
|`tools/check_performance_workflow_policy.py`|foundation静态策略检查；给新实验增加精确命名的串行例外校验，保留旧入口全部约束|
|`tools/prepare_e1_lowrtt_fullstack.py`、`.github/workflows/next-e1-lowrtt-fullstack.yml`|真实15ms稀疏Normal1的完整产品路径模板；它不是持续10M混合性能测试|
|`internal/linkdata/e1_sparse_real_netem_linux_test.go`|真实UDP socket+LINK/FEC定向损伤测试；不能当完整FakeTCP/TUN产品资格|

## 3. 已有入口如何使用

先在Actions checkout helper HEAD；另一个detached source worktree按完整PRODUCT_SOURCE构建client/server，记录version和binary SHA256。不得直接构建helper HEAD后声称用了旧SOURCE。

正式入口的配置示例（仅示意，不要求现在修改该触发文件）：

```json
{"workload":"mixed","loss":5205,"seed":1844,"product_source_sha":"a2db258b436a41fdee98c6c53abec9bab6ce600f","mode":"normal","lanes":1,"rate_mbps":10,"size_profile":"ordinary","diagnostic_mode":"off"}
```

在已有workflow的准备步骤中生成脚本：

```bash
python3 tools/prepare_large_mtu_harness.py --output "$ART/generated.sh" --workload "$WBD_LARGE_WORKLOAD" --loss "$WBD_LARGE_LOSS"
bash -n "$ART/generated.sh"
```

必须先按workflow设置并传递SOURCE/helper、ART、MODE、LANES、RATE、SEED、场景、带seed tc、FEC_PARITY/FEC_SCREEN、WBD_EFF_SIZE_PROFILE、WBD_EFF_DIAGNOSTIC等环境。generator的两个CLI参数并不能代替所有环境；HTTP sidecar、netem、时钟、清理、二进制选择都依赖它们。`sudo --preserve-env`须保留实验实际新增参数，不可只改manifest而产品仍跑旧值。

原分析器使用方式：

```bash
python3 tools/check_large_mtu_mixed.py --artifact-dir "$ART" --source "$PRODUCT_SOURCE" --helper "$WBD_HELPER_SOURCE" --workload "$WBD_LARGE_WORKLOAD" --loss "$WBD_LARGE_LOSS" --seed "$WBD_STRICT_SEED" --target-mbps "$WBD_STRICT_RATE_MBPS" --size-profile "$WBD_EFF_SIZE_PROFILE" --mode "$WBD_STRICT_MODE" --lanes "$WBD_STRICT_LANES" --diagnostic-mode "$WBD_EFF_DIAGNOSTIC" --output "$ART/summary.json"
python3 tools/efficiency_cost_ledger.py --artifact-dir "$ART" --output "$ART/efficiency-ledger.json"
```

注意配置JSON的diagnostic_mode是`off/on`，导出的WBD_EFF_DIAGNOSTIC及分析器CLI是`0/1`。正式workflow完整步骤是可执行使用范例；不要只复制上面两条就直接启动负载。

## 4. 当前限制：新FEC实验必须显式适配

核验快照的正式入口只准loss0/5205、FEC20:20；generator/loss-stage/analyzer的现有固定loss choices不含1%；主要300s业务与CPU/PPS账本硬编码300，helper生成manifest硬编码300ms单向。新实验需要0/1/5%、15/300ms、120s、off/20:20、同run串行多段，**当前不能直接运行**。

底层strict模板已支持FEC_PARITY=0/4/8/10/12/16/20，非20要求显式FEC_SCREEN=1，off已有single-only合法批量路径先例。复用时核实际两端--fec-parity与handshake结果，不能把FEC_SCREEN仅当改变标签；不能要求off必须形成send_multi而强行攒包。

最小做法：新建独立实验workflow+case plan+轻量serial runner/adapter；复用现有拓扑、业务和资源实现。必要给共享helper增加可选参数时保留所有正式默认、strict分支与原门；或采用专用派生器并记录原模板hash。为120s统一适配业务源、HTTP调度、stage sampler、捕获、manifest、analyzer和CPU/PPS分母，不能只把shell sleep改短。命名/claim/branch断言不可声称旧单样本入口本来支持serial。

新workflow建议固定为`.github/workflows/next-fec-policy-sequential.yml`，实验计划配置为`.github/fec-policy-batch.json`；仅明确实验分支的该配置路径push触发，一个job无matrix，批次guard核配置/源码/逐段顺序。当前仓库新feature分支workflow_dispatch尚不能假定可启动（已有E4方案为此使用配置push）；先读实际GitHub入口，优先沿用这种受限配置push，不修改default/canonical分支为触发测试。`check_performance_workflow_policy.py`目前按固定入口校验；新增精确实验名的独立串行规则，不把它放进旧单样本ACTIVE后再取消所有claim约束，也不利用未枚举workflow躲过策略门。

旧四门和E6正式300s资格不因这个120s探索实验放宽。新入口细则见[FEC策略实验](FEC_POLICY_EXPERIMENT.md)。

## 5. 已验证能力与未验证边界

32ms SOURCE的Normal1/Game2各lossless/5205有单次300s scoped PASS；15ms稀疏完整产品路径run37937452545双方2667/2667、每尺寸889、单向p99约15.44..15.75ms，没定向丢片。不要拿它证明低RTT丢片修复、CPU收益、Game4或其它FEC档位。

旧SOURCE有TCP-only输入/hash失败、8937/65507大包能力/归因未完成；profile-off没有内部lane流量证据就写configured lanes，不编actual lanes。额外socket drop和业务缺包是不同事实；FEC救回业务不能抹去socket资源FAIL。全局80秒S2C中断仍OPEN_DEFERRED。读取STATUS与原始summary才知道新源码资格，不从本文快照继承。
