# 2026-10-12 N2 C08：修复早期空lane，补齐受限CPU资源证据候选

上一提交 `fd5282ce8ce2c02bdf8eb4854dbf3ce404493cc4` 已**永久保留**首轮C08 Action [38203559446](https://github.com/lly8666/wobuzhidao/actions/runs/38203559446) job `114661980342` artifact `11726706308` 原始FAIL。SOURCE `251b6225c1d5df8fd7f1110b04b296075d4b1f60`、artifact SHA256 `dc92606858bcee76565e3258f5d3d40d49bac00b8e7907b901a7d30cbf25e962`、helper `413cd0ebe194df4a514b9b32eb1efe58b331263293b66183220ba920c505dc50`、freeze `288d005f6b419d41071648dd778191e8236fdc9abe2fa07d64cb0f0086262a8f` 精确记录在 `docs/evidence/adaptive-n2-c08-first-analyzer-fail-20261012.json`；不覆盖。

本轮代码只动 `tools/adaptive_n2_c08_steady1.py` 和单条C08工作流。原因：server诊断在最初lane尚未建立时合法输出`lanes:null`，原分析器对None迭代抛TypeError；改为`(prod.get("lanes") or [])`，保留断言有效活动期必须有充足快照。Actions在真实测量前新增**低成本**回归：Python source语法和client/server初始空lane输入，随后才跑同SOURCE的五netns性能；本地不执行Go/功能/性能测试。

同时第一轮产物虽含runner-cpu-scope/PSI/quota，**没有**实际client/server进程CPU采样，不能证明资源目标。派生脚本在业务开始时启动原项目已有 `tools/strict_resource_sampler.py`，按5秒有界采样client/server/router，业务及drain完毕后只清理own PID；不改变正式产品、密文、netem、FEC算法或业务调度。新增完整 encoder 累积`closed_by_k/source_bytes/parity_bytes`入有界分析，补qdisc总bytes/业务源bytes放大量，明确总体wire不可全部算FEC；CPU只计诊断ON本机，不跨虚拟机比CPU秒。若采样文件缺失标为测量无效，绝不冒充CPU=0或GRADE PASS。

同配置、同冻结模型版本`n2-independent-iid-timeband-v1`，20:4最低/普通允许R4..R8，末60s活跃>=90%；真实组k与UNKNOWN边缘分开。下一次只由**一次workflow推送**引发一个SOURCE/配置/场景/测量job C08独立Action；fresh Go unit/race+fixture smoke precede load。无新生产软参数、CLI/JSON/catalog/GUI变化，故不做伪同步。下一次任何失败应报告并局部定位，不通过延长尾段、挑runner/seed或更改验收范围消除。N2尚未正式通过，N3-N6尚未开始，physical NOT_RUN，历史2.73s和80s问题OPEN，不部署不移动qualification ref不合并。
