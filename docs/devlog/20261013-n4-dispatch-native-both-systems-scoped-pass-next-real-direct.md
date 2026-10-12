# 2026-10-13 N4 TUN安全分派边界 native Windows + Linux race 限定PASS，真实TCP/UDP Direct尚未连通

新的唯一SOURCE `fb91a835c135559771fe59381846dc5c0dc82d5f`，[Action38289962670](https://github.com/lly8666/wobuzhidao/actions/runs/38289962670)，Linux Ubuntu24.04 job114947400455 go unit/race SUCCESS，Windows 2022 job114947400307 native Go编译/unit SUCCESS。原始首次 SOURCE57783244/Action38289712226双方因新测试的无用 `net/netip` import 在编译期FAIL；第二次**只删了此测试导入**，生产 Go 分派实现保持同内容。所有红Action保留，不把首轮FAIL重标成PASS。

本次真正通过的是`internal/windowsclient/tun_packet_dispatch.go`和`tun_packet_dispatch_test.go`纯Go边界：先以已有512 flow/256 fragment、interfaceIndex+generation 关联和源lease限制分类；海外保护及UDP/TCP53劫持必须走既有受认证Normal/Game Router，直连LAN/CN/custom/underlay必须有显式`TUNDirectSink`，缺失backend或backend返回error不能将direct谎称为protected tunnel成功；跨出Wintun借用缓冲区前复制包，拒绝旧generation/无端口分片、源地址spoof、classifier与owner不同/32 lease。测试覆盖三个分支、真实错误传播与变更源包安全；先前N4纯目的地DNS分类SOURCEa4689a Action38284069629双平台PASS、bounded flow+fragment SOURCE05e5ae Action38284708568双平台PASS，全部仍为纯Go/逻辑层。

**关键不完成**：`TUNDirectSink`当前仅接口与模拟，没有真正Windows用户空间TCP/IP网络栈或透明socket forwarding/返回包回Wintun；不能简单把原始TCP SYN包送进`net.Dialer`并声称还原系统TCP连接。native Windows Wintun实际设备和驱动加载、物理网卡地址/IfIndex/source绑定、UDP映射、TCP状态关联、可逆所有权route/cleanup、actual network/TUN首达尚未跑。N4仍OPEN，物理NOT_RUN。

新用户要求的Windows GUI传输FEC和三套件已 native Windows GUI273 tests PASS，N3 AES128/256各100s真实五netns无损限定成功。N2完整TCP+UDP并发 per-flow10ms首达noHOL仍不具严格实网证据（当前Go noHOL/race限定PASS），GE相对静态网络单包首达p99 1833/1947.5ms与C04/C05资源降档FAIL都OPEN。后续应以**真实混合小流单Action/单SOURCE/单config/单scenario/单measurement job**验证并约束无HOL，不靠单UDP聚合10ms空桶；之后N4原生Direct TCP/UDP、分流可逆路由，再N6 Windows/Linux packages。严格不部署、不移qualification ref或合mainline。
