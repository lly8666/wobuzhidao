# E3 Stage B：最新Game4分片诊断通过、0入队丢失；独立OFF复验发测（2026-10-09）

仅工作`next/performance-efficiency-20261008`，父HEAD `2eaae1ae3ba0656ba6e9bfcc44360d58ca81fe4c`，冻结产品 `1146448d6340cf2e09b525189d233222d7ec9501`（4个固定入站shard各960 slot+control256，总4096；原有TLS/FEC/owner PacketID鉴权及auto MTU未改）。真实[Game4 staged5205 诊断ON Action 37821931789](https://github.com/lly8666/wobuzhidao/actions/runs/37821931789) job113464880736、artifact11569413849，**原始Actions SUCCESS、analyzer PASS_SCOPED_ACTIONS issues=[]**，sample/ledger完整，一Action一条样本。独立seed1836，真client TPROXY→加密outer→server TUN(实际MTU1273)，Game4真实active/physical lanes4，mixed TCP/UDP/HTTP(S)，每向逻辑3Mbps，300s+3s drain、300ms单向、FEC20:20、paddingOFF、record cap自动0、netem 5%前75s→20%中150s→5%后75s、profile ON。

**分阶段准确业务：** C2S与S2C UDP pre25792、stress51503、post25767，全部**0 missing**；C2S probe1500/1500、S2C probe1495/1495，全部0 missing、returned-only p99 621.049/623.758ms；TCP304/304条流CRC/长度匹配、HTTP(S)20/20、HTTPS10证书+body校验通过。netem C2S丢437404、S2C丢436220外层包（整段realized12.20%/12.18%；这是真实按阶段损伤，不可说zero-loss网）。本条socket/interface drop0。独立业务各向goodput2.9723Mbps。实际资源层AMD EPYC9V45 4vCPU，CPU PSI some10峰35.93、hostbusy峰60.47%，产品client111.95/server110.49=222.44 CPU-s，是**diagnostic ON**，严禁拿它与OFF算CPU收益。

**准确“一个锅”证据：** 原外层全局ready容量4096、peak2953，read=handled=3152943、overflowDrops=0；4个worker FIFO各cap960，分别enqueued/handled 788497/788497、788054/788054、787680/787680、788712/788712；其oldestDropped/rejected全为0、peak743/750/726/769、max age 65.47/59.01/58.18/64.00ms。后台Game TCP维护scheduled3156、coalesced21。上述read==handled与内部worker账本证明本**单条**诊断中的吞吐均匀和没有入队丢弃，但不能保证高压宿主永不溢出。

**稳定性并不合格：** 先前同产品系列/场景Game4 staged5205 OFF [run37817466498](https://github.com/lly8666/wobuzhidao/actions/runs/37817466498) 在AMD EPYC7763、hostbusy峰88.74%/PSI39.62，原始FAIL（S2C stress UDP446/51503永久缺、probe也缺）；在AMD EPYC9V45 [run37820693938](https://github.com/lly8666/wobuzhidao/actions/runs/37820693938) OFF、seed1835则PASS_SCOPED。新ON样本在9V45仍PASS只表明该宿主下方向正确，**绝不抹掉7763原始FAIL或归咎纯环境；也不能用ON/OFF混淆收益**。

本提交不动任何产品逻辑，只将唯一 `.github/efficiency-e0-sample.json`更新为**game4/mixed/5205/seed1837/profileOFF**，冻结相同PRODUCT_SOURCE `1146448d6340cf2e09b525189d233222d7ec9501`，helper取此次提交`GITHUB_SHA`，标准单一300s+3s无损哈希/真阶段5→20→5样本，无同runAB/matrix。读取原analyzer PASS/FAIL、每向pre/stress/post UDP和独立probe、host型号/CPU PSI/socket drops以及实际资源层；若落在高压环境仍失败，下一步优先按已归档分片960队列账、raw send contention与per-record热点判断最小修复，不冒险将4条lane合到一个更大的未认证队列。E3仍BLOCKED、Normal/Game2/低RTT、真正5205复测及三次同资源层OFF都还缺资格，E2无证实CPU获益。原E7约80s下行仍OPEN、E6/P6 physical NOT_RUN，main与实体未动。

[机器证据与精确源身份](../evidence/performance-efficiency-e3-game4-shard-5205-diagnostic-pass-run37821931789.json)。
