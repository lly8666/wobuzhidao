# 20261005-082000 Windows收包与处理分离候选

## 本轮目标和来源

连续修复P7。父SOURCE d28c0304d2ae23106b85adf90f4ef30b9ca9eb62 已过foundation37245556107、targeted37245556118、GUI37245556111、独立Normal5205 37245626945/Game5205 37245629048全部五项classification及P6 37245631118，同源包已部署。新SOURCE不能继承父测试。旧046 GUI漏页FAIL保留，d28已修。产品编译测试仍Actions，开发机只编辑/格式化/证据整理。

## 实际诊断与原因

原生D01 seed1321双端诊断运行中：Windows实际本产品pcap_stats supported=true、samples130、stats_errors0，在约108s业务附近已有driver_dropped112285。独立Windowsobserver此前完整3e D01为0drop且S2C payload包数与server端差异约0.034%；不是精确逐包匹配，也不表示产品handle0drop。客户端FEC pressure_retirements1038/recovered_sources20127，Record/Path errors0、lifecycle recovery attempts0。证据支持最早实测主损失点位于Windows产品Npcap队列；不能凭server overflow0继续归因WAN。最终完整回执待收，诊断on开销单列。

## 修改与复用

Windows旧同一线程ReadSegment→FakeTCP/FEC/owner→Wintun完成才继续capture。新增BufferedClientIO复用本分支SegmentMux，每incarnation独立reader与有界handler队列，容量沿用Linux4096。未扩大Npcap内核buffer、shadow4096、FEC或协议，最多10物理incarnation保持。FIFO仅保持真实收包次序，不等Seq、ACK、另一lane或凑包。保留owned payload copy，防止borrowed capture buffer混包。

wrapper保存原始capture错误并唤醒Read，Close一次关闭route/base并取消错误观察者，失败候选/rotation/休眠仍由生命周期负责。诊断新增receive_queues，观察peak/queue age/overflow，默认off无采样。沿用现有bounded overload策略，内部overflow不能算无损PASS；更晚掉包或尾延迟更差均不能认定修复。

确定性单测覆盖handler暂停reader仍排空、乱序源Seq到达即交付、重复借用buffer不混包、capture原错返回、close恰一次且唤醒Read；交Actions unit/race。无old复用，无新增CLI/JSON字段。

## 风险、资格与下一项

完整d28 D01 seed1321已收：C2S9.99891M、S2C9.09706M/9.029%字节损失，probe2970发2738回。Windows产品driver_received2105611/driver_dropped336986，340次stats采样均成功；FEC pressure3229/recovered65065，最终inflight0，Record/Path错误0，正常exit0、owned规则清理通过。证据windows-native-receive-20261005.json及压缩回执，包含先前3e S16/D02与d28 Actions。累计43个唯一case已执行6个、剩37未执行；完整300s样本8份（不同SOURCE，不能继承），不含SETUP_FAIL/两份不完整诊断。

本候选NOT_TESTED。有界队列只吸收突发，持续能力不足可能增加排队，须一起验driver/queue drops、age、RTT、业务loss、CPU和退出收敛。先core/race/GUI、独立Normal/Game5205和P6，再同源native300s D01/S16。不能把Linux性能门冒充Windows收包收益。继续DNS反向互备/双故障及剩余矩阵，不扩大参数掩盖失败。
