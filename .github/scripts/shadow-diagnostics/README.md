# 单条诊断交接

本分支只修改实验框架。产品固定 A：`484e29463bb83c503ee3baf669c23b013dd44d6c`。
没有修改 FEC、4096 窗口、DTLS、ACK 或 repair 策略。A 是最简单的诊断基线，不代表已评出性能冠军。

## 执行约束

**一个 workflow run 只有一个场景、一个测试 job，无 matrix。请上一条完成后再运行下一条。**

按顺序单独运行：

| profile | 测量路径 | 内层速率 | 单程延迟 | 每方向随机丢包 |
| --- | --- | --- | --- | --- |
| direct80 | 本机 UDP echo，绕过产品 | 80 Mbps | 0 | 0 |
| a20-loss20 | A 完整单 lane，FEC 20:20 | 20 Mbps | 300 ms | 20% |
| a80-loss0 | A 完整单 lane，FEC 20:20 | 80 Mbps | 300 ms | 0% |
| a80-loss2 | A 完整单 lane，FEC 20:20 | 80 Mbps | 300 ms | 2% |

每条只发 20 秒，收包最多再等 65 秒。重复实验也必须新建一次独立 run。
不能把不同 hosted runner 的差异当作某一个参数的因果效应；先做以上筛查，异常场景至少另跑一次。

工作流：`.github/workflows/shadow-isolated-diagnostics.yml`。
若已在默认分支注册，可对本分支手动 dispatch，选择一个 profile。
若 GitHub 不接受未注册 workflow 的 dispatch，使用以下推送入口（无需先合并产品）：

1. 从本交接提交创建 `run/shadow-isolated-<profile>-<unique-id>` 分支。
2. 将 `.github/scripts/shadow-diagnostics/profile.txt` 改成表中的一个 profile。
3. 提交并推送该分支；工作流只读取这一行，未知配置直接失败。
4. 首条 direct80 与当前默认值相同时，可在该文件增加一个末尾空行形成变更；shell 会移除末尾换行。
5. 运行提交不要带 `[skip ci]`。仓库已有普通 CI 可能也响应 push，但本诊断 run 始终只有一个场景。

未触发任何线上实验。交接提交的 `[skip ci]` 用于阻止发布框架时启动原有 push CI。

## 改动和读数

- echo 热路径移除逐包十六进制落盘。保留原来的 socket 配置，避免靠增大缓冲掩盖问题。
- 发包时间戳取实际 sendto 前时刻；RTT 不再混入计划发包时间的落后量。
- 每轮最多发送 32 包、读取 256 包。过期发送槽位跳过并计数，避免无限追赶导致大突发。
- `load_valid` 表示实际发送达到目标的 99.5%，且没有发送错误或载荷损坏。false 时场景失败，不能据此评判产品吞吐上限。
- `down_payload_bps` 含 drain 收到的包、分母为发送窗口，表示最终完成量；不是稳定期吞吐。RTT 分位数仅覆盖已返回包；`within_1000ms_ratio` 分母是所有成功发送包，包含丢失影响。
- 每秒以及负载结束、进程退出前采集 CPU/softnet、进程 CPU ticks/starttime/线程数、socket inode、namespace UDP/raw 表的 drops、SNMP 和 tc qdisc 累计量。使用时间差求增量，区分 netem 与本地丢包。
- 采样有开销，尚未实测量化。CPU 满载只能证明资源压力，不能单独证明锁争用或归咎虚拟机。
- `provenance.json` 记录产品/helper SHA、run/attempt、CPU 和配置；另有二进制 SHA256、测试脚本 patch、各层原始日志及最终 shadow 计数。
- 上传白名单不包含 ticket、证书私钥或 tunnel 配置文件。

诊断 run 绿色仅表示执行和基本 guard 成功，**不是低丢包/低延迟验收通过**。
缺失任一端 teardown stats 时，不能补零或进行完整收支归因；结合原始日志注明缺项。
本框架尚未增加产品逐包追踪，不能用各层累计计数直接证明某一具体序号在哪个函数丢失。

## 判读顺序

direct80 不达标：先处理测量器/runner；不要调产品缓存。
a20-loss20 正常而 a80-loss0 异常：优先排查高负载本地处理；结合 UDP/raw drops 与 CPU，而不是归因 FEC 抗丢包不足。
a80-loss0 正常而 a80-loss2 异常：再检查损失触发的 ACK/repair 开销、排队和 FEC 分组损失。
如果 Game 发出量到 DTLS 输入量已明显减少，缺口发生在 FakeTCP 前，4096 repair 淘汰不是这个缺口的直接解释。

## 已完成验证

对固定 A 的原始测试脚本离线生成完整最终 harness，检查替换标记、单 lane/netem 配置、echo 热路径和监控插入点，完成 shell/Python 语法检查。没有启动网络负载，也没有执行 Actions。

可复查：`python3 .github/scripts/shadow-diagnostics/check_generation.py <A-checkout>`（Linux）。
