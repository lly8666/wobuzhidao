# 2026-10-08 自动MTU首个独立基础功能门结束（证据分支，不变产品源码）

- 产品精确 SOURCE_SHA `f7f6e55bc9167fbd31d9b2e328a8ea70ec362ae0`，工作分支 `work/mtu-auto-inner-20261008`，父链 `b4ea061... -> 954a151... -> c912d801... -> f7f6e55...`。文档独立分支 `investigation/mtu-auto-inner-qualification-20261008` 从该产品SHA建出，不改产品源码，也不能把文档HEAD等同跑测SHA。
- 本轮核对 GitHub Actions 的三次真实workflow，不依据旧PASS字样：run37739744820/SHA954a151 conclusion FAILURE，Linux基础PASS/真实TUN PASS、Windows模拟Fail在PowerShell原ValidateRange(9000,9000)；run37740215961/SHA c912d801 conclusion FAILURE，Linux基础PASS、真实TUN PASS、Windows历史测试助手死断言9000；run37740452679/SHA `f7f6e55bc9167fbd31d9b2e328a8ea70ec362ae0` conclusion SUCCESS、run_attempt1、3/3 job PASS，<https://github.com/lly8666/wobuzhidao/actions/runs/37740452679>。
- 第三轮 job ID：Linux unit/build/race/ARM64+PARAMETERS 113189704063 PASS；Windows unit/build/owned模拟 113189704224 PASS；Linux netns gerçek kernel TUN值576/1143/1199/1249 113189704228 PASS。workflow没有上传artifact，artifact IDs空、不能虚构SHA256或声称抓包/端到端性能。
- 基础功能资格仅覆盖预算函数/跨平台Go构建回归、hosted Windows计划及恢复模拟、Linux可设置真实TUN MTU。**尚无新SHA真实Windows Wintun驱动Apply/Cleanup、普通TCP SYN/MSS/吞吐、UDP大小/DF/ICMP、真实远程物理、路径MTU自动探测资格**。不能据此宣称全功能完成，性能与p99仍NOT_RUN。
- 原12条300秒A/B/C*0/5/20/30%长混负载均保留FAIL，尤其大UDP>8936、B真实TCP注入不足、A20 1020ms零交付边界OPEN；候选2f7 UDP截断保护未合并，历史失败不因新基础功能PASS改写。
- 当前候选静态接口MTU从外层配置及本端入向record限制派生，未读取已知更小的实际underlay MTU；Windows/服务端反向record限制及peer MSS可能使单个内层IPv4包仍需要LINK多分片。下一项应独立资格验证和必要最小修正；禁止直接称之完整PMTUD。
- 测试工具提交只修旧fixture中的9000硬性断言，不宣称产品收益。下一项原子任务：GitHub Actions真实TCP MSS/UDP DF/ICMP功能；过门后严格每独立run一条旧/新300秒正常业务样本、保存offered/completed/p99+missing/CPU/PPS/queue/loss，原始FAIL不可覆盖。
