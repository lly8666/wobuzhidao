# 固定b4、真实双向10Mbps混合UDP、300秒无损：明确识别Linux TPROXY 8936B界限

## 原始资格与结果

SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`（freeze ref原封），helper `56f39aff649fc6aa508fb3897bab6ff9b0d77cf8`，唯一正式性能Action `https://github.com/lly8666/wobuzhidao/actions/runs/37731062203`，seed2608101、Normal单lane/FEC20:20、外层预算MTU1400、内层服务TUN9000、客户端Linux实为OpenWrt TPROXY、biz/target应用接口9000，300ms单向、loss0%双向，连续300秒业务+10秒drain。原始workflow **FAIL**，summary **FAIL**，artifact `11530166388` ZIP sha256 `aeaaff9ab27fe8ae6f3fa5c72923b2ea4479ccc84f99086f9544c0112c9de054`；有界头pcap已删除留hash。此前37729338574及37729716707助手INVALID仍保留，不覆盖。

业务端原始实际发送成功C2S/S2C=372252412/372241936B，first-valid收=130281240/130279736B，goodput3.4741664/3.4741263Mbps，发送覆盖率>99.99%。实测netem两向drop=0/attempted1295756和1423793；双向96/256/512/1000/1372/4068B所有已送包都有效收到且无业务重复，UDP小探针3000/3000、conditional RTT p99约601.148ms，窗口最多300ms无首次业务交付，不说明全部业务连续性。两方向全部8972(6223/6222)、8973(8297/8297)、65507(1705/1705)业务包首次有效接收均为0；大包返回RTT没有任何值，必须并列缺失/超时，禁止删掉它们后的平均p99宣称PASS。S2C客户端业务接收线程额外16,224个封包头错误，刚好等于该方向三档大包发送总数；C2S目标未见大包有效接收。此结果不是600ms RTT造成的稀疏偶发迟到，也不是声称已复现原Windows M03/1554约80秒回程中断。

## 源码因果边界

在固定b4产品 `internal/platformflow/frame.go`：`MaxPayload = logicaltunnel.MaxLeasedIPv4PacketLen - IPv4HeaderSize - FrameHeaderSize = 9000-20-44=8936`，是**平台flow UDP字段**上限，不是合法IPv4 UDP最大值，也不是LINK或9000B内层TUN自动分片预算。Linux客户端 `internal/openwrtclient/socket_linux.go udpLoop` 分配 `MaxPayload+1` 并在 `n>MaxPayload` 丢弃合法更长payload，未送入隧道。服务器 `internal/platformflow/udp.go readUpstream` 使用 `make([]byte, MaxPayload)` 从目标真实UDP socket接收，较长目标回包可被内核截断为8936B后继续包装发送，导致客户端原封包的声明长度/CRC不再成立。日志+精确大小断点共同构成受控因果支持；后者的截断拒绝兜底尚缺正式产品保护测试，不能默认为已修。

这是 **Linux TPROXY/platformflow入口的实际产品路径FAIL**；该Linux客户端没有客户端TUN，不能等同Windows Wintun的9000分片路径，也不能宣称整个大包延迟问题已定位或物理M03故障同因。是否引入合法UDP到IPv4片的全程支持须单独确定有界、账户/地址隔离与一次分片架构；若先消除回程危险截断，也不能把合法大包改成“预期拒绝”从业务loss分母删除。当前无产品修改、没有因果证据支持扩大FEC/4096/全局缓冲/改变RTO或压缩期限。

## 仍需工作

先增加受控极小产品功能回归拒绝no-silent-truncation和后续小包不污染；或建立正式Linux TUN客户端全链路以覆盖Windows packet语义（无法则A这三档在Linux TPROXY是产品能力缺陷与本专项路径限制，不能伪PASS）。B真实TCP助手的 https://github.com/lly8666/wobuzhidao/actions/runs/37731561854 仅静态/生成/内核检查PASS，B/C性能NOT_RUN。保留M03/1554两个故障与后续物理Windows→ARM另测边界。
