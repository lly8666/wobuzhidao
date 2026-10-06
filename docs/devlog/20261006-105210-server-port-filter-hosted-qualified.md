# 服务端接收端口过滤 hosted 收口与配套部署

## 本轮目标和阶段

P7尾延迟定位。固定产品/helper SOURCE 9211b24679ff97c46c20f69af47ed3f3e850b2ad，qualification/9211b24-server-port-filter-20261006。不把文档HEAD或父版本成绩当当前源码资格。

## 修改与原因

产品改动为此前窄BPF接收过滤，保留所有目的监听端口的peer/SYN/persona/TLS fallback；本轮修掉未使用测试导入后精确源码重验。过滤无关IPv4/TCP端口及UDP的入队/解析工作；不改发送、FEC、shadow4096、repair或HOL语义。实机部署新目录，旧7eeb二进制可回滚，配置/身份保留，未重装Npcap。

## 复用来源

当前raw adapter、成熟Actions和既有打包/原生助手；无old提取。

## Actions证据

Linux-server37404307413真AF_PACKET过滤/字节一致与默认端口兼容PASS，native多客户端12项及服务化12项PASS；core/race37404305850、foundation37404305990、GUI37404306007、lifecycle37404305849、startup37404337202、preflight37404337020 PASS。生命周期 fullstack37404554110共36样本+aggregate37/37 jobs PASS，P6 37405198748三个目标包和receipt门通过，Windows/ARM文件hash独立核验。

五条独立性能120s，FEC20:20、padding off、300ms单向，严格原门：
- run37404668426：strict-normal-lossless-seed1401-rate10-lanes1，min阶段goodput=9.999979Mbps，CPU client/server=47.14/47.13s per120s。
- run37404671232：strict-normal-5205-seed1401-rate10-lanes1，min阶段goodput=9.998645Mbps，CPU client/server=69.85/70.21s per120s。
- run37405155266：strict-game-lossless-seed1382-rate3-lanes4，min阶段goodput=2.999957Mbps，CPU client/server=85.63/81.15s per120s。
- run37405158213：strict-game-5205-seed1382-rate3-lanes4，min阶段goodput=2.999872Mbps，CPU client/server=69.94/65.21s per120s。
- run37405161026：strict-game-5305-seed1382-rate3-lanes4，min阶段goodput=2.999678Mbps，CPU client/server=98.37/93.62s per120s。

五分类全部PASS，18项同源同helper分阶段RTT对PASS，max p95增加10.954188ms，max p99增加11.297867ms；全部socketdrop0。十方向无send failure/skip，p99 send lag<=1.1ms；独立manifest/claim/助手hash/真实diag过滤receive_port443/kernel_port_filter=true均核验。不下载原始大pcap，仅小metadata/有界diag。跨VM CPU不推固定优化比例。

## 问题、排查与风险

d9b编译FAIL三个run保留，真内核门当时未执行；修正921才通过。收集四lane diag时原2MiB读取界过小，仅该只读metadata界放到16MiB、压缩单读取仍<2MiB，其余原2MiB及测试门不动。部署资格读取曾GitHub API超时，未进入远程变更；有限重试后继续，不归因产品。

此为scoped hosted资格，不是当前SOURCE全70/strict18/1800s/P7。旧7eeb maxUDP4迟到和旧rawp99/Wake loss未自动关闭；过滤去除了无关工作，但是否解决所有原生尾巴还未知。MTU原生控制器只在load外记录IP fragment/reassembly whole-host计数，不能直接归因为单tunnel；不修改正式负载/1s超时/100ms pacing。

## 下一项原子任务

配套921 timed M03 seed1415 300s，独立target逐事件时序/最大UDP档p99/小包及外层窗口审计，raw删除；随后D04双resolver故障、默认NRPT/物理DNS metadata泄漏观测/10M业务与p99恢复，ownedcleanup。每性能Action只一条，保留所有旧失败，不盲扩缓冲或恢复HOL。
