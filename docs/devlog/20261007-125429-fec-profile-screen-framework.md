# 20261007-125429 其他FEC快速独立Actions筛查框架

## 本轮目标和阶段

用户授权快速批量其他FEC测试，指标符合理论/性能目标则冻结当前实现。产品SOURCE d6cb6cee4c241aac8dd2f542a876edc57bf3d7db不变；本轮只改测试框架/文档。

## 修改与原因

新增next-fec-profile-screen单样本workflow，显式FEC0/4/8/10/12/16/20，shared strict脚本默认20语义保留，非20必须专用screen标记，双方flags/manifest/诊断实际TxPath/RxPath/profile一致才可通过。独立schema防止其他档位误混formal20:20 final18。原analyzer所有输入/capture/正确性/资源/性能门完整作为reference保留，任何FAIL不改绿；新screen额外检测覆盖完整wall数据与至少2完整秒无业务交付，不将其单独冒充严格noHOL证明（codec/transport定向门仍在core）。

理论输出按full20+R及actualpartial min(k,R)全部k的source残余范围，明确非business fragment/Game/finitepolicy精确oracle。Screen结果一条一份，不为让理论看起来匹配修改runtime/FEC/重传/4096。旧正式20门/已通过结果不变。原始pcap分析完保留hash/byte/count回执后删除，上传仅小JSON/JSONL/txt/log，不保存业务payload或测试TLS私钥。

新增7个测试框架单位检查：理论source残余不是整块失败率、partial/off/非法geometry、实际两向profile、错源/缺诊断/伪profile拒绝、连续性覆盖及中段停顿。只在Actions执行，不本机测试。preflight执行该检查及原seedednetem/资源/单run策略；每performanceAction仍只一条。

## 复用来源

沿用本分支scripts/strict_weaknet_sample.sh和loss-tolerant-v1分析函数，不导入old。无产品模块/参数清单变动；新输入是测试配置，不是新增产品配置项。

## Actions证据

当前框架尚待本提交Actions preflight，未部署新产品或启动36条负载。预期6profiles×2modes×3scenarios=36独立run，Normal每向10M/1lane，Game每向3M/4lane；每profile/mode配对同seed独立lossless，300ms每方向、30/60/30秒、混合包、paddingoff，产品源码固定d6。一次screen不是正式三重复资格；给出有限样本/不同runnerCPU边界。

## 问题、排查与风险

源shard理论不是多片业务丢失；若只原20损失参考门失败，保留REVIEW_OR_FAIL并人工按实际分片/partial/有限policy解释，不修改历史PASS。本机socketdrop/CAPACITY_LIMITED、内容损坏、输入无效、p99或连续性超标仍需定位。未实现trace oracle不能宣称精确恢复效率已验。S17原生已完53普通样本/24工况/19NOT_RUN，raw686仍报压力，P7/M03与当前完整资格不关闭。

## 下一项原子任务

Wait Actions preflight for explicitFECscreen identity/partial math/one-sample policy; if PASS freeze harness SHA and dispatch36independent Actions at exactproduct d6, oneprofile/mode/scenario/seed perrun. Collect all compact receipts, per-profile same-seed lossless RTT200/500ms checks and probe coverage/continuity, compare observedloss to actual theoretical bounds as reference, no blind runtime change. Rawpcaps hashed/deleted afteranalysis, no rawpayload artifacts. Freeze profile implementation if performance/latency/noHOL/capacity consistent; isolate/repeat only failed anomalous samples. Remaining19native/M03/full70/18/1800s are not closed.
