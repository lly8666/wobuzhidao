# 同源 DNS互备、最大UDP重复与 raw socket 环境

产品/助手SOURCE仍3a594a34191159bd7224f35ba9117cdf6f239c69，qualification ref不动；三个新增样本依次各300s，未并行负载、未改sysctl/产品。累计9条完整physical样本，P7仍IN_PROGRESS，历史FAIL/NOT_RUN不抹掉。

D02/1537只对当前lease的1.1.1.1 TCP/UDP53做FORWARD故障，DROP84，TUN元数据看到备用8.8.8.8的UDP24/TCP60响应；60/60系统解析成功，故障内部23/23含UDP12/TCP11。支持主DNS故障时备用工作，但不做逐query未观测关联。D03/1538故障8.8.8.8，规则DROP0，健康主DNS承担解析60/60；不冒充强制反向fallback已覆盖。普通53物理NIC观测无泄漏，范围不含DoH/DoT/其他NIC。

两条10M双向吞吐接近目标，仍FAIL business_loss：D02 C2S13/S2C295包、字节loss0.00219%/0.05213%，p99302.532ms、最大输入send lag222.432ms；D03 C2S20/S2C0、0.00502%/0%，p99121.196ms。探针2975/2975和2979/2979。server rawdrop122/104；既有完整性/输入p99/cleanup门按原分析保留。无匹配native RTT低载基线，不能把p99变化归因DNS或VM。

M03/1539完整300.130s，全允许发送包最终完整回收，但UDP8973/IP9001允许IP分片的167条中2条超过1秒，p991081.631ms/max1125.313ms；严格及时门FAIL，不以晚到补成PASS。96B1330/1330、p99116.925/max212.260ms；UDP8972共333条及时，65507有167条及时/p99229.464/max272.558ms。非法65508及DF超9000仍按MessageSize正确拒绝，bad/duplicate/invalid/socketerror0，server rawdrop0。有限wire窗口外层长度/校验和/固定seq字节通过且raw已删；owned退出清理恢复。不能关闭旧M03，也不能用接收buffer解释rawdrop0的本条late。

用户询问ARM raw socket缓冲如何改：实际rb212992，与系统rmem_default/rmem_max212992一致；raw_linux.go没有SO_RCVBUF。当前环境未改变。临时设置系统rmem_max/rmem_default各1048576，重启wbd-server后用ss -m -p -A packet核验其rb1048576；这是全局新socket默认，不是WBD独占设置。回滚两值212992并重启。长期建议只设置raw recvFD并getsockopt记录实效；Linux显式SO_RCVBUF通常返回请求值双倍，目标实际1MiB可请求512KiB并核验上限/readback。尚未开发/部署，不把扩大buffer记作修复。参考[Linux socket(7)](https://man7.org/linux/man-pages/man7/socket.7.html)。

证据沿用docs/evidence/lane-duplex-3a594a3-physical-20261007.json；本机分析和原始计数receipt hash随样本留痕，不提交凭据或大pcap。下一步匹配RTT/default split/IPv6及大包late定位。产品变更需要新SOURCE/Actions资格，不能由物理验收暗改二进制。
