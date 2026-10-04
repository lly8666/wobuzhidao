# Linux 服务端部署与多客户端

本任务由2026-10-04用户授权继续实施。最新决策：认证保持简单的一组用户名/密码，允许多台设备共用；每设备独立InstallationID，由服务端内存租约池随机选择未占用IPv4地址。租约7天，认证重连续期，活跃/退休lane仍占用的地址不能回收。不持久化IP映射，服务端重启后允许重新分配；只持久化客户端安装身份。

## 现有与新增边界

复用当前真实TLS/protected admission、共享TUN、多TunnelOwner、源地址隔离和Normal/Game生命周期。自动分配通过已有受保护建连消息的WBAL扩展请求携带InstallationID及1..4lane数量，回复追加IPv4。V2记录加密/FEC/ACK/恢复数据面不变；旧WBAD静态入口原字节不变，自动客户端对不支持扩展的服务端明确失败，不静默退回手填地址。账号与InstallationID决定稳定TunnelID，安装ID不是额外认证密码。

服务端省略lease4进入自动模式，默认允许每客户端1..4lane；max-clients默认256、最大4096（身份/地址容量，不是带宽承诺）。显式lease4保留已测静态部署。每台自动客户端可分别选择Normal1或Game2/3/4。用户名密码共用不代表身份共用，复制另一台设备的完整安装身份会被视为同一设备，不允许据此承诺恶意同账号互相隔离。

客户端省略lease4、account和tunnel-id即可自动分配；account取username，TunnelID由InstallationID派生。GUI新profile已经生成安装ID；CLI省略时Windows存于便携data目录，Linux存于/var/lib/wbd-client/installation-id。serverIP/SNI/route-key等现有建连配置仍必需，未新增公开握手或常驻管理进程。Windows网络地址在第一条真实受保护回复成功后、创建TUN及配置路由前确定。后续lane和换代回复必须与当前地址一致；重启服务端重新分配需客户端重新建立逻辑owner，不允许静默改活跃owner地址。

## 部署工作顺序

1. 多客户端自动租约及双平台客户端接线；Actions单测/race、同账号多客户端Normal/Game并存、源地址隔离、换代稳定、7天续期/到期/活跃保护、池耗尽及错误密码不分配。当前IMPLEMENTED_PENDING_ACTIONS。
2. 服务端统一退出/清理/非零错误码、配置只读校验；systemd就绪与单实例、网络所有权持久恢复、窄范围RST抑制，保留foreign规则。当前IN_PROGRESS。
3. 安装/配置模板/管理工具、手动指定版本升级/回滚/卸载保留配置；amd64/arm64包与Actions真实systemd/网络安装生命周期。当前NOT_RUN。

应用建议目录为/opt/wbd/releases/<version>、/opt/wbd/current、/etc/wbd配置与证书、/var/lib/wbd网络恢复记录（不存IP租约）、/run/wbd锁；普通日志交journald。不做在线账户面板、不新增HTTP管理端口、不擅自热加载FEC/MTU/身份、不并行启动两个服务争抢raw/TUN。

## 验收

所有编译/测试/格式化在Actions。功能unit/race可集中；性能一个Action一条样本。安装重复执行、配置拒绝、启动部分失败、SIGTERM/SIGKILL、规则恢复、升级失败回滚、原配置保留以及实际正式入口双向业务须分别记录；ARM交叉构建不是原生运行、物理P7仍NOT_RUN。IP租约不用写磁盘；网络恢复日志是不同对象，仍需要持久化才能恢复进程崩溃前的系统修改。
