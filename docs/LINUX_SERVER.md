# Linux 服务端部署与多客户端

2026-10-05最新原生五分钟进展：固定6181db6未改产品；S01 Normal10与S02 Game4×3达到目标附近但C2S缺74/10包，不能写无损PASS；M01外层MTU1400大包至9000B无坏数据但有1次迟到。D01默认NRPT+10M出现约90秒双向中断、约30.12%业务loss和8/60 DNS失败，明确FAIL；整体P7仍PARTIAL。优先按STATUS.physical_5min复现D01并异常触发抓包定位最早边界，不直接归因DNS/VM或扩大buffer/FEC/4096。完整DNS互备、LAN/CN/IP/IPv6、其他配置/生命周期/弱网未跑。方案PHYSICAL_5MIN_ACCEPTANCE.md、日志devlog/20261005-021317-five-minute-native-capture-matrix.md、evidence/physical-5min-6181db6-20261005.json及压缩原始计数为当前证据入口；每性能Action只一条。原始pcap已删、退出owned清理通过，服务端保留active。

## 当前只做简单部署测试（用户最新要求）

不开发通用安装器、Go版wbdctl、在线升级或新的回滚管理。直接用固定发布包：核验包与manifest，解压复制已编译的wbd-server，填写server.json和证书，检查配置后启动。现有wbdctl及其Actions记录保留为历史能力，不作为当前测试部署入口；Python3.8安装工具错误保留记录，但不阻塞直接部署，也不因此重新打包或改变数据面。

测试服务器使用/opt/wbd/wbd-server、/etc/wbd/server.json与证书、/var/lib/wbd/network-state.json和/run/wbd。包内systemd unit仅将/opt/wbd/current/wbd-server改为/opt/wbd/wbd-server，其余就绪通知和owned网络清理保持。先不启用开机自启；手动替换程序前停止服务，不自动联网升级。配置、认证密钥不写仓库。分别操作如下：

```sh
# 启动前检查
/opt/wbd/wbd-server --config /etc/wbd/server.json --check-config
systemctl start wbd-server
# 查看运行情况
systemctl status wbd-server --no-pager
journalctl -u wbd-server -n 50 --no-pager
# 需要停止时执行
systemctl stop wbd-server
```

无需在服务器安装Go，也不调用Python安装工具。首次配置仍需实际接口/本机IPv4、监听端口、SNI/decoy、认证、证书和不冲突的地址池；架构匹配、管理连接与其他服务保留仍需确认。

2026-10-05：固定6181db6的原生Windows CLI/Npcap/Wintun→ARM WAN业务已验。Normal1双向10Mbps两份完整120秒、Game4双向3Mbps一份120秒均跑满、业务loss0，p95约72/69ms；两模式受控DNS/UDP/TCP/验证证书HTTPS及退出清理通过。首轮120秒未取得客户端完整计数、服务端单向约4.16Mbps，另一次fresh Normal AF_PACKET drops+125仍待定位，不能宣称长期稳定或整个P7通过。人为弱网、GUI实际操作、长测未验。见[本轮日志](devlog/20261005-014639-native-wan-no-pcap.md)和[含原始小回执的证据](evidence/physical-native-6181db6-20261005.json)。

连接工具使用本机WSL OpenSSH，测试在两个远端原生系统执行，不在WSL内承载业务。Windows vmxnet3虚拟NIC，ARM Ubuntu20.04/aarch64，不能称裸机NIC成绩。客户端应用文件、日志和临时文件留便携目录；已断开并恢复路由/DNS，移除临时任务和测试地址，ARM服务保留active、autostart disabled。未生成pcap/pcapng/etl，也不保留HTTPS响应正文。

本轮新增tools/physical_*测试助手，使用方式与失败保留规则见本轮日志；它们不是安装器，不打进产品包。正式产品源码和发布二进制SOURCE不变，不因此重新发布。

## 历史预检与已发布工具

此前预检中6181db6 ARM包10文件hash及原生`--version`PASS；Ubuntu20.04/Python3.8的wbdctl.stage因Path.is_relative_to缺失FAIL。旧服务按用户指令卸载并保留root专属备份；旧进程、自启、owned规则和TUN清理。该问题属于可选管理工具，按最新简化部署决策不再是当前待修任务。失败证据保留在[预检日志](devlog/20261004-213700-physical-preflight-legacy-uninstall.md)。

