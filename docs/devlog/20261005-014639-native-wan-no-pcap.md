# 20261005-014639 原生Windows到ARM业务与短性能，无原始抓包文件

## 本轮目标和阶段

开始HEAD d5c8c125dbe66ea7e68ad58bfa5df85d311e4553，目标分支next/tlslike-dataplane。用户明确授权继续搭环境跑实际测试，允许本机WSL作为连接工具，并要求避免或清理大抓包文件。当前P7局部原生验收；部署仍仅固定包解压、配置和启动，不扩建安装/更新管理。

两端产品SOURCE始终6181db66b67594b07cd989b8b8b5848cedf6ccc3，预发布linux-server-rc-20261004-6181db6。ARM包sha256 c81edf4a3f724918c0ae2eef4baa24457d7b7074bd9516330ed2e23cce0f8fe4；Windows包sha256 487f853904318216fa0170063f29df0dc7b2e0bea5aa37c3217bf49e5864e38a。manifest分别10/13文件与版本已验。不改产品Go代码、wire、FEC、4096、恢复策略、buffer或发布包。

## 修改与原因

新增六个可复用测试助手，不进入发布包：

- physical_windows_session.ps1：SYSTEM隐藏任务拥有真实CLI，stdin stop或有界watchdog正常退出；状态、CPU/RSS及前后网络快照写本目录，日志约2MiB上限。清理超时保留进程排查，不强杀。
- physical_windows_business.ps1：受控DNS精确字节、TCP echo、使用明确CA实际验证HTTPS和102400字节正文hash；正文临时文件finally删除。
- physical_udp_server.py、physical_udp_client.cs、physical_windows_udp.ps1：独立双向定速混合UDP，有界计数/去重/完整性/RTT探针、产品与夹具CPU分开；结果不保存业务字节。客户端测量通过真实Windows .NET socket，Add-Type仅编译测试助手，不编译产品。首版summary timeout会抛异常丢客户端计数，后续改为保留计数并可通过SSH读独立服务端JSON，volatile字段及SummaryReceived/ProbeSent明确跨线程观测；这不是产品性能修复，也不能证明首轮异常原因。
- physical_packet_counts.py：AF_PACKET只读短头部并聚合，最多15秒，不落原始包/载荷。实际8秒只观察到C2S，不能宣称双向完整抓包资格。

STATUS、AGENTS、LINUX_SERVER、WINDOWS_GUI、ROADMAP与ACCEPTANCE同步真实范围；evidence/physical-native-6181db6-20261005.json保存全部小回执、原文件hash和未解问题，不保存配置/凭据/证书私钥。源计数本地目录D:/codex/wbd/evidence-physical-20261005，仓库JSON内嵌原始小回执，方便全新agent复核。

## 复用来源

复用当前配套发布二进制及tools/config_business.py受控业务目标，无old源码移植，无参数改变，REUSE_LEDGER不变。WSL Ubuntu22.04仅OpenSSH连接两个远端，业务不经过WSL代理。

## 环境和测试口径

Windows11Pro/8逻辑CPU/vmxnet3虚拟NIC；ARM Ubuntu20.04.4/Linux5.4/aarch64/2CPU/Neoverse N1。属于原生系统/驱动与实际WAN测试，Windows不是裸机NIC。Npcap运行，真实Wintun已创建/使用。同源CLI每次正常断开后切Normal/Game，不测试GUI实际点击或跨服务器切换。

FEC20:20、MTU1400、startup padding off；Normal1每方向10Mbps、Game4总有效业务每方向3Mbps，复制不计goodput。UDP payload按96/256/512/1000/1372字节循环，最大完整IPv4包1400。120秒发送+3秒接收尾窗；RTT为隧道UDP探针，不是600ms人工RTT。未注入人工丢包/延迟，不能继承为5205或其他弱网资格。

