# E4 回到产品：终止工具兔子洞，Linux raw recv 去掉第2次解析的最小候选（2026-10-09）

仅工作分支 next/performance-efficiency-20261008，父HEAD bff5aef4d8e737388f12575ffbcdedc4148a0cae。当前正式业务资格对应SOURCE仍为 ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072（Game4 true5205 profileOFF单条 scoped PASS，9V45 0loss profileOFF socket client33/server86 FAIL；profileON client42 FAIL）。之前[Foundation37881417298](https://github.com/lly8666/wobuzhidao/actions/runs/37881417298)完整SUCCESS；Cgroup2 quota UNKNOWN以及先前OFF 12s合成PSI17.06%超阈值，继续挖cgroup/BPF calibration的边际业务价值极低：**停止追加同类观测工具和无效runner抽样**。

**直接源代码审查**发现 `internal/faketcp/raw_linux.go` 里的 `ReadSegment` 对每一个通过IPv4/TCP完整性及目标端口过滤的入包：先`ParseIPv4TCP(ip)`，然后`rawOwnedIPv4TCP(ip)`克隆并`ParseIPv4TCP(owned)`再次完整解析固定字段及TCP选项。第二遍逻辑只是为了把Payload指针转换为owned，因此是每个有效包的重复CPU工作。受控最小候选：保留原首遍Parse、source IP+port filter、只对通过过滤的包拷贝一次，继承首次解析的固定字段；从已经通过验证的IHL和TCP doff确定payload偏移，依首次解析的实际payload长度重定位到owned中，避免尾部以太网/IP padding混入payload。保持另一个原有`rawOwnedIPv4TCP` helper作为独立test oracle，不改其外部行为。

在`internal/faketcp/raw_linux_test.go`加4种包形态严格比对旧版copy+reparse的Segment全字段及owned wire字节：普通payload、真实IP total小于frame的尾部填充、SYN options零payload、ACK SACK options+payload；篡改原始packet scratch之后重新确认`Segment.Payload`与wire不受影响。既有Linux16包预排队/单syscall批量/owned ring复用和稀疏单包不等待等测试保持，Foundation Linux/race及Windows/TUN/TPROXY/fallback全部为硬门。该候选是CPU热路径的**静态去冗余**，不是证明导致9V45微突发socket loss的因果修复。

这是**产品代码候选**，一旦提交当前分支HEAD源变更，所有ba8ed1历史profile/pass/fail资格继续按SOURCE严格隔离，不能继承到此候选，更不能称CPU gain已证。没有派发300s业务Action或新的BPF校准；旧9V45 OFF/ON资源FAIL不可删、不得通过扩大SO_RCVBUF/queue/FEC buffer消音。下一步先等Foundation，否则立即仅修复/回退此最小候选；若通过再评估一次真正有意义的验证，并预设停止线，不让工具基建再次侵占业务工作。E7 ~80秒S2C断流OPEN_DEFERRED，E6/P6/物理未放行。
