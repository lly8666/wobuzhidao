# 20261003-202000 长测抓包分段修正

## 本轮目标和阶段

P5/P6交付前验收。起点 eb03fae1c27a7eb06ca9141b0d6371f5591125bb，保持产品运行代码不变，修复四lane抓包整理的框架容量误判。

## 修改与原因

180s Game run 37121715765 已完成业务和停流，但离线 soak_capture 的100MiB/60s文件上限不足，导致manifest与汇总未生成。tcpdump改为15s非覆盖分段，保留128B snaplen、100MiB单文件硬界和16GiB总磁盘界。manifest在离线处理前生成，任何后处理失败仍保留源码/负载身份。不改变速率、产品、网络、socket、允许损失和时延门。Actions fixture核对四处15s分段和manifest顺序。

## 复用来源

当前 tools/prepare_soak_harness.py 与 soak_capture.py，无old复用。

## Actions证据

eb03源码 foundation 37121335125、工具及30次race 37121335185 PASS；五个live config run 37121717603/19594/21927/24268/26447 PASS；Normal180 run37121713748 PASS。Game180 run37121715765 FAIL（抓包后处理文件大小），原始artifact11273229729保留。不得把它改写成正式PASS。新SHA框架/配置/长测均NOT_RUN，待精确源码独立Actions。

## 问题、排查与风险

不是产品性能结论，也不是免检runner原因。原始文件仍可在只读Actions诊断恢复其业务数据。正式1800s需验证总磁盘界及完成所有压力阶段。15s分段不会丢弃或抽样捕获数据；压缩在测量结束后。

## 下一项原子任务

新SHA基础门通过后重新运行Game180诊断，并展开70配置、18独立弱网、正式长测及P6包。P7仍NOT_RUN。
