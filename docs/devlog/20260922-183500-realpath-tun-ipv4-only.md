# 20260922-183500 realpath shared-TUN IPv4-only 启动修复

## 本轮目标和基线

目标一句话：依据首轮正式二进制 realpath Actions 的最早失败证据，只修 Linux shared-TUN IPv4-only 上线时序和证据上传问题，再用同拓扑 exact-SHA 回归，不提前进入18主测。

开始分支 `next/tlslike-dataplane`，远端 HEAD / 已测试 SOURCE_SHA / harness SHA 均为 `7a15f11fdeb96710154fb4894b29e3856791d758`。该 SHA 只有 realpath harness/docs 新增，没有改 FEC、repair、runtimeowner 或业务数据面。原主线 agent 按用户要求继续暂停；本轮不修改 TLS_STARTUP_PADDING 小功能。

## 原始证据

- realpath calibration: https://github.com/lly8666/wobuzhidao/actions/runs/35710291033，job 106688980621，FAIL。
  - 正式 `wbd-server` 在正式 client 与业务发生器启动前即退出，日志为 `wbd-server stopped: linuxserver: invalid IPv4 packet`。
  - 脚本随后在 `kill -0` 存活检查失败；这不是需要忽略的 kill 错误，而是 server 已死亡的二次证据。
  - artifact 上传又因 `artifacts/realpath-calibration/server-key.pem` 为 root-only，Actions uploader 报 EACCES，因此首轮 pcap/qdisc 原始文件没有成功上传。证据缺口必须修，不能把该轮叫校准PASS。
- targeted: https://github.com/lly8666/wobuzhidao/actions/runs/35710290998，整轮 SUCCESS。
- foundation: https://github.com/lly8666/wobuzhidao/actions/runs/35710290924，30 jobs仅旧 `p5-weaknet-5-30-5 run2` 红灯。job 106689033052 的120s产品harness自身完成，但40次HTTPS仅30成功、10失败，repair=509、wire_amp=5.088080；旧validator因post5无法复算而FAIL。artifact 10686810529，zip sha256 `1e75ff691eaf33a4e12da0d5c13e67e2a26fbb590390343e3e2ea1527a7d96ee`。因为7a15f11没有产品数据面改动，这个旧随机门失败保留为用户体验事实，但不能当成本轮realpath改动造成的性能退化，也不择优重跑覆盖。

## 根因判断与最小修改

活动代码的 `packetDestination4` 对非IPv4和畸形IPv4统一返回 `linuxserver.ErrInvalidIPv4`；正式 server 的TUN读取循环把该错误作为进程级错误。首轮时序显示 shared TUN 刚上线、尚无业务就触发，因此优先处理 Linux TUN 的 IPv6 link-local/control 自动流量，而不是取消IPv4校验。

产品侧最小修改：

1. `internal/linuxserver/network_plan.go` 给共享TUN增加 owned sysctl `net.ipv6.conf.<tun>.disable_ipv6=1`，与当前IPv4-only lease/data plane一致。
2. `internal/linuxserver/runtime_linux.go` 在 `OpenTUN` 已创建但接口仍 down 时先写入所有sysctl，再执行 `ip link ... up` 和route。这样不会在disable_ipv6生效前打开接口；setup失败仍由既有cleanup恢复保存值。
3. unit contract固定三个owned sysctl；privileged资格额外断言 `disable_ipv6=1` 且 shared TUN 无 `inet6` 地址。
4. 不改 `RoutePacket` 的畸形IPv4校验，不把所有 `ErrInvalidIPv4` 吞掉。

harness侧只修证据可靠性：

- 临时TLS私钥改放 `/tmp`，退出即删除，不上传私钥。
- cleanup保证root运行产生的ART证据对Actions uploader可读。
- diagnostic tail 对缺失文件使用 `|| true`，避免辅助诊断步骤制造额外红灯；最终 `Enforce calibration result` 仍严格要求harness和validator成功。

本轮未扩大4096/FEC/socket缓存，未改padding默认，未降低任何弱网门槛，也未增加隐藏带宽cap。

## 测试状态

本提交只编辑源码/harness/文档；按AGENTS.md未在本地编译、测试、race、netem或性能运行。候选 SOURCE_SHA 是包含本日志的提交，提交后由 GitHub Actions `GITHUB_SHA` 与manifest精确记录；提交前状态为 **NOT_RUN**。

下一轮必须首先读取新 exact-SHA realpath run 的 job log/artifact。只有正式 client/server存活、raw/TPROXY/TUN双向无损业务与300ms双向qdisc/抓包校准全部PASS，才允许扩18主测。

## 下一项

若新realpath仍FAIL：保留上传成功的server/client日志、四点pcap、qdisc和kernel state，确认最早非IPv4/路由/TPROXY/FakeTCP断点后最小修复。若PASS：固定该拓扑，新增120s 30/60/30、Normal1 10Mbps/向与Game4 3Mbps/向、FEC20:20/padding off/MTU1400、3 seed × lossless/20%/30%的18个独立job，并在进入主测前补齐互斥成本总账与每秒资源采样。
