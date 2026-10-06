# 原生探针分阶段延迟元数据与最大UDP重复失败

## 本轮目标和阶段

P7。产品继续固定9211b24，不改运输算法；本轮补齐DNS D04能按故障/恢复阶段核验p99的助手元数据，并保留两次最大UDP不同结果。

## 修改与原因

physical_udp_client.cs保留原12字节P7P探针、10Hz频率、业务发送预算/包长、socket设置、3200ms drain及原p95/p99计算。额外记录已收到探针的原SentTick及同一次原RTT读数，最多8192条，有溢出错误计数；记录起点QPC频率和客户端UTC用于对齐故障阶段。没有新增网络包、payload保存或产品配置，统计发生在助手接收端。助手需要Actions编译后才上传实机；不能拿修改前的助手hash冒充本版。

## 复用来源

现有原生UDP助手、predelivery-tools编译门；无old提取。

## Actions证据

本助手NOT_RUN，提交后Actions编译与相关core/GUI门。产品9211b24仍保留已有五条独立性能/18RTT对和37生命周期PASS；文档64148be foundation37406401382、targeted37406401169、GUI37406401308、preflight37406401226成功，不重跑相同产品性能或将文档HEAD当新产品。

## 问题、排查与风险

同产品原生M03：seed1415 PASS，2040/2040 exact/及时，最大UDP170/170，p99380.45ms/max564.96ms，server rawsocket drop0，receive69258；seed1416 FAIL，2028送/2027回，最大UDP169送/168回，seq1316迟到1308.20ms，seq1796未回，1352 small全准时。后者独立target收到并echo1796，目标echo89.56us；server rawsocket drop0、filter443 true、receive67854。不能拿第一条健康关闭故障，也不能把第二条说成runner接收能力崩溃。

服务端TUN1400使最大UDP每次echo产生48个IP片，9000UDP附近每次7片；seed1415 FragCreates11730、seed1416 11661与fixture数量匹配，ARM分片/重组失败0。Windows第二份完成675个重组而server676个分片datagram，剩余回程缺片/重复/内层校验尚待缩小；全机Reqds总量相同不能证明每个offset齐全。当前未改MTU或扩大缓存、FEC、4096，也不延长1s门。

## 下一项原子任务

先验本助手。同时为M03回程缺片设计只记录IPv4 ID/offset/校验与Windows drop reason的有界观测；已确认测试机内置Pktmon可用、当前未运行，但尚未启动抓包。原始大文件仅限定窗口/容量，审计后删除，不能录全机业务或凭未运行观测写根因。D04只在助手通过后执行，按故障/恢复阶段p99、默认NRPT和所选物理NIC普通DNS泄漏三条证据分别验；不能把DNS失败看成业务停顿。
