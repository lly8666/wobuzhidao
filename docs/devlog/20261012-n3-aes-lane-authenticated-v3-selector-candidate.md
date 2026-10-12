# 2026-10-12 N3-B：把独立 AES record 核心接入 V3 Lane 收发路径（测前候选）

用户的优先顺序已经明确：先无HOL，再配置和Windows GUI，再尽快收口、推进N3–N6。不重复陷入N2单seed优化。现有N2 Go并发无HOL限定PASS [Action38257097791](https://github.com/lly8666/wobuzhidao/actions/runs/38257097791)与后续Action38263051747，**不是独立跨TCP/UDP所有小流10ms完整物理资格**。Windows GUI真实WinForms传输设置分页/Normal auto off/fixed模式/初始R20/R0与品质读取已通过原生258项测试、截图 [Action38260818090](https://github.com/lly8666/wobuzhidao/actions/runs/38260818090)，N3 AES尚未开放为GUI选项。

N3 record单层核心Source `f321e671c546b1ee5cc4e239ce7f462e7d02f90b` [Action38263629068](https://github.com/lly8666/wobuzhidao/actions/runs/38263629068)已通过AES128/256标准库GCM、方向隔离HKDF、各自HP PN AES块、独立测试向量/tamper/replay/noHOL/V2 ChaCha旧wire、Go unit/race/vet及Windows/Linux cross-build，**但先前 Lane 只调用旧ChaCha构造器，所以尚不能公开宣称产品支持AES。**

这一提交只预声明N3-B内部最窄接线：`LaneConfig.RecordVersion/RecordCipher`在 `ClientLaneConfigFromAdmission`、`ServerLaneConfigFromAdmission` 只从TLS保护且协商确认过的`AdmissionResult`拷贝，V2/传统直接Lane保持0/0选以前ChaCha构造器。V3必须是有效suite1、2、3才使用`tlsrecord.NewSealerForCipher`和`NewDecoderForCipher`，从role+direction keys仍分别选择C2S/S2C，不用全局runtime猜AES，也不读取未认证配置真值。N2 auto policy非零时还需要Policy.Cipher与Lane.RecordCipher相符，缺少V3、无效suite/mismatch配置必须提前fail-closed。新 `internal/datapath/n3_aes_lane_test.go` 用真实datapath Lane/TLSRecord/FEC encoder路径验证V3的ChaCha/AES128/AES256各自在0/4/20 parity的首发C2S/S2C字节完全交付、旧lane默认和非法乱选被拒；不是仅调用crypto/aes示例。恢复原31B RECORD overhead/MTU规则，TLS外层伪装/路由与FEC源码不改。

**仍然未打开**`realityfront.AdmissionPolicy.SupportedNow` 的AES：实际受保护Admission/CLI/GUI不会发出AES，N3网络真实性能、同runner硬件AES与软件fallback CPU比较、错suite跨端失败均需随后单独GitHub Action证据。拟新workflow `.github/workflows/next-adaptive-n3-aes-authenticated-lane-core.yml`仅一个Go测试job、0真实性能job，先精确SOURCE/policy然后Go单元/race/vet和Win客户端/Linux ARM64服务端交叉build、小回执hash；本条提交为候选，不预填任何PASS或Run ID。上述明确通过再推进N3-C受保护策略双端互认与严格cipher错配、独立AES128/AES256实网单场景Job，才在GUI传输页显示受支持密码套件。原旧非本阶段`realityfront`测试vet goroutine Fatalf失败SOURCE35ce7df已存；本条只vet当前改变的datapath/tlsrecord模块，不撤销其真实单元/race。

总体N2仍OPEN：C04 S2C 5%恢复降档78.3%、C05 S2C10%档70%红、GE业务p99 1.83–1.95s长尾、历史2.73秒探针和80秒S2C未关闭。N4 Windows TUN/N5 GUI更多闭环/N6正式交付包未收口、physical NOT_RUN，不部署、不移qualification ref或mainline。
