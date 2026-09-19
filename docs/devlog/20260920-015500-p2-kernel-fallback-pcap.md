# 2026-09-20 P2 普通内核TCP/TLS fallback与连续pcap

## 基线

基线 `b71b57502db65b221037e803935058f27423c703`。Actions run 35458197765 PASS：repository-contract、Windows/Linux unit/build、Linux race、tlsrecord fuzz/reference全部成功；recognized WBD的TLS1.3 ticket/ALPN/resumption策略及admission detach后无旧TLS writer已验证。P3 LINK/FEC/MTU不修改。

## 硬门缺口

此前所有fallback均是内存peer或serializer证据，不能满足ACCEPTANCE要求的“普通内核TCP TLS客户端经真实网络入口、验证decoy证书、完整HTTP、正常关闭、连续抓包”。根树也没有最小Linux raw I/O，因此无法用内核TCP栈直接驱动FakeTCP association。

## 修改

- 新增Linux-only `faketcp.RawIPv4Endpoint`：AF_PACKET从指定接口读取真实IPv4/TCP包，忽略PACKET_OUTGOING重复；现有ParseIPv4TCP进入association；现有MarshalSegment生成checksum/options/DF并通过IP_HDRINCL raw socket发送。只服务P2最小入口，不新增TUN/DTLS/内核TCP业务通道。
- 新增受环境门控的 `TestKernelTLSFallbackVerifiedHTTPAndNormalClose`。Actions以root运行：普通 `crypto/tls` 客户端通过内核TCP连接 `127.0.0.2:24443`；FakeTCP raw adapter完成SYN/TLS/fallback；原始ClientHello送到受控 `target.test` kernel TLS server。
- 测试动态创建受控CA和leaf证书，客户端RootCAs显式信任CA，`InsecureSkipVerify`保持false；断言VerifiedChains、目标ALPN=http/1.1、真实HTTP GET与完整响应体、TLS EOF以及双向FIN后association表清空。
- hosted adapter只抑制一次FakeTCP sender看到的ACK，真实内核ACK仍被pcap捕获，从而触发一次RTO。emitter在重传前先检查相同Seq payload完全一致，随后放行duplicate ACK。
- Actions新增连续 `tcpdump -i lo` pcap。iptables只丢fake server端口的内核自动RST，避免本机无监听socket与raw server竞争；不丢FakeTCP业务包。
- 新增标准库Python pcap检查器：要求tcpdump kernel drop=0；单四元组SYN/SYN-ACK/final ACK；server MSS=1360、WS/SACK协商；ACK不超过该方向实际已发送范围；server IPv4/TCP checksum正确、DF且无分片；无RST；双向FIN；至少一组间隔>=0.5s的相同Seq server payload逐字节一致。生成带SOURCE_SHA的JSON summary与原始pcap artifact。

## 外观边界

这项资格只证明普通fallback路径真实互通与TCP/TLS线格式，不把loopback或受控站点结果宣称为指定公开网站完整指纹一致。recognized WBD仍是本地Go TLS server；Firefox120仍是固定模板。内层TLS业务指纹继续由P3/P4/P5承担。

## Actions / SOURCE_SHA

本日志随网络资格实现提交创建，提交前为 `NOT_RUN`。所有编译、root raw网络实验、race/fuzz和pcap分析仅由GitHub Actions执行。通过后必须以本提交精确SOURCE_SHA、run URL、pcap artifact和summary回填STATUS；失败则P2保持OPEN。

## 下一步

运行新增 `p2-kernel-fallback` 与既有回归。只有真实网络job、Windows/Linux基础回归、race/fuzz都通过并核实artifact后，才提交P2关闭状态；物理Windows/Npcap仍为P7。