当前固定预发布SOURCE `6181db66b67594b07cd989b8b8b5848cedf6ccc3`：[配套Windows/Linux下载](https://github.com/lly8666/wobuzhidao/releases/tag/linux-server-rc-20261004-6181db6)。amd64真实部署12检查、native多客户端及正常/强杀重建12检查、GUI208、基础/race、36生命周期与两个独立5205全部PASS，详见[evidence/linux-server-6181db6.json](evidence/linux-server-6181db6.json)。下面较早SOURCE记录只是历史过程，不能替代这个固定证据。旧985不含自动租约，c956不含新增客户端强杀网络恢复，推荐本次两端配套。ARM包在Actions交叉构建，已新增用户ARM原生部署与上述Windows CLI端到端业务/短性能结果；整体P7仍PARTIAL，最新full70/strict18/1800s仍NOT_RUN。

本任务由2026-10-04用户授权继续实施。最新决策：认证保持简单的一组用户名/密码，允许多台设备共用；每设备独立InstallationID，由服务端内存租约池随机选择未占用IPv4地址。租约7天，认证重连续期，活跃/退休lane仍占用的地址不能回收。不持久化IP映射，服务端重启后允许重新分配；只持久化客户端安装身份。

## 现有与新增边界

复用当前真实TLS/protected admission、共享TUN、多TunnelOwner、源地址隔离和Normal/Game生命周期。自动分配通过已有受保护建连消息的WBAL扩展请求携带InstallationID及1..4lane数量，回复追加IPv4。V2记录加密/FEC/ACK/恢复数据面不变；旧WBAD静态入口原字节不变，自动客户端对不支持扩展的服务端明确失败，不静默退回手填地址。账号与InstallationID决定稳定TunnelID，安装ID不是额外认证密码。

服务端省略lease4进入自动模式，默认允许每客户端1..4lane；max-clients默认256、最大4096（身份/地址容量，不是带宽承诺）。显式lease4保留已测静态部署。每台自动客户端可分别选择Normal1或Game2/3/4。用户名密码共用不代表身份共用，复制另一台设备的完整安装身份会被视为同一设备，不允许据此承诺恶意同账号互相隔离。

客户端省略lease4、account和tunnel-id即可自动分配；account取username，TunnelID由InstallationID派生。GUI新profile已经生成安装ID；CLI省略时Windows存于便携data目录，Linux存于/var/lib/wbd-client/installation-id。serverIP/SNI/route-key等现有建连配置仍必需，未新增公开握手或常驻管理进程。Windows网络地址在第一条真实受保护回复成功后、创建TUN及配置路由前确定。后续lane和换代回复必须与当前地址一致；重启服务端重新分配需客户端重新建立逻辑owner，不允许静默改活跃owner地址。

## 部署工作顺序

1. 多客户端自动租约及双平台客户端接线；Actions单测/race、同账号多客户端Normal/Game并存、源地址隔离、换代稳定、7天续期/到期/活跃保护、池耗尽及错误密码不分配。SOURCEe0973219基础/race与双向隔离、候选保护、DORMANT重建错误通过；GUI208检查通过。SOURCEd8a40176完整36生命周期通过。正式Linux raw/TPROXY多客户端及重新分配后业务恢复正在新增验收。
2. 服务端统一退出/清理/非零错误码、配置只读校验；systemd就绪与单实例、网络所有权持久恢复、窄范围RST抑制，保留foreign规则。SOURCEd8a40176 Actions37189866577实测通过。
3. 安装/配置模板/管理工具、手动指定版本升级/回滚/卸载保留配置；amd64/arm64包与Actions真实systemd/网络安装生命周期。SOURCEd8a40176 Actions37189866577通过；arm64仅交叉构建。

应用建议目录为/opt/wbd/releases/<version>、/opt/wbd/current、/etc/wbd配置与证书、/var/lib/wbd网络恢复记录（不存IP租约）、/run/wbd锁；普通日志交journald。不做在线账户面板、不新增HTTP管理端口、不擅自热加载FEC/MTU/身份、不并行启动两个服务争抢raw/TUN。

## 验收

所有编译/测试/格式化在Actions。功能unit/race可集中；性能一个Action一条样本。安装重复执行、配置拒绝、启动部分失败、SIGTERM/SIGKILL、规则恢复、升级失败回滚、原配置保留以及实际正式入口双向业务须分别记录；ARM交叉构建不是原生运行、物理P7仍NOT_RUN。IP租约不用写磁盘；网络恢复日志是不同对象，仍需要持久化才能恢复进程崩溃前的系统修改。

## 安装、配置与日常操作

目前明确验证的系统是Ubuntu24.04 amd64、systemd、Python3、iproute2、iptables/nftables、可用/dev/net/tun。其他发行版需满足相同条件并单独验证，OpenWrt/procd不在本部署壳范围。Linux包通过next-linux-server的exact-SHA artifact取得；旧Windows GUI预发布9857bdb没有本轮自动地址扩展，客户端/服务端须使用本轮配套包。没有新发布标签时勿对旧标签使用--version期待下载Linux包。

解压WBD-Linux-amd64.zip（ARM用arm64），运行包内工具安装，工具随后位于/usr/local/bin/wbdctl：

```sh
sudo python3 linux-amd64/wbdctl install --package WBD-Linux-amd64.zip
sudo chmod 600 /etc/wbd/server.json /etc/wbd/server.key
sudo wbdctl check
sudo wbdctl start
sudo wbdctl status
sudo wbdctl logs
```

首次install只安装，必须先编辑/etc/wbd/server.json再check/start。配置模板里的server-name、raw-interface、listen-ip、decoy、route-key-hex、username/password、tls-cert/tls-key均需替换。Linux正式入口的 `raw-recv-buffer` 默认524288，含义是AF_PACKET `SO_RCVBUF` 的请求字节数；0继承系统默认。启动日志必须以 `WBD_RAW_RCVBUF` 的effective/limited字段为准，Linux通常读回约请求值两倍，但受 `net.core.rmem_max` 限制。程序不改sysctl、不使用SO_RCVBUFFORCE；受限不是“1MiB已生效”，回滚设0并重启即可。证书路径相对于配置文件目录；证书/私钥由部署者提供，不在线生成冒用证书。监听地址须属于指定物理接口，监听端口须由WBD独占；内核现有HTTPS服务不得占用同一地址端口。租约池不能与宿主LAN、其他VPN或路由重叠。不要写lease4/account/tunnel-id来启用默认随机分配；max-clients默认256。check只校验配置和证书读取，不证明实际外网连通、上游防火墙、NIC或decoy可达。

start先只读校验，再启用开机启动及启动服务。Type=notify在网络与正式服务循环就绪后才报告active；fatal运行错误非零退出，systemd有限速地重启。停止用wbdctl stop/restart；recover只读网络恢复记录清理WBD所有权，不访问客户端IP租约。当前机制不提供跨进程无损热升级，升级会短暂重连，应用层自行恢复。自动客户端IP重新分配后Windows GUI清理旧网络状态并重启客户端；直接CLI会报告WBD_CLIENT_LEASE_CHANGED并非零退出，由调用者/服务管理器重启，不静默更改运行中owner。

```sh
sudo wbdctl upgrade --package WBD-Linux-amd64.zip --sha256 <发布的ZIP摘要>
sudo wbdctl rollback
sudo wbdctl uninstall
```

upgrade先核验包内哈希、平台、源码/版本及现有配置兼容性，停止并清理旧服务后切换目录；启动失败恢复旧二进制和旧unit。install拒绝替代不同的已安装版本，必须走upgrade。rollback显式切回previous。uninstall移除服务和WBD网络设置，保留配置、证书、历史包及管理工具。--version只接受手动指定且已经具有Linux资产的GitHub发布标签，不自动追踪latest。不要从外部粘贴与当前版本不配套的管理脚本。

## 租约期限的精确定义

期限从上次通过认证的自动建连算起7天，换lane/重连会续期；纯业务包不逐包更新租约表。只在新认证分配时检查到期项，DORMANT且无lane可回收；客户端明确全lane FIN，或全lane在至少max(90秒,3个server keepalive间隔)内没有认证记录，也可先安全关闭旧owner再回收。连第一条认证记录都没到的lane用其创建时间计龄；任意近期有认证数据的lane或仍retiring的owner继续保护地址。该清理只由7天过期项分配触发，不把missing keepalive改成普通业务空闲，也不修改原idle-dormant策略。在线地址不会按钟强行断开。同设备仍在表内的重连沿用地址；服务端进程重启立即丢失内存映射，允许重新分配，既有应用会话不承诺跨重启存活。系统网络恢复journal保存路由/防火墙/sysctl所有权，客户端InstallationID保存设备身份，两者都不是IP持久化。租约管理不进入steady record/FEC/ACK热路径。

客户端显式停止时每条owned lane尽力发送一次FIN，不等ACK、不因丢包延迟清理。Normal/Game更换需要先停止旧客户端；服务器只在旧全lane明确关闭或已长期失活时重建模式，沿用同一设备/IP租约。仍在线/换代中的模式变化拒绝，弱网FIN遗失时可能需等到旧lane失活再重试，不强杀仍有认证业务的旧客户端。

客户端异常退出且没有FIN时，原端口复用的SYN也按同一全lane失活条件处理：max(90秒,3个server keepalive间隔)前仍保护旧owner，之后后台安全detach并接受原握手重试；不回收未到期的7天地址。无需改4096、MTU或重传模式，不改变普通业务idle逻辑。

Linux/OpenWrt正式CLI在/run/wbd-client按kernel network namespace持有独占锁，并记录boot/namespace、canonical网络参数和随机NFT所有权标记；这是TPROXY系统网络恢复journal，不是IP租约缓存。SIGKILL释放锁，新进程先核对旧owned IPv4/IPv6 route/rule和NFT marker，再恢复自己的残留网络状态，按新配置启动；健康旧进程仍运行时拒绝重复启动，foreign状态不匹配时不自动删。正常停止删除journal并恢复自己的规则。只有带新journal的本版本残留可自动恢复，旧版本无journal的未知残留仍保持拒绝接管。

共享TUN内层MTU使用既有lease合法IPv4包上限9000；配置`mtu`是外层连接预算，不能通过修改它扩大内层缓冲或发超路径的大外层包。升级前正常停止并清理旧owned网络，重新启动建立正确TUN；旧journal按原值恢复仍支持。候选是否实际通过见STATUS.server_inner_mtu_separation。
