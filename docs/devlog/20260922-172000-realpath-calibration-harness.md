# 20260922-172000 正式二进制真实路径无损校准 harness

## 本轮目标和阶段

目标一句话：先把正式 `wbd-client` / `wbd-server` 独立进程经真实 raw socket、OpenWrt TPROXY、路由/防火墙、Linux shared TUN 的无损闭环与 300ms 单向链路/抓包校准跑通，再允许扩展 18 个严格主测。

开始分支 `next/tlslike-dataplane`，远端 HEAD `1fc6b1c7c1917860b4b65ce6449b6361d421fce7`。原主线 agent 按用户要求继续暂停，本 agent 接手增强 P4/P5；TLS_STARTUP_PADDING 小功能不在本轮修改范围。

开始前重新读取 AGENTS.md、PROJECT_CHARTER.md、docs/STATUS.json、docs/WEAKNET_QUALIFICATION.md、docs/TLS_STARTUP_PADDING.md、docs/ACCEPTANCE.md 与近期 devlog。所有编译/运行/网络测试仍只允许 GitHub Actions，本轮未在本地运行测试。

## 修改与原因

1. 进度口径校正：
   - 复核 `896ae94bea1aa1e532b2a14ed9d3e707a72f5286` 的 next-foundation run 35698760856 为整体 SUCCESS。
   - 复核 `1fc6b1c7c1917860b4b65ce6449b6361d421fce7` 的 next-foundation run 35703632978 为整体 SUCCESS；GitHub Jobs API 当前列出 30/30 job SUCCESS，用户/UI口径为31/31 checks。只记录可核查的接口差异，不再把第5原子写成等待。
   - 同SHA targeted 35703632735 为6/6 SUCCESS，startup-padding 35703632926 为2/2 SUCCESS。
   - 第5原子因此只关闭“代码回归等待项”；增强 P4/P5 仍保持 REOPENED，真实路径性能 NOT_RUN。
2. 旧弱网证据口径校正：
   - 1fc6b1c foundation 的 FEC-off 5%→30%→5% run1 原始 job 106668822384：40次计划请求仅36成功、4失败，repair=687，wire_amp=5.329970，但旧 validator PASS，artifact 10683248864。
   - run2 job 106668822286：31/40成功、9失败，repair=592，wire_amp=5.352490，旧 validator 仍 PASS，artifact 10683916726。
   - 这两份只保留为历史HTTPS回归事实，不用 workflow 绿色替代用户体验，也不计入新的持续双向UDP资格。
3. 新增真实路径校准：
   - `.github/workflows/next-realpath-calibration.yml`：Ubuntu 24.04 root runner；只在 Actions 安装网络工具、编译正式二进制、执行 netns/veth/raw/TPROXY/TUN 校准、上传原始证据。
   - `scripts/realpath_calibration.sh`：五 namespace 拓扑为 business generator -> 正式 OpenWrt TPROXY client -> raw/veth -> router netem -> raw 正式 server -> shared TUN/NAT -> target。两方向各仅一个 netem qdisc，固定 delay 300ms、limit 200000、无 rate cap；四点方向抓包保存损伤前后证据。
   - `tools/realpath_udp_duplex.py`：C2S/S2C 独立持续UDP发生器，不用echo冒充第二方向；64/256/1200字节等包数循环、序号/长度/内容校验；低速probe单独回显，只用于RTT。
   - `tools/check_realpath_calibration.py`：独立复算两方向唯一交付、内容/重复、发生器p99迟到、probe、pre/post抓包配对约300ms、capture drop、外层IP字节以及重复TCP序列形成的外层repair。校准配置为Normal1/FEC20:20/padding off/MTU1400/0% loss，低速8s，仅验证路径与注入/抓包，不冒充10Mbps持续主测。
4. harness 额外保存 active/after-cleanup 的 nft/ip rule/route/TUN/iptables、qdisc统计、link统计、tcpdump drop计数、二进制版本和manifest。未引入隐藏带宽限制。

## 复用来源

无。本轮没有读取或复制 old/ 源码；只依据当前活动树与权威文档设计 harness，因此 REUSE_LEDGER 不变。

## Actions证据

已存在并本轮复核：
- SOURCE_SHA `896ae94bea1aa1e532b2a14ed9d3e707a72f5286`：https://github.com/lly8666/wobuzhidao/actions/runs/35698760856，foundation overall PASS。
- SOURCE_SHA `1fc6b1c7c1917860b4b65ce6449b6361d421fce7`：https://github.com/lly8666/wobuzhidao/actions/runs/35703632978，foundation overall PASS；Jobs API 30/30 success。
- 同SHA targeted：https://github.com/lly8666/wobuzhidao/actions/runs/35703632735，6/6 PASS。
- 同SHA startup-padding：https://github.com/lly8666/wobuzhidao/actions/runs/35703632926，2/2 PASS。
- 旧5305 raw jobs/artifacts：job 106668822384 / artifact 10683248864 = 36/40；job 106668822286 / artifact 10683916726 = 31/40；两者旧validator均PASS。

本提交新增的 realpath calibration 在提交前为 **NOT_RUN**。只有 exact-SHA workflow 产生 run/job/artifact 后才可改写其状态。源码SHA与harness SHA在此原子提交相同；manifest另保存四个harness文件SHA256。

## 问题、排查与风险

- 真实 netns TPROXY + raw socket + shared TUN 尚未有该 harness 的 Actions 运行证据，任何路径细节目前都只是实现候选，不能写 PASS。
- Linux raw FakeTCP 会与内核TCP RST竞争；harness仅在client/server namespace OUTPUT丢RST，避免内核抢答，不改变产品wire或业务路径。
- 校准 qdisc 仅 delay 300ms、无loss和rate cap；低速0.5Mbps/方向、8s不是性能资格。
- 外层pcap用96B snaplen但保留原始packet length与IPv4/TCP头，足以复算外层IP字节/Seq/ACK/repair和pre/post延迟；它不是业务payload内容证据，业务内容由两端独立应用校验。
- 主测要求的FEC source/parity字节、Game复制、进程/线程CPU、GC/RSS、queue age等一等成本/资源账本尚未接入，必须在18主测前补齐。
- 物理资格仍 NOT_RUN。

## 下一项原子任务

等待/检查本 exact-SHA `next-realpath-calibration` Actions。若失败，依据client/server原始日志、active kernel state、四点pcap与qdisc计数定位最早断点，仅做最小修复并同条件复跑；若通过，固定该拓扑并把发生器扩为120s 30/60/30、3 seed × Normal/Game × lossless/20%/30%的18个独立job，同时加入规范要求的资源采样和互斥成本总账。
