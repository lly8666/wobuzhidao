# Linux 服务端部署与多客户端

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

首次install只安装，必须先编辑/etc/wbd/server.json再check/start。配置模板里的server-name、raw-interface、listen-ip、decoy、route-key-hex、username/password、tls-cert/tls-key均需替换。证书路径相对于配置文件目录；证书/私钥由部署者提供，不在线生成冒用证书。监听地址须属于指定物理接口，监听端口须由WBD独占；内核现有HTTPS服务不得占用同一地址端口。租约池不能与宿主LAN、其他VPN或路由重叠。不要写lease4/account/tunnel-id来启用默认随机分配；max-clients默认256。check只校验配置和证书读取，不证明实际外网连通、上游防火墙、NIC或decoy可达。

start先只读校验，再启用开机启动及启动服务。Type=notify在网络与正式服务循环就绪后才报告active；fatal运行错误非零退出，systemd有限速地重启。停止用wbdctl stop/restart；recover只读网络恢复记录清理WBD所有权，不访问客户端IP租约。当前机制不提供跨进程无损热升级，升级会短暂重连，应用层自行恢复。自动客户端IP重新分配后Windows GUI清理旧网络状态并重启客户端；直接CLI会报告WBD_CLIENT_LEASE_CHANGED并非零退出，由调用者/服务管理器重启，不静默更改运行中owner。

```sh
sudo wbdctl upgrade --package WBD-Linux-amd64.zip --sha256 <发布的ZIP摘要>
sudo wbdctl rollback
sudo wbdctl uninstall
```

upgrade先核验包内哈希、平台、源码/版本及现有配置兼容性，停止并清理旧服务后切换目录；启动失败恢复旧二进制和旧unit。install拒绝替代不同的已安装版本，必须走upgrade。rollback显式切回previous。uninstall移除服务和WBD网络设置，保留配置、证书、历史包及管理工具。--version只接受手动指定且已经具有Linux资产的GitHub发布标签，不自动追踪latest。不要从外部粘贴与当前版本不配套的管理脚本。

## 租约期限的精确定义

期限从上次通过认证的自动建连算起7天，换lane/重连会续期；纯业务包不逐包更新租约表。到期只在新认证分配时回收已经安全DORMANT且无active/retiring lane的owner，在线地址不会按钟强行断开。同设备仍在表内的重连沿用地址；服务端进程重启立即丢失内存映射，允许重新分配，既有应用会话不承诺跨重启存活。系统网络恢复journal保存路由/防火墙/sysctl所有权，客户端InstallationID保存设备身份，两者都不是IP持久化。租约管理不进入steady record/FEC/ACK热路径。
