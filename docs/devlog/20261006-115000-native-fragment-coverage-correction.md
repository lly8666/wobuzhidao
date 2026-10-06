# 原生fragment观测覆盖失效与真实TUN回归门

## 本轮目标和阶段

P7；起始b353038，产品9211b24保持不变。seed1419完整300.086s又出现最大UDP170送169回、missing1828；8972/DFtrue另一次late，小包1359/1359 timely。这个应用失败保留，fragment观测无效不能推根因。

## 修改与原因

Linuxraw helper实际observer_packets3400但selected rows0；既有physical_header_watch已明确ETH_P_IP只看进入kernel的数据，server回复是AF_PACKET outgoing。改成ETH_P_ALL后仍只导出受控target→lease的IPv4UDP metadata；不改产品socket/缓存。增加Actions真实隔离TUN fixture：现有65507/8973/96 echo形状发送一次，独立核验48/7/1片、offset连续/总长/头部checksum/observerdrop。此为functional fixture，不是性能试验，native下一长测前必须确认回程实际可观测。

Windows --stats按安装版本help与Microsoft文档是打印统计后退出，不产生--out文件。拆成统计stdout单独保存和不含--stats的metadata conversion。原1419trace stats events0，pcapng168B无packet，虽然Wintun counters Rx13089/Tx3400：计数不等于逐片捕获成功。候选明确启用同一Microsoft-Windows-PktMon provider的0x3f/level5，以验证当前OS的packet event记录；仍仅受控IPpair/选定Wintun/64B/16MiB/390s。该兼容候选效果尚未验证，不说“已修复空事件”。不得改为TCPIP/NDIS全机trace或解除filter。

PS dryfixture遵循真实--stats不写文件语义，防止假转换门放过这类错误。Linux新增ready metadata，native需先短controlled probe确认非空packet inventory与完整覆盖，再启动长测，不再盲跑300s。

## 复用来源

ETH_P_ALL规则来自现有tools/physical_header_watch.py，无old提取。

## Actions证据

b353 predelivery37409806054四jobs/core37409806148/GUI37409806189 PASS，foundation37409806078完成7success/9skipped（scoped路径门，不冒充product全70/18/soak）。本helper修正与真实TUNfixtureNOT_RUN，提交后先验证。产品921既有资格未变。

## 问题、排查与风险

1418 onlysetup失败清理已核实：clientSTOPPED/Exit0、network/NRPT/firewall0，Pktmon未运行/filter无/raw无，serveractive/原backup无/临时地址无/18446无。

1419monitor已stop/ownfilter已删；手动只读转换保留stats与metadata摘要，rawETL/pcapng本地和remote已删除。不能把空捕获写成“没有丢片”，也不能用总frame数相等证明ID/offset正确。两个实机观察问题属于测试工具，不是FEC/MTU算法bug；产品应用missing1828独立保留。诊断有256MiB ETW buffers开销，本场不是正式吞吐/资源对比。

参考：[Microsoft pktmon etl2txt](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/pktmon-etl2txt)、[pktmon start](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/pktmon-start)。已安装程序help用于平台参数核对。没有文档可以把events0解释成有效捕获。

## 下一项原子任务

Actions真实TUNfixture和PSfixture通过后，短probe确认两端非空fragment metadata；Windows仍不支持则明确UNSUPPORTED/另选受控观察边界，不能继续相同长测假装有诊断。随后用有效边界证据决定是否有必要优化server内层MTU9000/outer独立1400或修其他交付缺陷；当前未改product MTU/timeout/FEC/4096，不恢复HOL。
