# 20260923-183000 Actions workflow_dispatch 受控中继

## 问题定位

`next-strict-weaknet.yml` 已改成纯 `workflow_dispatch` 以满足 one-run-one-sample，但仓库默认分支是 `main`，该工作流此前只存在于 `next/tlslike-dataplane`。GitHub只会在工作流文件存在默认分支时接收 `workflow_dispatch`，因此此前可由push自动运行的正确性Actions没有问题，而新的纯手动性能入口无法从connector直接创建新run。

## 默认分支最小注册

`main` fast-forward提交 `850be97845e737affa13e9e7a5b3d216e6fd47ae` 新增同路径 `.github/workflows/next-strict-weaknet.yml`。该文件只负责注册workflow/input schema，默认分支版本的job固定主动失败，避免误在main上执行性能样本。对 `--ref next/tlslike-dataplane` 的dispatch由目标ref上的正式workflow版本执行。

## Active branch受控relay

新增 `.github/workflows/next-performance-dispatch-relay.yml` 与 `.github/perf-dispatch-request.json`：

- relay仅在request/relay文件push时创建run；
- attempt 1只校验请求、确认远端branch HEAD与`GITHUB_SHA`完全一致，然后输出`ARMED_ONLY`，不产生性能样本；
- connector已有`rerun_workflow_job`能力。attempt >=2 才用该run的`GITHUB_TOKEN`调用 `gh workflow run next-strict-weaknet.yml --ref next/tlslike-dataplane`；
- dispatch前查询同workflow/branch的既有workflow_dispatch runs；若已存在相同`head_sha`则直接退出，保证一个request HEAD最多一个正式样本；
- relay本身不包含任何measurement marker，不执行sample，不改变`next-strict-weaknet`的`workflow_dispatch`-only和one-run-one-sample政策。

GitHub官方语义允许`GITHUB_TOKEN`触发`workflow_dispatch`产生新workflow run，且rerun会递增`run_attempt`；dispatch endpoint需要Actions write权限，因此relay只授予`contents: read`和`actions: write`。

## 首个请求

`normal / lossless / seed=601 / rate_mbps=10 / lanes=1`，FEC仍由正式sample脚本固定为20:20。产品实现仍是`610d5092`，正确性修正仍是`6c4169f7`；本提交只修Actions控制面。

## 当前状态

本日志提交时首样本仍为`NOT_RUN`。下一步是等relay attempt1完成后，由connector重跑relay job；随后读取新建的strict workflow run、artifact和`loss-tolerant-v1`结果。原有FAIL与artifact继续保留。
