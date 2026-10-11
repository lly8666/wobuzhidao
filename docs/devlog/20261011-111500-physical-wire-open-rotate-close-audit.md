# 双端抓包：建连/稳态/换代干净，主动断开外观待改

用户要求详细抓取程序建连、换lane、结束全流程。只观测原部署SOURCE `7fb98fab79834a351a1dbe04eebb207f66bea28b`，Windows正式客户端到LinuxARM。未部署adaptive新分支，未改产品、配置、全局sysctl、4096/FEC/MTU/恢复政策，未dispatch Actions。GUI原先已断开，采用同一active.json/portable目录和原SOURCE stdin控制器自动启动/停止；不冒充GUI按钮点击测试。首轮server冷重启；后续结束专项保持serverPID505875。

## 工况与捕获有效性

Normal1/FEC20:20、outer1400/实效inner1273、www.cisco.com、routeall、rotate5..10m、双方idle2m、keepalive15s、diagnostic off。实际tls-startup-padding=true，同hash未变；此前聊天说明中的paddingoff是记录错误，本次核对纠正。本UDP人工负载不触发内层TLS识别，不能验收该填充功能。

600.0007s、双向各0.02Mbps，96/256/512/1000/1372B混合UDP与100ms周期探针。这是外观专项，不是10Mbps容量或多业务并发NoHOL资格；不把固定探针节奏当产品keepalive指纹。首人工加密负载前十多秒属于目标/夹具准备，不当产品握手耗时。

snaplen1600保存完整外层包，每phase12MiB/800s以内，Windows重叠7.627s、ARM14.364s，精确timestamp+frame去重；合并每端<=16MiB。Windows58781/ARM58783帧，各phasecapture drop0/truncation0。IP/TCPchecksum错误0、IPfragment0、maxIP1400B。复用既有p2/physical_wire_audit读器，现场捕获器为有界diagnostic helper，hash保留，不是新产品或Actions资格。

tail supervisor遇到状态JSONExtraData；原收集器因重叠phase的capture.stop已存在而中断。原capture handles未重启，闭合pcap/target仍在，SCP补取同一run分析；HARNESS_RECOVERED保留，不算产品FAIL。离线扫描过慢被停止并换成索引，未改变远端测量。没有重新生成好样本。

## 实际结果

业务2318/2318双向全收，坏payload0；probe5948/5948，p99=109.077ms，max189.304ms，>1s0。CPU仅稀疏计时client16.984CPU-s/server8.8CPU-s，不推广10M余量。server raw中途定点skmem.d0、rb1048576；原660s resource watcher在专项收尾取消，不冒充全窗资源最大值PASS。

初建三次握手Windows点60.026ms；换代新连接85.474ms，SYN/ACK序号正确、ACK在已发送范围内、RST0。MSS1360/SACK/WS8正确；clientTTL128、serverTTL64。未协商timestamp不判错。稳态payload与纯ACK均window65535；bootstrap1020..1024是阶段窗口，不误报为稳态跳变。

两lane各方向均完整可重组TLS字节序列，Windows独特record合计35171，unaligned/partial-tail/invalid-header/overlap-conflict0。ClientHello body656B，SNIwww.cisco.com，ALPN offerh2/http1.1，Firefox120 JA3=b5001237acdf006056b409cc433726b0。ServerHelloTLS1.3/0x1301AES128-GCM，扩展43/51；后续外层17/0303。ServerHello后23/448/96/53/139B加密record长度序列，不能不解密就对应证书/票据/ALPN消息。

C2S32974个packet都在server找到。server25809/Windows25807，两个原包269.865s/466.974s缺于Windows点，121.095ms/113.195ms后同Seq同payload重发，密文字节冲突0、业务0损失。捕获drop0支持此观测，不定位互联网中哪台设备丢弃；不是原80秒初始中断。

548.95s换代40000→40001：新SYN/真实TLS/双向加密data先发生，旧lane再双方FIN/ACK。Windows重叠452.985ms、ARM388.950ms，旧双方FIN都有正确ACK；server先FIN不异常，说明已确认新lane。本样本支持A→A+B→B，不替代弱网candidateFAIL/并发门。

## 主动断开专项

