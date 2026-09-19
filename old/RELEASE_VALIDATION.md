# 本提交验证记录

运行环境：WSL Ubuntu-22.04，Go1.23.12，GOTOOLCHAIN=local。

## 通过

- 单测：internal/fec、internal/linkdata、internal/session、internal/faketcp、internal/pathmtu、cmd/wbd-faketcp、cmd/wbd-faketcp-mux、cmd/wbd-link-proxy。
- race：internal/fec、internal/linkdata、internal/session、internal/faketcp。
- 仅两个LINK入口的maxBlocks有产品行为差异：64→640。

## 未通过，随预发布公开

cmd/wbd-link-server-mux 的 TestLinkPolicyForInnerMTURejectsOutOfRange：

```
mtu_policy_test.go:37: mtu=1461 unexpectedly accepted
```

在独立、未修改的父提交 bfca8bdc8bc42840607a1a5cc6de18b806e02bb8 上运行同一个测试，也得到同样失败。此处记录事实，不擅自判定是测试过期还是策略实现有误。后续agent应核对MTU契约并单独修复；本基线没有隐藏、删除或跳过该测试来宣称全绿。

## 未执行

没有为本发布提交新跑fullstack、native wolfSSL构建、Windows/OpenWrt构建或完整仓库测试。历史Actions测试的是父源码加测试覆盖，不能自动当作本tag全部平台的验证结果。

本发布提供源码和开发交接，不提供未经本提交验证的二进制，也不承诺性能达标。
