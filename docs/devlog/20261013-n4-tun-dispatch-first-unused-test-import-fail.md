# 2026-10-13 N4 新TUN分派桥首SOURCE：双系统Go测试编译前FAIL，最小修复

原Exact SOURCE `57783244d7bf33ae61aca0ad5c79a13d31e3ceec`，[Action38289712226](https://github.com/lly8666/wobuzhidao/actions/runs/38289712226)，Linux job114946578930与Windows2022 job114946579298均FAIL。Linux Go输出 `internal/windowsclient/tun_packet_dispatch_test.go:7:2: "net/netip" imported and not used`，Go未完成新分派逻辑单元与race；Windows也在Go构建步骤失败。没有触及真实Wintun或native TCP/UDP、更未更改宿主机路由。**必须保留红Action原样，不得把其叫作安全分派PASS**。

最小修复仅从`internal/windowsclient/tun_packet_dispatch_test.go`删除未使用`"net/netip"` import，**不改`tun_packet_dispatch.go`**、原五元组/分片缓存、Normal/按代际密封、GUI配置、AES、SOURCE测试工况。随后新 SOURCE由Github完整重跑`.github/workflows/next-adaptive-n4-tun-classifier-core.yml` Linux unit/race与Windows native Go unit，等待严格结果。先前SOURCE`05e5ae8e8a16aacf70a9cb1145065b5346859518` [Action38284708568](https://github.com/lly8666/wobuzhidao/actions/runs/38284708568) Linux+Windows 512 flow/256 fragment纯Go边界PASS不会因本编译错误倒退。新分派边界只有Go接口+mock direct，不具备任何实际Windows system-stack TCP/UDP direct backend，不能交付为已经可用的N4。

此前用户要求Windows“传输”页自适应FEC和AES128/256正常配置已有native Windows GUI 273项PASS，质量无证据显示UNKNOWN；N3两AES各100秒Go真实加密无损限定PASS。剩余N2真正独立TCP+UDP并发10ms首达noHOL 未测，C04/C05 R档过保护效率FAIL与C10 GE首达p99约1.8–1.95s/历史2.73s、80s仍OPEN。N4和N6不以这次失败充合格，physical NOT_RUN，不部署、不移动qualification ref、不并主线。