ARM临时lo地址198.18.0.1/32作受控目标，Windows显式route-mode=all迫使私网目标走隧道；管理LAN192.168.0.0/18及server /32直连保留。目标实际记录源为服务端分配lease10.66.109.1，Normal→Game→新Normal仍同lease。测试业务socket请求2MiB receive buffer，ARM实际425984B，仅夹具，与产品AF_PACKET212992B是不同对象，未调产品参数。

## Actions证据

本轮没有新Actions性能或产品测试；实际产品SOURCE资格仍见evidence/linux-server-6181db6.json。文档/助手HEAD不是产品SOURCE。后续任何产品修改先跑Actions正确性、独立Normal/Game5205，每性能Action只一条样本；不得在同run串行A/B或matrix。实机本轮分别独立发送窗口，不与hosted成绩混并。最新SOURCE全70/严格18/1800s仍NOT_RUN。

### 助手复用操作

先核验固定包/manifest/原生version，再部署助手；不得只因脚本SourceSHA写6181就把其他二进制测量登记为6181。Windows把physical_*.ps1与physical_udp_client.cs放在便携bundle的上一层，已检查的配置放bundle/data/test-config.json。用SYSTEM隐藏scheduled task调用physical_windows_session.ps1 -Bundle <bundle> -MaxSeconds <上限>，待data/p7-status.json RUNNING且Ready后发送业务，结束创建data/p7-stop.request并确认STOPPED/Exit0及network snapshots；每次切换模式前先正常退出。

ARM只有临时受控地址启用时运行python3 physical_udp_server.py --bind 198.18.0.1 --wbd-pid <实际serverPID> --output <单样本JSON>，Windows调用physical_windows_udp.ps1 -Bundle <bundle> -Mbps 10 -Seconds 120 -Seed <独立seed> -Name <独立名称>；Game4将Mbps改3，同时真实产品配置lanes4。每次服务端启动一个目标进程，客户端运行一个样本；保存双端JSON后目标进程退出。夹具仅这两档/120s已执行，高速或更长时间需重新审计1M去重bitmap边界，不能照参数最大值外推资格。

业务检查使用当前tools/config_business.py受控DNS/TCP/HTTPS目标及单次CA，端口15353/18444/18443；physical_windows_business.ps1 -Bundle <bundle>验证上述响应，不把私网直连当隧道。头部统计工具运行参数见其argparse，只保留JSON计数。产品raw socket另用ss -apnm读取每样本前后，由serverPID/fd7识别，禁止用整机累计drop冒充本样本差值。业务JSON未取得服务端summary时先保留客户端计数，再SSH读取独立目标JSON，不重写成PASS。

## 原生实测结果

| 场景 | 时长 | C2S/S2C goodput Mbps | 双向业务loss | RTT p95/p99 ms | 客户端/服务端CPU-s | server AF_PACKET drop增量 |
| --- | --- | --- | --- | --- | --- | --- |
| 首轮Normal10 seed1001 | 120s | 4.1599/未知 | 未知 | 未取得 | 未取得/42.35 | 缺before，不能算 |
| 定向Normal10 seed1002 | 30s | 9.99990/9.99990 | 0/0 | 75.64/84.06 | 26.27/16.78 | 未作独立完整门 |
| Normal10 seed1003 | 120s | 9.99973/9.99994 | 0/0 | 72.32/82.83 | 105.55/67.03 | 0 |
| Game4×3 seed1004 | 120s | 2.99990/2.99990 | 0/0 | 68.92/73.34 | 127.92/71.17 | 0 |
| 新进程Normal10 seed1005 | 120s | 9.99980/9.99994 | 0/0 | 72.43/84.47 | 108.00/68.62 | 125，仍有接收pressure |

第一份FAILED_MEASUREMENT_INCOMPLETE：客户端报Target summary timeout，未保存客户端发送/接收完整计数；独立目标仍记录Rx96414包/62398416B、Tx231768包/149999172B、bad0。不能计算应用loss，也不能说发送达标或单纯是取summary失败；保留原始回执，不以复跑成功抹掉失败。

