# E0 场景生成 preflight 原始 FAIL，修复并继续（2026-10-08）

精确起点`ec01cf5636790dfbef7c3dddaca36d0dea92e767`、产品SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733`。单独Actions [run 37760789251](https://github.com/lly8666/wobuzhidao/actions/runs/37760789251)，job 113256400786，artifact 11541978012，attempt1，**workflow failure**。该run在 `Generate one strict fullstack scenario` 时抛 `NameError: name 'os' is not defined`，源于新增诊断关闭逻辑读取`os.environ`而遗漏import。sample stage=skipped、analyzer fail closed；没有任何300秒业务、CPU或丢包测量。错误属于助手、非产品，但原始FAIL必须保留，不能把先前scoped-PASS继承给本run。

修复只改两个助手文件：`tools/prepare_large_mtu_harness.py`明确import os；`.github/workflows/next-efficiency-e0-single.yml`将整个场景生成及bash -n从构建之后前移到Install之前，使类似助手失败发生在联网依赖安装与Go构建前，避免无用Actions资源消耗（同run仍只会执行**一条样本**）。原3s drain、profile-off禁client/server diagnostic JSONL和分析器fail closed、完整性及业务配额均保留，未动产品源码、MTU、4096、FEC、维修或生命周期。

本次提交触发的独立新helper Actions结果尚未确认，不做PASS承诺，下一步以run/job/artifact原始结果验300s真实业务、TUN真实MTU、auto record、100ms正式调度、off CPU/pps/drops及各方向探针/完整性。E0其他Normal TCP/混合、Game4、大UDP尺寸能力边界、诊断账本仍NOT_RUN；E1-E6和P6未开始，80s下行E7开放，物理未跑。

机器可读记录见[本轮evidence](../evidence/performance-efficiency-e0-run37760789251-preflight-fail.json)。
