# 分流最终验收收口

## 目标和阶段
IP_SPLIT_ROUTING功能验收与独立5205回归、36生命周期、P6新包。

## 修改与来源
本提交仅补充每提交新增日志和STATUS索引。275ab94四模式真实TCP/UDP-DNS/HTTPS分流及休眠/唤醒/清理全部PASS；其foundation与targeted被仓库检查阻止，因为上一提交修改已有日志而没有新增日志。保留原始失败，不更改仓库检查门槛。产品代码未变。

## Actions与风险
275ab94 splitroute37154208594 PASS，foundation37154208522/targeted37154208515 FAIL（文档契约），不冒称产品core已运行。修正后新SOURCE需重新通过foundation才启动独立Normal/Game5205。此前Windows/Linux core d11 PASS只作历史证据。物理网卡/Npcap/Wintun与ARM原生NOT_RUN。

## 下一项
冻结本提交，当前SOURCE foundation与四模式PASS后独立性能、生命周期、包；证据如实更新。
