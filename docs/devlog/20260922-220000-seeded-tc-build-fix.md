# 20260922-220000 seeded tc 构建修复

## 基线

SOURCE_SHA:
`1d192a041f3f37bcbcb28b1dd60faf9318a8da33`

上一轮弱网优先级同步：
- targeted: https://github.com/lly8666/wobuzhidao/actions/runs/35736933075 — PASS
- foundation: https://github.com/lly8666/wobuzhidao/actions/runs/35736933073 — PASS
- seeded-netem preflight: https://github.com/lly8666/wobuzhidao/actions/runs/35736933177 — FAIL_HARNESS_BUILD

产品数据面没有修改。strict 18样本workflow仍为workflow_dispatch-only；旧31分钟低负载soak未运行。

## preflight v1 原始失败

job 106776470565：

1. Ubuntu 24.04系统iproute2为6.1.0。
2. 固定下载kernel.org `iproute2-7.2.0.tar.xz`。
3. SHA256 `4c2fa124c2cf0afd7ca34d1eeacba6ba048a56f6374e2aab93dafbdbd4eea9c0` 校验PASS。
4. 当前脚本执行 `./configure` 后直接 `make -j2 tc/tc`。
5. 编译在 `tc/tc.c:19` 失败：`fatal error: version.h: No such file or directory`。

因此该失败只能说明builder绕过了iproute2正常构建依赖，**不能**说明新tc不支持seed，也不能说明runner kernel拒绝seed属性。

## 根因与最小修复

iproute2顶层构建负责生成配置；其 `version` target通过 `git describe` 写 `include/version.h`。但本harness使用固定release tarball，不是Git worktree，不能依赖 `git describe`。

本轮仅修改 `scripts/build_seeded_tc.sh`：

- tarball与SHA256固定不变；
- `./configure`后显式写
  `static const char version[] = "iproute2-7.2.0";`
  到 `include/version.h`；
- 先 `make -C lib`，再 `make -C tc`，使用项目自己的子目录Makefile与libnetlink/libutil依赖；
- 只安装 `tc/tc` 到runner临时目录。

不改产品包、不替换系统tc、不改netem场景、不删seed、不降资格门槛。

## 已确认的同SHA基础

`1d192a` targeted整轮成功，foundation整轮成功；因此文档/分析器的弱网口径同步没有引入产品回归。旧P5/P6/31m soak全部保持skipped。

## 下一步

只重跑小型 `next-strict-harness-preflight`。必须依次证明：

1. 固定tc构建成功并输出版本；
2. runner kernel实际接受 `loss random 20% seed 123456`；
3. 同一runner再接受seed 654321并保存qdisc JSON。

三项都PASS前不启动generation3的18个120秒主测。
