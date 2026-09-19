# 2026-09-20 P2 关闭：Actions真实内核TLS/HTTP与pcap通过

## 关闭基线

P2产品资格绑定精确 SOURCE_SHA `04f57fa880837e3bc16374cd93bcff6aa8bc4202`，GitHub Actions run [35458896477](https://github.com/lly8666/wobuzhidao/actions/runs/35458896477)。本日志/STATUS是关闭后的文档提交，不把文档HEAD冒充为产品SOURCE_SHA。

P3并行成果保持原样：LINK单数据报分片/重组、FEC off + 20:4/8/10/12/16/20全固定挡位、统一MTU/预算均保留其既有Actions证据；本轮没有修改tlsrecord/pathmtu/FEC/LINK/datapath或Game语义。

## 本次P2实际完成项

### TCP建连与生命周期

- 普通合法SYN继续可进入ClientHello/fallback，不以WBD SYN persona做服务端身份门槛。
- 精确重复SYN复用同一half-open association，保持同server ISN/SYN-ACK；SYN-ACK重传次数与half-open寿命有界。
- future ACK超过已发送序列空间明确拒绝，不释放pending；重复/旧ACK保持幂等。
- peer MSS/WS/SACK和实际receive window进入bootstrap发送；zero-window等待真实窗口更新，非零小窗口发送可容纳短段。
- bootstrap从逐chunk stop-and-wait改为最多4 chunk有界flight，同时保留每flight累计ACK屏障和移交边界ACK屏障。
- 本端通告窗口由256KiB实际bootstrap容量推导，不固定虚报65535。
- FIN占一个序列号，尾payload先交付；重复/乱序FIN、FIN重传、half-close均有测试；CloseWrite只结束发送方向。
- RST按状态/当前接收序列合法性接受，不接受任意伪造RST；表Sweep有界释放half-open/closed association，四元组可安全复用。
- internal DetachTransition只移交TLS/bootstrap adapter所有权，不等于关闭外层association；关闭/cancel会唤醒sender/bootstrap/raw读取等待者。Linux raw endpoint Close在最终资格中保持<=1s。

### TLS外观、ticket与切换

- 普通访客仍将原始ClientHello byte-exact转发给固定配置的真实decoy，目标自行选择ServerHello/ALPN/证书链；SNI不匹配不会变成任意目标开放代理。
- recognized WBD本地server固定TLS1.3、RenegotiateNever；不协商h2/http/1.1等未实现应用ALPN。
- 真实Go TLS 1.3 NewSessionTicket在Handshake返回前由stdlib生成；不sleep等ticket。服务器UnwrapSession显式拒绝恢复，客户端不配置session cache，不新增0-RTT；每lane仍完整TLS+protected admission+新exporter/context/nonce。
- 顺序固定为TLS握手内ticket -> protected admission -> PrepareTransition -> final reply ACK -> DetachTransition；成功detach后的session不再暴露旧tls/uTLS writer，因此无法续写close_notify/KeyUpdate/ticket。
- 最后bootstrap payload/ACK loss、transition queue、首record丢失后后续record立即交付、同Seq重传相同密文等既有P2证据均保留。

## 最终Actions与真实网络证据

Run 35458896477 全部PASS：

- repository-contract PASS。
- Windows unit/build PASS。
- Linux unit/build、race、directed tlsrecord fuzz、independent reference generator PASS。
- privileged `p2-kernel-fallback` PASS；kernel integration test耗时1.24s，且测试内显式 `server.Close<=1s`。
- 普通内核TCP + `crypto/tls` 客户端连接 `127.0.0.2:24443` 的raw FakeTCP入口，经fallback访问受控 `target.test` TLS server。
- 客户端用测试CA `RootCAs` 实际验证证书链，`InsecureSkipVerify=false`；`VerifiedChains` 非空，目标协商 `http/1.1`。
- 完成真实HTTP GET、完整响应体、TLS EOF和正常双向FIN关闭；association表最终清空。

连续pcap artifact：

- artifact ID `10589721030`，名称 `p2-kernel-fallback-04f57fa880837e3bc16374cd93bcff6aa8bc4202`，ZIP SHA256 `4518f772a84f10d61b7a355f805e74f6cedacd7c078c27479554ab4088c8993a`。
- 29个目标flow packets，tcpdump 58 packets received by filter，`0 packets dropped by kernel`。
- server IPv4/TCP checksum 14/14验证通过，DF存在，无意外IPv4分片。
- SYN/SYN-ACK/final ACK同一四元组；server MSS=1360、WS=8、SACK=true，Seq/ACK范围合法。
- 无RST；server/client FIN均存在。
- 实抓同Seq `1510000975` 的server TLS payload在 `1.024768s` 后重传，payload逐字节相同。
- foundation artifact `10588996549`；tlsrecord-reference artifact `10589696286`。

## 剩余外观差异与诚实边界

P2关闭不等于“指定网站完整指纹一致”。recognized WBD仍由本地Go TLS server完成握手；Firefox120只是固定uTLS persona，不是2026当前Firefox。ServerHello选择、cipher/curve、扩展集合与顺序、证书链、握手record尺寸/分段、恢复行为等仍可能与指定目标网站不同。普通fallback真实互通与格式通过只证明该路径透明代理受控目标成功。

内层业务TLS的包长、方向、突发和往返节奏仍属P3/P4/P5。P2没有增加padding策略、假HTTP、随机延迟、cover traffic或凑包等待，也不以外层ticket声称消除内层指纹。

Linux RawIPv4Endpoint只是P2 hosted资格需要的最小适配；完整产品platform I/O、Windows物理Npcap/TUN和最终真实网卡抓包仍按P4/P7执行。

## P3交接

P2工作流关闭，项目最早未完成阶段回到P3。继续保留既有LINK/FEC/统一MTU成果，下一任务按DEVELOPMENT_PLAN第13节完成session/owner串接与显式有界record padding能力：默认0/off、只用MTU余量、无等待、不增加分片、不改变FEC/Game/4096有限恢复。共享STATUS修改前继续同步远端。
