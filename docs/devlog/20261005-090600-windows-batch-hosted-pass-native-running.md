# 20261005-090600 Windows batch hosted资格通过，原生复测开始

## 本轮目标和阶段

继续 P7 原生容量缺陷。产品固定 SOURCE `24ff220b86b103689d3e2cebcb8411f87a4910ab`，本提交只登记真实Actions、同源部署及正在执行的300s，不改产品。

## 修改与原因

更新唯一STATUS/入口和证据，区分hosted回归与真实Npcap性能。不把新Linux runner较低CPU当作Windows补丁提速，也不将正在测的前段满速当整条PASS。

## 复用来源

上一候选日志085400、既有单样本strict、P6配套包、原生有界D01框架。无新算法或参数。

## Actions证据

全部精确24ff：foundation [37249419753](https://github.com/lly8666/wobuzhidao/actions/runs/37249419753)，Windows/Linux unit、race及真实平台检查SUCCESS；targeted37249419725、GUI37249419694、lifecycle37249419692、**fullstack37249419695**、P637249454182均SUCCESS。网络/DNS分流/填充/服务端专项也SUCCESS。

独立单样本Normal5205 seed1271 [37249449381](https://github.com/lly8666/wobuzhidao/actions/runs/37249449381)、Game5205 seed1272 [37249451343](https://github.com/lly8666/wobuzhidao/actions/runs/37249451343)：CAPTURE/CORRECTNESS/ENVIRONMENT/INPUT_VALIDITY/PERFORMANCE全部PASS，socket0/errors空。CPU57.07/57.11、52.35/49.26 CPU-s/120s仅属该两台hosted runner，不是相对8f的优化收益。原summary与byte-range取回的小raw计数/版本/host manifest完整保留，未下载整份巨型pcap。

证据 [windows-ready-batch-actions-24ff220-20261005.json](../evidence/windows-ready-batch-actions-24ff220-20261005.json)及同名压缩回执。latest full70/strict18/1800s仍NOT_RUN，不能继承历史2b。

## 问题、排查与风险

Windows和ARM均核验同SOURCE版本、manifest所有file hashes、Actions gates，再部署；server保留8f rollback、原配置不变，客户端保留installation-id/原受保护配置。Npcap未重装：libpcap DLL1.10.6，Npcap driver1.88；不能把DLL版本误认成驱动版本。部署前产品进程0、owned NRPT/firewall0、ARM无raw pcaps/临时备份、服务active。

D01 seed1351已开始，300s双向Normal10M/FEC20:20/all/默认系统DNS，诊断on、CPU profiler off。早期batch_calls确实增长，driver drop0、用户overflow0，约79s双向累计接近目标，但仍RUNNING：必须等全300s与drain/正常cleanup，查看最坏排队年龄与RTT。Windows真实批量路径的收益仍未验收。

## 下一项原子任务

收齐D01，核对write calls/batch次数、DLL墙钟时间/send lock wait、两类drop、业务loss/RTT/CPU/原始完整性和cleanup。若收口则同SOURCE独立重复D01并rotation；若失败按新时间线定位最早handler压力，不扩大FEC/receive/shadow。继续剩余DNS/IP/MTU/idle/config矩阵，每Action性能一条。