首轮clientFIN得到ACK，但12s尾窗只有40B加密下行，无serverFIN。追加同配置短连接/10s业务/断开后连续180s抓包：39/39业务双向全收，100/100probe；ARM点clientFIN59.870s、server立即ACK、serverFIN153.670s，**相差93.800s**。serveridle2m，源码Tick在payloadidle满足且PeerWriteClosed后Dormant，时间与此相符。

迟到serverFIN在ARM物理接口出现，Windows未观察到；另外7条晚S2C payload也未在Windows捕获，最终ACK无。全程capture drop0、client退出/清理成功、无RST。晚包在哪个网络/过滤边界消失尚未定位，不能直接说互联网黑洞或客户端丢包。当前是单次best-effort clientFIN、早退RX、server沿idle迟后收尾；**功能断开成功，不等于完整及时的四次挥手**。

## TLS-like诚实边界与修改建议

- 只判本SOURCE本工况建连/稳态格式/换代结构通过，不给99%像Cisco或不可检测百分数。Firefox120 preset、本地GoTLS固定serverpolicy不是Cisco完整指纹；源码NextProtos=nil、真实ticket启用但拒绝resumption，每lane完整握手。
- 当前握手AES128-GCM、稳态exporter派生ChaCha20-Poly1305+protectedPN，是独立TLS-like封装而非标准TLS1.3按序记录加密。被动随机密文不能直接读出算法变换，仍不能称标准TLS互操作PASS；新分支cipher适配另验。
- 原tls.Conn writer在handoff主动detach，不能接回来补KeyUpdate/ticket/close_notify，否则破坏序列/密文ownership。票据、EncryptedExtensions、closure alert在加密区；无traffic secrets不解密，不把看不到NST当缺失。源码ticket策略与抓包可见性分别记录。
- **P1及时lane收尾**：accepted权威peerFIN后停止该closing lane新增health，复用CloseWrite/TransportCloseComplete/有界退休发送自身FIN。client显式退出留极短有界RX grace完成控制ACK，再清理；candidate/其它Game lane继续fresh，不等2m/无限ACK，不加可靠排序或扩大重传窗，不把保活丢失当idle。Actions覆盖explicitexit、idle/wake、rotation、Game其它lane、candidateFAIL、FIN loss/reorder/duplicate/oldgeneration；性能仍一Action一条。
- **P2建连轻量一致性**：核当前开发分支cipher前后一致，必要时受保护Admission一次性长度变化/维护浏览器模板；不用假h2、稳态凑包等待或cover洪泛。
- **P3可选结束提示**：只在有证据时用一次有界受保护控制；不重接旧TLS writer、不伪造明文TLS1.3alert、不把自有keys声明标准TLS兼容。交付/CPU优先。

标准对照：[TCP关闭](https://www.rfc-editor.org/rfc/rfc9293.html#section-3.6)、[TLS1.3 closure alerts](https://www.rfc-editor.org/rfc/rfc8446.html#section-6.1)。用于解释差异，不恢复严格TCP/HOL。

## 收尾

两台远程机器ownedpcap、编译tmp、目标198.18.0.1、SYSTEM任务和所有observer/load已清理；network-state/ownedNRPT/firewall0。server505875仍active、同hash；GUI已恢复但未连接，需点连接继续试用。Git只保存数值/header摘要/hash，不上传raw、ticket/keys/凭据/完整配置/业务正文；capture filter的家庭公网地址也移除。

**本机raw清理未完成**：自动审批两次拒绝删除，先为限定目录批量命令，再为明确literal文件路径的非递归命令，均只返回blocked by policy而无详细原因。没有换壳/用其它方式绕过。12份本机raw总计71792885B保留在两份owned证据目录，完整hashinventory已存，未上传Git；须后续清理。记录LOCAL_BLOCKED，不写全部raw删除PASS。

证据 `docs/evidence/wire-lifecycle-7fb98fab-physical-20261011.json`。结果PASS_SCOPED_OPEN_STEADY_ROTATION_WITH_EXPLICIT_CLOSE_APPEARANCE_GAP；项目IN_PROGRESS，主动结束外观OPEN，原b4约80s故障OPEN，不继承到adaptive新SOURCE，不改当前FEC next_task。
