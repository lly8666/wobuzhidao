# FEC SIMD S3 真正阻塞：旧A在真实pilot服务端提前退出；停止盲跑

**当前精确工作线：** `next/fec-simd-20261010`。本轮仅修复并增强 S3 测量夹具和审计，未改变产品FEC源码，未合并或推送规范主线。

## Actions 新证据
- 原基线A `a2db258b436a41fdee98c6c53abec9bab6ce600f`；新融合默认候选B `7fb98fab79834a351a1dbe04eebb207f66bea28b`。15秒Q1 Normal1/mixed/20:20/300ms单向/0%/双向10Mbps/seed2261，Go1.23.12 linux/amd64，入口 `phase=pilot nonce10`，同一runner计划顺序A→B。
- [Actions run38036729414](https://github.com/lly8666/wobuzhidao/actions/runs/38036729414) FAIL；[artifact11663593875](https://github.com/lly8666/wobuzhidao/actions/runs/38036729414/artifacts/11663593875) 给出确定 `PRODUCT_SERVER_EARLY_EXIT`。A `sample_exit=1`、analyzer/ledger exit1，业务biz、target、stage与runtime-flags存在，但最终 `manifest.json`未写，A **INFRA_INVALID**；B **NOT_RUN**（禁止把未尝试的B当失败/通过）。客户端/服务端私有日志为352/490字节，仅上传长度/hash和分类，不上传内容。
- 脚本退出后原有1个root-owned case进程，已经通过限定PID的sudo回退发送SIGTERM，但严格clean仍false。主机4 CPU，cpuset0-3，cpu.max UNKNOWN；pre-start CPU PSI avg10 22.1%，不能把INVALID归结为单一容量原因。
- 精确两产品二进制hash和判定在 `docs/evidence/fec-simd-pilot-38036729414-server-exit.json`。旧pilot [38034569663](https://github.com/lly8666/wobuzhidao/actions/runs/38034569663)、[38036026445](https://github.com/lly8666/wobuzhidao/actions/runs/38036026445) 的相同无manifest失败及中间静态FAIL均保留，不能再无新证据盲触发。
- 同 helper 的 Foundation [38036729423](https://github.com/lly8666/wobuzhidao/actions/runs/38036729423) SUCCESS、此前 static preflight [38036674251](https://github.com/lly8666/wobuzhidao/actions/runs/38036674251) SUCCESS，**都不能覆盖真实业务FAIL**。

## 正式资格及边界
- S1共用SIMD乘加与标量回退，S2 WBD自定义矩阵、单worker fused full block已由原candidate源码实现；S2融合默认B对应上述固定源码。跨平台Core native ARM64/amd64/Windows amd64 run38034338215、Foundation38034338220、lifecycle38034338226此前scoped PASS。native ARM核心单测不是native ARM吞吐、Windows hosted不是Npcap物理通过。
- kernel微基准 [38034516259](https://github.com/lly8666/wobuzhidao/actions/runs/38034516259) 的AMD EPYC7763 P20短shard结果详见 `docs/evidence/fec-simd-micro-38034516259-scoped.json`。SIMD span对标量加速显著；融合相比span在小包只接近持平，不能保留“必有整机收益”说法，也不能按微基准通过 p99/CPU。
- **S3 真实A/B完成对数0**；Q1/Q2/Q3和L1/L2/L3的120s ABBA、跨独立runner重复、native ARM real-business CPU/p99、300s关键确认、P6最终配套包与manifest均 `NOT_RUN`。B在此pilot只有二进制hash，没有运行真实业务；有效交付GiB、CPU-s/有效GiB、RSS比较、p99/未返probe/交付空洞/raw/socket drop均不得填0或虚构。
- 当前旧A服务端提前退出的根因尚未知，需独立收集经过脱敏的服务端退出原因或构建可重复的合格旧A对照，**不能放宽业务完整性/验收阈值、不许因旧版故障跳过A仅测B称为改善**。约80秒下行E7仍按用户决定OPEN_DEFERRED，不能直接认定此次同因。保留旧11项RTT/Game4/300ms TCP-off与MTU/PMTU失败。
- physical `NOT_RUN`，不得以S1/S2核心绿灯替代P6发布/原聊天物理复验。

## 下一步
只在有**新**服务端退出/完整业务证据后恢复S3；先完整合格的A/B pilot，才按原Q/L ABBA、host分层、300秒/P6顺序；严格遵循用户的性能及质量硬门。不做第三次无变化的pilot重试。
