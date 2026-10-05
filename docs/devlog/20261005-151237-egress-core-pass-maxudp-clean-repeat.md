# 回程资格核心通过与最大UDP干净复测

## 本轮目标和阶段

产品候选SOURCE2737415，仍部署660。回程mask正确性/race先验，完整性能还未收齐。

## 修改与原因

本次仅归档已完成结果并更新唯一状态，产品算法不再叠改。273基金foundation37275839495、targeted37275839507、lifecycle37275839449、GUI37275839509、Linuxserver37275839467 PASS；新Game单lane资格/未编码兄弟/partial入口交付、既有多lane首次交付/去重和自动租约race门通过。9408旧Windows间歇失败保留，无新上下文复现，原因仍未确定。

## 复用来源

现有同源P7助手和Actions；无old导入。

## Actions证据

273独立lossless/5205 Normal10 seed1401，以及lossless/5205/5305 Game4×3 seed1382五run在跑，profileOFF/dynamic邻居。每Action仅一sample；单workflow green不等于配对p99门。

## 问题、排查与风险

660 M03 seed1398完整300.020233s，8972false172/172、true171/171，8973false171/171，65507false171/171、小包1368/1368，target计数/逐序号全部匹配。8973/65507 DFtrue、65508两DF API均按预期MessageSize；无bad/duplicate/timeout/socket错误、存活/owned退出PASS。Windows全机ReassemblyFailures前后0；它不是每隧道归因。首次新诊断样本没再缺包，不抵消1365/1366旧各1包最终缺失，不把没有产品改动的repeat宣称修复。seed1397 SSH过期无负载不计结果。

原生23完整样本/11工况/32 NOT_RUN。两邻居诊断均健康、无秒级尾，故ARP和shared锁根因仍未证实；不凭跨宿主CPU差异宣布收益。bounded抓包已按既有助手分析并删原pcap，没有下载Actions大pcap。纯休眠fixture不评p99。

## 下一项原子任务

五独立性能样本正式配对/CPU/逐秒门通过后下载同SOURCE273配套包，先native S19五分钟复验醒来首秒损失；不盲扩buffer或叠加raw算法。p99原失败独立继续定位，最大UDP旧原因保持未收口。