三个完整120s发送均约100%目标，bad payload/duplicate均0，探针分别1189/1189、1192/1192、1189/1189；包与字节逐向计数全部对齐。Helper CPU另外记录，不混入产品CPU；约120秒下Normal Windows平均0.88–0.90核、ARM0.56–0.57核；Game Windows1.07核、ARM0.59核。不是CPU性能优化对比或最大容量测试。

两模式受控DNS/UDP精确回应、TCP精确echo、HTTPS证书验证及102400字节正文均PASS，正文SHA25627783e87963a4efb6829b531c9ba57b44f45797f6770bd637fbf0d807cbdbae0。系统NRPT解析www.cloudflare.com成功。这不独立证明双DNS故障互备。普通合法HTTPS直接指定WBD公开IP、SNI www.cloudflare.com访问/cdn-cgi/trace，默认验证证书、HTTP200/216B/tls_verify0/curlExit0，无insecure、正文不落盘；证明普通HTTPS fallback通路，不证明recognized WBD握手与Cloudflare完整指纹一致。

Game8秒头部统计实际四个源端口40000..40003，各约11280 data packets，观察socketdrop0、RST0、IPv4分片0、max IPv4 length1290。仅该时间窗C2S证明；未观察到S2C不能补写通过。混合payload1372业务完整到达证明当前MTU1400路径可用，不等于所有路径PMTU/全部MTU值已验。

## 清理和空间

客户端各会话STOPPED/Exit0，IPv4路由前后签名差异0；owned NRPT0、IPv6 firewall0、network-state不存在、WBD进程0。第一功能会话实际watchdog结束（不是人工stop）；后续Normal/Game/fresh均requested_stop。最终任务移除，data/tmp清空，测试证书/HTTPS正文/ARM lo地址移除，临时target进程结束。配置回到lanes1、bypass-lan-cn，管理直连保留；未实际重连验默认分流，不冒充该项原生资格。

未创建pcap/pcapng/etl文件；不删除其他agent历史文件。只保留几十KB JSON/txt计数与已安装包/程序。WSL SSH masters退出、临时app凭据副本与测试CA删除。正式服务端仍active且autostart disabled，现有V2Ray/frps、SSH/RDP未改变。Wintun系统驱动安装是用户接受的便携例外；程序配置/日志/临时文件均本目录。

## 问题、排查与风险

首轮异常原因未解。fresh Normal产品AF_PACKET累计6612→6737，而业务FEC后loss0；正常120s及Game两窗均6612→6612。源码internal/faketcp/raw_linux.go的ETH_P_IP socket绑定网卡，未在内核限定监听TCP端口，整NIC其他IPv4也会进接收队列；故125可能包含背景，但不能断定全是背景，更不能凭吞吐正常判capture-clean。

总体原生短时吞吐/完整性表现好，仍PARTIAL_NATIVE_WAN，不关闭P7，不发正式release。未验GUI实际操作/UAC/跨服务器切换、人为20/30%loss与600msRTT、原生rotation/DORMANT/黑洞、IPv6外部无泄漏、padding on、其他FEC档位、长测及完整双向外观。

## 下一项原子任务

先在固定6181db6独立Normal10/120s重现接收pressure与首轮异常：保留完整双端offered/unique/loss/RTT/CPU，读取产品socket逐秒Recv-Q/drop及系统CPU/网络压力，分离WBD端口与同NIC其他流量，继续只保留计数不存payload。证据若指向内核入队背景压力，评估Linux raw ingress在内核按实际监听IP/TCP端口过滤的最小优化；须同时保留普通TLS fallback、flags/分片边界及多lane/客户端语义，先Actions正确性和独立5205再发布/实机。未证明原因前不调大buffer/FEC/4096，不将失败归因虚拟机。诊断收口后依次原生GUI/生命周期/弱网，每次独立样本。
