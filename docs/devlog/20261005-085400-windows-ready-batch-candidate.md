# 20261005-085400 Windows 已就绪包批量发送候选

## 本轮目标和阶段

继续 P7 容量缺陷收口。开始 HEAD `8b456c641d96601c08046bc18e272cde48439e32`，产品仍部署固定8f53f33。上轮仅分离capture没有修复处理速度，本轮只减少 Windows Npcap 当前已就绪包的逐包发送调用。

## 修改与原因

`NpcapEndpoint.WriteSegments`接现有`SegmentIO.EmitBatch`/runtimeowner已就绪发送接口，每批最多8包、16KiB native scratch，不等待下一业务包、时间、ACK或累计Seq。Npcap可选sendqueue exports全部具备才启用，DLL分配失败/缺export退回原单包；稀疏单包、ACK/bootstrap/FIN/control仍直接发送。native queue由sendMu保护，incarnation gate覆盖完整调用，Close等所有活动调用结束再destroy，未引入第二个异步发包队列。

同一个原serializer计算frame/checksum/options/IPID，不改wire/FEC/Game/MTU/repair4096/3s期限/32ms修复相对新鲜包策略。sendqueue同步参数固定0，不用timestamp pacing/busy wait。回执按含16字节Windows pcap header的byte offset换算已消费前缀；短写/畸形回执立即报错，不将已发前缀或不确定后缀重发。既有runtimeowner处理已发备份与未发备份，不整包重试。

原诊断增加write_calls/batch_calls/batch_requested_packets、实际DLL调用wall time及sendMu等待total/max，默认off时不读时钟。它们用于验证真实driver路径，不将时间当CPU。无新用户配置，GUI字段全集不变。

新增跨平台serializer/顺序/IPID环绕/checksum/队列字节上界、稀疏立即发送、无exports回退、部分前缀不重发、错误flow整批预拒绝、oversize scratch单包回退测试；Windows ABI单测检查uint32+pointer布局及16字节pcap header。真实Npcap驱动在hosted不具备，不能用单测伪造驱动性能通过。

## 复用来源

本分支`runtimeowner/send_batch.go`、`faketcp/npcap.go`及现有Npcap gate/serializer。未读old提示词或替换成熟算法。[Npcap官方API](https://npcap.com/guide/npcap-api.html)说明sendqueue降低逐包调用的context switch；[libpcap实现](https://raw.githubusercontent.com/the-tcpdump-group/libpcap/master/pcap-npf.c)返回PacketSendPackets消费字节，queue布局见官方pcap.h。仅用接口实现，不复制其协议/旧架构。

## Actions证据

当前产品候选 NOT_TESTED。先push本提交，精确SOURCE core/race/Windows/GUI，再各独立Normal5205 seed1271与Game5205 seed1272，以及同源P6。不得部署未经门槛检查的包。

上一助手提交8b456c6的foundation37248426581、targeted37248426677、GUI37248426566、predelivery37248426514全部SUCCESS；它们不能作为本候选资格。

## 问题、排查与风险

8f D01 seed1342 profiler-on完整300s：C2S9.99998M、S2C8.56691M/14.33%字节损失，Windows真实OS计数339.14 CPU-s/300s。退出0/owned清理通过。只作为诊断，不是profiler-off重复基线。

profile持续344.86s、总sample权重1123.51s，其中等待stdin的ReadFile也约343s，NpcapRead约328s。因此不能写“cgocall消耗88%CPU”，更不能说产品用了3.26核。Windows Go1.23定期暂停线程取stack，外部阻塞也会入样，官方runtime/os_windows.go可核对。WriteSegment在sample中约231.56s，接收handleSegment中sendACK约101.79s，表明同步发送路径值得减调用/验证，但仍不等于driver真实CPU定量结论。FEC/crypto不是此profile中占主导的Go热点。

证据 [windows-native-profile-8f53f33-20261005.json](../evidence/windows-native-profile-8f53f33-20261005.json)、131KiB原profile和压缩小回执保留；不含业务正文/凭据。43case已执行7个唯一项、11份完整300s样本，36项未跑，跨SOURCE成绩不能继承。

风险：native batch可能使现有就绪包以burst发出，必须原生看实际batch触发、queue年龄/overflow、loss/RTT，不能仅凭调用次数减少标修复。16KiB是同步序列化scratch，不是扩大接收或重传库存；每最多10 incarnation有界。部分driver回执不是合法边界时已消费完整前缀计数保留，不确定尾部不重发，错误显式上报。

## 下一项原子任务

Actions及打包通过后同源Windows/ARM部署，D01至少一条300s先观察batch_calls/write time/send lock wait、driver与用户queue丢包。若仍有损失，依据新时间计数定位接收handler/外部写入等待，禁止盲增buffer/FEC/shadow或降低10M/20:20负载。改善后再独立重复、rotation和剩余配置/DNS/IP/MTU/idle，原始有界抓包分析后删除。
