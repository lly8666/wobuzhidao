# 20261007-193015 同源 Normal / Game 换代五分钟

固定SOURCE3a与qualification ref未变，普通profile/stage timing off，helpers逐项同源。两条完整300s，独立负载与退出清理。

Normal S16/1533双向9.999489/9.999953Mbps，上行缺22包/0.004907%字节，下行0，probe2979/2979、p99=115.4047ms、server raw drop71，严格business_loss仍FAIL。generation1..6，physical peak2/retiring1，活动期未见active=0；五次换代附近probe均返回，局部p99约108.46–116.74ms。

Game4 S17/1534双向2.999990Mbps、业务loss0、probe2979/2979、p99=107.4307ms，server raw drop567，BUSINESS_PASS_TRANSPORT_PRESSURE。四logical均有换代、9个ref、physical peak5/retiring1、未见active全0，lease稳定，观察边界PASS；快照不证明精确逐包A→A+B→B或严格零HOL。

两条DNS60/60、专用物理NIC plaintext53 observer无泄漏且drop0；Npcap和用户route overflow0，完整性/outer预算/raw capture删除/owned清理通过。长时间numeric header observer自身有drop，故未观察到某个FIN或跨窗口顺序不可严格用于否定或通过关闭门。有界全量pcap窗口capture drop0但只有局部覆盖。

ARM rmem_default/rmem_max/wmem_default/wmem_max均212992，产品AF_PACKET socket实际rb212992；同源码Actions summary产品socket rb1048576。当前openRawIPv4Endpoint没有SO_RCVBUF设置，继承系统默认。余量不同不是唯一根因证明，不把小丢包直接归因WAN。用户询问设置方式已答复；本轮没有修改全局sysctl/产品/包。程序SO_RCVBUF有效值Linux通常为请求的2倍，应读回核验；rmem_default路径与该setsockopt语义区别记录，后续试验不得混入此组。

四条普通均达目标附近、probe全返；两条Normal残余loss、四条server接收pressure与缺匹配native低载RTT基线仍保留。P7非全PASS。S18 idle/keepalive与M03最大UDP由同一原聊天串行控制器继续，不另起负载或重复开发。证据 `docs/evidence/lane-duplex-3a594a3-physical-20261007.json`。
