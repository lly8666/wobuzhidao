# 原生Game休眠的额外业务边界与只读元数据观测

## 本轮目标和阶段

产品/部署SOURCE660b370，文档/观测代码HEAD773764c。用户p99与性能优先；各四条独立Actions workflow已成功，但正在核对原summary/配对，不先写全性能PASS。

## 修改与原因

tools/native_idle_boundary_observer.py是可选P7只读助手：TUN上仅观察本lease的上行，排除fixture UDP18445；最多1000frame/335s，保存numeric时间/地址/端口/协议，不保存tcpdump正文、DNS问题或pcap，不生成一包流量。未解析/cap/drop明示不能证明完全静默。新增parser测试防正文泄漏/协议猜测，接入predelivery Actions；产品代码、4096/FEC/缓冲完全不动。

## 复用来源

沿用physical_dns_observer的有界tcpdump机制，扩大到idle背景上行的numeric tuple。

## Actions证据

773 foundation37267913480、targeted37267913600、analysis37267913440、predelivery37267913542、GUI37267913529、lifecycle37267913590、fullstack37267913569全部PASS。Gamelossless37268226805/5305同seed1382 37268229298、Normal5205seed1393 37268232056、Game5205seed1394 37268234502每run一条、workflow success；只读artifact收集与配对验收仍在进行。本新observer助手Actions待跑，未用于已完成S19。

## 问题、排查与风险

S19 seed1392完整300s：C2S2.999896M、字节loss0；S2C2.950594M、loss1.643461%。双端释放/同lease恢复可见；client恢复成功3/3、failed0。61.6s关闭后69.6s发生额外唤醒，70.6s owner比静默前多4个160B内层业务数据报；105.6s又关闭，120/240s按计划重新建立。因为助手确未在静默发送，来源尚未知，不能误判外层keepalive触发；整体S19记PARTIAL_BACKGROUND_INPUT_UNATTRIBUTED，不能通过降低静默门写PASS。下行唤醒附近质量损失另保留。无探针所以p99 NOT_EVALUATED。

更新唯一STATUS与实际MTU9000说明，保留M03两份失败。native小诊断、测量、0退出/owned清理证据已归档；原pcap分析后删除，没有引入大抓包。

## 下一项原子任务

读完773四条small artifacts与配对尾延迟；observer Actions通过后S20纯下行，再附背景归属的S19复测，随后最大UDP精确序号和剩余33工况。若p99再次出现队列1s级等待，先用时间证据定位具体边界再窄修；不能扩大缓存或恢复HOL。
