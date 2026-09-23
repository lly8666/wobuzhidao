# 20260923-184000 strict workflow GitHub expression 修正

## 结论

Actions dispatch 中继已经成功创建正式 `workflow_dispatch` run，但 run `35842774407` / job `107121575982` **没有产生性能样本**，不能记为产品/性能 FAIL。

## 原始失败证据

SOURCE_SHA `e80feae62536c975ce92d6ad4a165451d1c7cc2d`：

- relay attempt 2 明确 dispatch：`normal/lossless/seed601/rate10/lanes1`；
- strict job 的环境实际显示 `WBD_STRICT_MODE=\\normal`、`WBD_STRICT_SCENARIO=\\lossless`、seed/rate/lanes 同样带前导反斜杠；
- `scripts/strict_weaknet_sample.sh` 在进入测量前 exit 2；
- `loss-tolerant-v1` 报 `invalid choice: '\\normal'`；
- artifact 名被渲染为含 `\\`，`actions/upload-artifact` 因非法字符拒绝；
- Actions artifacts API 返回空数组。

根因是 active workflow 文件里 GitHub expression 被保存成字面反斜杠 + GitHub expression，而不是正常 GitHub expression。这是控制面模板错误，不是 dataplane 性能结果。

## 修复

本提交只做 Actions 控制面机械修正：

1. 去掉以下三个性能workflow中 GitHub expressions 前的字面反斜杠：
   - `.github/workflows/next-strict-weaknet.yml`
   - `.github/workflows/next-strict-capacity-diagnostics.yml`
   - `.github/workflows/next-strict-packet-socket-diagnostic.yml`
2. `tools/check_performance_workflow_policy.py` 新增静态门：ACTIVE performance workflow 出现 escaped GitHub expression 即失败。
3. ` .github/perf-dispatch-request.json` revision=2，从而生成一个新的relay request HEAD；仍是完全相同的首个资格输入 `normal/lossless/601/10/1`。

不改产品代码、wire、FEC、MTU、rate、lane或loss-tolerant-v1门槛。旧run和失败日志保持不变。

## 同SHA非性能回归

`e80feae` 的 `next-p4-steady-targeted` run `35842730081` 全部PASS；`next-foundation` run `35842730106` 的repository contract、Linux/Windows active-go、P2及privileged jobs全部PASS，历史P5 measurement jobs按政策SKIPPED。

## 下一步

新提交触发relay attempt 1后，由connector重跑relay job形成attempt 2，再由relay创建新的 `next-strict-weaknet` workflow_dispatch run。只有真实sample完成、analyzer输出和immutable artifact存在后才给出性能结论。
