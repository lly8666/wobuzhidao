# 20261006-211200 Npcap owned返回复制候选

## 本轮目标和阶段

开始HEADc4e5e7bfc0050969dcf915c624f74335f9b83517，next/tlslike-dataplane/P7实机诊断。部署产品仍a280566b3a1642d43afe95f29a70ab4daf4e8759；完成profile后选择窄资源优化，候选未经Actions不得部署。

## 修改与原因

internal/faketcp/npcap.go：MarshalSegment创建独立packet，encodeNpcapOutbound另创建frame并copy IP包。旧return又append整包是第三份存储，不再有外部输入别名必要；直接return packet，仍与frame独立。每encode调用去掉一次整包分配/复制；控制/数据/现有readybatch共用该函数。无pool、借用buffer、unsafe新指针，Npcap ingress生命周期owned副本保持；wire/MAC/checksum/IPID/generation/sendMu/部分批量回执/sparse立即发送/同Seq重传/FEC/MTU/4096/ACK节奏不改。

internal/faketcp/npcap_test.go：在既有实际序列化流绑定测试增加调用者payload变更、frame变更、后续encode、packet变更四个生命周期独立性断言，保护历史ownership修复。gofmt仅格式编辑，本地未编译/运行Go测试。

## 复用来源

无old迁移。原子修正现有所有权边界。native助手十四文件仍b393字节一致；guardprobe来自a280；CPUprofile复用已有显式环境入口，默认off。

## Actions证据

c4e文档父四自动Actions37466477380 foundation/37466477223 targeted/37466478006 GUI/37466477312 preflight PASS，仅父文档及a280运行源码，不属于新候选。新候选core/race/独立Normal与Game5205/P6/native全部NOT_RUN，冻结提交后各自执行，每性能Action一条，禁同run A/B/matrix。a280 scoped13Actions与024 full70/18/1800s保留历史不继承。

## 问题、排查与风险

sourcea280 CPU诊断seed1469完成300.0010533s：goodput9.999980/9.996484M，C2S0missing、S2C148missing(.0349643%字节)，2979探针全回但p95=502.9129/p99=662.8784ms、latephasep99=696.8754ms。Windowsuserqueue overflow3233，rawserver124，clientFECpressure23；serverAbandoned/RepairEvicted23，但FreshBlocked0、完整性0。客户端CPU320.390625s/server163.73s、助手21.546875/36.647819s单列。诊断开启会改变工作负载，不能把此次当普通性能资格或与off直接判断退化原因。

profile151041B、总时段357.568s含startup/stop，样本权重1183.21s明显大于实际CPU；1044.94s foreign/syscall权重包含阻塞，不表示CPU88%耗在DLL。按首project frame分桶：NpcapRead333.41、singlewrite144.12、batchwrite51.01、TUNRead139.28s，均是采样权重不是独立真实耗时。nonforeign138.27s中FECxor19.23s、EncodeActive累计21.96s、mallocgc累计14.18s、encodeNpcapOutbound累计5.24s；累计会重叠，不能相加或当占真实CPU比例。同期现有DLL墙钟调用/锁等待及队列年龄另存证据。两份profile都支持FEC/调度/分配是热点；无证据本次轻微copy优化能独自解决nativep99。更大优化先定位ACK/driver同步及队列时间边界，不能简单放大状态或改有限恢复。

证据native-cpu-profile-a280566-seed1469-20261006.json/.pprof；配置/cleanup/输入/完整性/FEC原计数保留，rawpcap已分析删。此诊断另列，不增加48普通300s/23unique/20NOT_RUN的资格计数，未关闭M03。D06on/off功能结论及失包仍保留。

## 下一项原子任务

冻结candidate，Actions验证所有权/真正Linux与Windowsunit/build/Linuxrace及readybatch partialprefix/fallback。两个独立5205 Normal1/10M seed1470、Game4/3M seed1471；保持原p99/业务门、runner容量分类。基本门通过后同源P6再nativeprofileoff复验，只有实测才能报告CPU/p99变化。若门失败保留失败并修窄问题，不继续下一优化。
