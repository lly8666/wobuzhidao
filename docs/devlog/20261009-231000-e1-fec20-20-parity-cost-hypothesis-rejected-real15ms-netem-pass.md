# E1 15ms实际UDP路径已绿；FEC20:20校验字节节省假设被真实单测否定（2026-10-09）

工作分支 next/performance-efficiency-20261008、父HEAD 7c9f0b10b4293bb97a4ab55148833d82601e95f9、正式32ms SOURCE a2db258b436a41fdee98c6c53abec9bab6ce600f不改。前一条提交同步触发**三个独立Actions**：真实[Linux netns 15ms UDP 37932313178](https://github.com/lly8666/wobuzhidao/actions/runs/37932313178) SUCCESS；[Lifecycle37932313153](https://github.com/lly8666/wobuzhidao/actions/runs/37932313153) SUCCESS；[Foundation37932313522](https://github.com/lly8666/wobuzhidao/actions/runs/37932313522) **FAIL**。原始FAIL必须留存，不可把成功的网络功能当整体PASS。

真实测试在独立netns lo用`tc qdisc netem delay 15ms`，真实UDP收发FEC20:20 wire。首个1372B应用UDP拆分多个源片，故意不发送第一片，32ms到期发送校验片，确实恢复一次；之后45ms发另一独立96B包，收到一次且早于自身校验，禁重。可核对的内核socket测量：parity→recovered one-way **15.088ms**，source→fresh one-way **15.060ms**。资格仅是 Linux UDP socket+LINK/FEC路径，**不**是正式FakeTCP/TUN client-to-target、也不意味着300ms历史压测能证明15ms产品全栈。

资源假设的**反证**来自Foundation Go测试：同30个96B稀疏包（12ms间隔）、显式8/32ms窗口、FEC20:20：8ms `sources=30/parity=30/sourceBytes=4560/parityBytes=4560`；32ms **完全相同**。原测试武断要求32ms更少parity片/字节，因此红灯。阅读实际`FastBlockEncoder.flushParity`：20:20下按活跃source shard个数输出对应parity，累计数不会因分块窗口自动减少；我们必须明确**32ms并未节约这个构造的FEC带宽**，更不能声称CPU优化。

只纠正非产品Go测试的错误收益断言，改检查真实 `path.State().Encoder.PartialBlocks`：32ms能否让稀疏包聚成更少的局部parity块（可能减少关闭/初始化工作），parity数量/字节数反而严格断言**相等**；新的Go测试只提供block closure统计proxy，CPU-s与PPS开销仍NOT_MEASURED。未改产品运行时、packet-size class、timer、buffer、queue、协议、源码冻结源。下一次Foundation/Lifecycle/netns重新审；如果关闭数也未变，诚实记录NO_HOTSPOT而非继续钻微优化。

维持原32ms Normal1 lossless、Normal1 5205、Game2 lossless、Game2 5205分别一条300秒profileOFF scoped PASS（真实Game2 lanes未确认），不为挑宿主再跑。旧Game4 EPYC9V45 profileOFF socket drops client33/server86与ON client42正式FAIL，以及E7约80秒S2C中断OPEN_DEFERRED，都不因新测试改变。CPU gain UNPROVEN。
