# E0 普通无诊断基线纠偏（2026-10-08）

**精确提交父 HEAD:** `ed7e3d08ecaaabd9ae60a9dc215d68daa67a3dd2`，目标 `next/performance-efficiency-20261008`，正式产品 SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733`。此前单条 [run 37755799764](https://github.com/lly8666/wobuzhidao/actions/runs/37755799764)（artifact 11539464746、seed1801，Normal10 UDP ordinary lossless）原始 `PASS_SCOPED_ACTIONS` 不覆盖 CPU 基线；原 FAIL 与 PASS 均不改写。

## 问题与修正
1. 原生成器 `DRAIN_S=15` 与 E0/E6 要求3秒不符：`tools/large_mtu_mixed_business.py` 改为3，`tools/prepare_large_mtu_harness.py` manifest `drain_s=3`，真实负载仍300秒且goodput分母300秒不变。没有排队延迟或业务降速。
2. `--diagnostic-jsonl` 非空会让正式server的 `ObserveTiming` 自动启用 per-record clock，用户要求普通profile-off**不能**开启。生成器默认 `WBD_EFF_DIAGNOSTIC=0` 时删掉client/server诊断CLI入参；侧车独立 `strict_resource_sampler.py` 继续读取CPU/host CPU-PSI/steal/cgroup、queues/drops/pps观测数据。此项修复不动产品实时数据面。
3. 分析器 fail closed 验manifest中300s、3s drain、100ms源默认tick和自动record 0，且普通样本不得发现任何诊断JSONL；修 `large_mtu_resource_report._diag` 对历史诊断层级（client product.owner/lanes、server product.tunnel.*）解析，以备独立诊断样本。原首条cpu/profile on审计保留。

## 本次冻结计划与资格
样本仍固定唯一 `.github/efficiency-e0-sample.json`：Normal单lane、每方向逻辑业务10Mbps、UDP ordinary 96..4068B、lossless、seed1801、300ms单向、300s业务、3s drain、FEC20:20、padding off、外层MTU1400、client/server正式CLI record-limit0。每次push只触发 `.github/workflows/next-efficiency-e0-single.yml` 一个测量job**一条样本**；不存在同run A/B、性能matrix或连续校准。工作流正式版本和helper SHA由Action自身GITHUB_SHA绑定；产品SOURCE固定bf11fbf。当前提交之前尚未有修正后的run结果，故 `NOT_QUALIFIED`，不能用前次原PASS充当新helper验收。

更多差异见 [evidence](../evidence/performance-efficiency-e0-profileoff-correction-20261008.json)。下一步只读读取该提交引发的独立Actions运行及原始artifact，确认diagnostics absent、业务输入/交付、时延、MTU、进程CPU、runner型号配额PSI、队列/drop。此单案例通过也不证明TCP、混合、Game、jumbo、功能全验或CPU收益；E1开始前还需相同正常off基线及成本账本。

80秒下行失活仍OPEN留E7，旧证据不可删。未操作物理机，未修改主线、自动MTU/FEC/systematic/4096/noHOL/owned清理协议；没有 `PHYSICAL_PASS`、同源P6包未创建。
