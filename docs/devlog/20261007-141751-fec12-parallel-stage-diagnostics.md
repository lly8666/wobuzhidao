# 20261007-141751 并行定位20:12客户端短停顿

## 本轮目标和阶段

用户明确要求多线程排查解决，root与client_locks/raw_io/repair_fec三路并行审计。开始HEAD d7614b91；当前产品d6不变。先补Linux客户端丢失的既有stage timing接线，再实证阻塞位置，不盲改FEC、4096、缓存或同步发送策略。

## 修改与原因

- root：Linux入口复用Windows已有private env校验，在网络修改前校验，仅值1且显式diagnostic-jsonl启用ObserveTiming及raw写入计时。strict harness只在cpu_profile=true明确开启，manifest分别记录requested。普通qualification保持off。
- client_locks：runtimeentry可选client_pipeline固定累计计数，分开consumer总时长/state锁等待/handler以及tick/runtime/retire/schedule；off JSON省略、无新增热点时钟，不改消费/错误/生命周期顺序。
- raw_io：raw single/batch增加default-off lock_wait/lock_hold/marshal/syscall聚合，不改发送锁范围、flags、timeout、部分发送前缀、回执和所有权。每次调用/chunk只读一次开关。
- repair_fec：补齐laneTransport.handleSegment八条早返回锁持有计时，并将owner.Stats锁等待纳入OwnerNS；原返回值/Emit位置不变。不是FEC算法修改。
- 三路分别补默认关闭、开启、错误/早返回、wire及有界计数测试；已格式化和diff静态检查，unit/race仅Actions。

## 复用来源

复用新分支qualificationdiag.ClientStageTimingEnabled、durationAccumulator、既有阶段计时与strict profiler。无old提取或新公共参数，PARAMETERS.json不变。

## Actions证据

旧产品d6专项诊断37579975921 / harness48d9cf2 已SUCCESS。CPU/mutex/block原始top与hash：docs/evidence/fec12-d6-contention-profile-20261007.json。client CPU92.76s/148.20s；累计mutex8088ms，其中Lane.outbound5212ms、raw单包604ms/batch679ms；xorMul占18%、syscall35.63%。这些不是单次284ms停顿证据，不把profile当普通性能资格。旧Linux没有启用stage timing，该run不能冒充新字段实效。候选新SOURCE待提交、core/race/定向诊断NOT_RUN。

## 问题、排查与风险

客户端mux与上行业务同时积压，raw同步发送锁和Lane编码/解码共享锁都是候选。netem延迟并不意味着Sendto同步等300ms；累计profile不能证明单次慢调用。新计时包含调度等待和嵌套耗时，禁止相加当CPU或把重叠累计当唯一根因。默认off不采时钟，新raw atomic读仍有极小成本，需普通无profile样本验证。11配对RTT FAIL、物理S01/S16/M03与未跑项保留；不能用新诊断关闭FEC/全产品性能。

## 下一项原子任务

提交并冻结候选ref，先core/race，再单独Normal20:12/5305 seed1508 profile Action验client_pipeline、lane/transport与raw write_timing实际开启，按max增量定位阻塞。仅改有实证的原因，后续普通单样本验证吞吐/p99/noHOL；每性能Action一条，不把诊断收益算性能优化。
