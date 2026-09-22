# 20260922-083500 P4 per-SHA targeted Actions

## 原因

第3原子产品提交 `9c330454e64034f0d775b78a5d6fd7a2c6a92912` 已触发 next-foundation 35670825915，但现有workflow使用分支级：

`concurrency: next-foundation-${github.ref}, cancel-in-progress:false`

上一SOURCE_SHA的两个31分钟P5 soak仍在运行，因此新提交处于pending且没有任何job。若继续沿用这一编排，每个P4原子修复都必须先排队完整P5 soak，既不是“定向回归”，也会不必要地反复消耗runner。

## 新workflow

新增 `.github/workflows/next-p4-steady-targeted.yml`，concurrency按 `github.sha` 隔离，不会排在branch-wide soak后面。它只做P4原子快速门：

- repository contract；
- Ubuntu/Windows `faketcp/runtimeowner/runtimeentry`完整单测和client/server build；
- Ubuntu同包race；
- Game3/4、same-ID replacement、DORMANT/wake、rotation、显式steady close聚焦测试；
- Linux root shared-TUN iptables/nft；
- OpenWrt风格真实TPROXY ownership。

它**不**运行、替代或降低P5弱网、负载、31分钟soak，更不用于最终真实路径18份主测。最终增强P4/P5仍须最终同SHA经过真实netns/veth/netem独立进程资格。

## SHA语义

本提交只增加Actions编排和对应STATUS/devlog，不修改第3原子的任何产品Go源码。故本基础设施SHA是相同产品代码的定向资格候选；只有新workflow实际PASS后才进入第4原子。
