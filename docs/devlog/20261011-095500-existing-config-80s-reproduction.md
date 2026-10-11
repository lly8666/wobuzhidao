# 现有物理配置复测：约80秒下行故障未复现，原故障保持OPEN

用户2026-10-11明确要求使用已经搭好的两端与现有配置复现，覆盖本轮开发方案“最终阶段才物理”的默认限制，仅针对既有试用部署；未部署新源码、修改共享产品或打断GUI连接。Windows→Linux ARM64产品两端均为 `7fb98fab79834a351a1dbe04eebb207f66bea28b`，不是adaptive最新83eca5e，更不是原b4。

## 配置和测量范围

GUI真实活动配置是 `active.json`：Normal1、FEC20:20、outer1400、实效inner1273、route all、rotate5m..10m、idle2m、keepalive15s、diagnostic关闭。预检查曾读到备用test-config的bypass-lan，随后读回活动配置修正；没有把备用文件作为真实生效证据。服务raw rb=1048576，进程与服务端配置hash前后相同，GUI/活动配置hash在测量中及结束后相同。当前活动PID保持。

复用精确7fb SOURCE的五份native MTU/metadata helper，字节经Git换行归一化逐个校验。外部wrapper只启动有界目标/观察器与调用既有C#探针，不改产品配置。临时目标198.18.0.1:18446确实走WBD-Trial，ARM临时loopback地址由本任务创建并撤销。请求按原M03 maximumUDP序列串行、最多10次attempt/s，时间300.100897s、尾排空3.070328s。稀疏UDP echo功能测量，非10Mbps吞吐、TCP负载或并发NoHOL资格；真实成功业务payload约0.384Mbps/方向。

## 结果

成功发送1879包，目标1879包且全部echo成功，客户端1879包，missing=0、bad/invalid=0、receive errors=0。96B 1366/1366；8972B、8973B、65507B各171/171。全部返回p99=102.164ms、max=273.391ms；96B p99=61.462ms；65507B p99=104.073ms、max=105.411ms；超过1秒late=0。65508B越过UDP合法上限，以及大包DF禁止分片的API拒绝分别计MessageSize，均未算成网络丢包。

观测开始约45.704s见新SYN/40031，旧40030随后FIN；新lane与旧lane两个方向payload计数均持续增加，新lane持续业务超过80秒仍正常。仅证明本次没有原来匹配NIC S2C计数冻结和业务缺78包的表现；观测有界、不是逐包证明A+B→B顺序。

server product raw累计drop从0到0、rb1MiB；独立ARM header observer漏39包，Windows observer漏0，捕获漏包单列，不能宣称NIC逐包无损。默认诊断off，因此没有新的FreshSent/Emit对应计数证据。客户端CPU9.6875s、helper0.75s；server325s资源窗CPU增量4.49s，仅记录，不下吞吐性能优化结论。

## 结论、限制与收尾

`PASS_SCOPED_CURRENT_CONFIG_80S_NOT_REPRODUCED`。本次开始时客户端已经连接，期间发生自然换lane，**未重启程序做冷建连**。原b4使用inner9000，而本次1273；源码、网络时段和连接阶段都不同，所以原 `M03-1554-INITIAL-S2C` 仍OPEN；原Sequence676/65507B 1225.578ms迟到也不被本次好样本关闭。不能宣布根因修好。

无raw pcap生成；metadata/时序保留本机，GitHub仅紧凑summary、每文件size/hash。ARM owned目标/观察器退出、临时地址撤销，Windows owned编译tmp已清理；原GUI、client、server保持运行。证据 `docs/evidence/repro80-7fb98fab-current-config-20261011.json`。

后续若继续定位，仍需在不覆盖用户配置的前提下单列冷建连重复，再依据复现证据查emit tuple/syscall与内核OUTPUT/route/conntrack/qdisc。未复现不盲改FEC、timer、4096或扩大buffer。新adaptive N0–N6开发进度不受这条旧部署物理样本改变。
