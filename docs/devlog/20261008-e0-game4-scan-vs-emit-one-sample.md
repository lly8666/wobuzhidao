# E0：精确SOURCE冻结Game4 TCP scan/emit单样本（2026-10-08）

## 检验门通过，才独立发测
目标分支 `next/performance-efficiency-20261008`，本提交父SOURCE `408a5cfb44883d959073c4d41c07c23a767a72db`。该产品代码只增加`TCPServer.TickTimed`可选scan/emit/abort/snapshot诊断与七项计数；已有 profile-OFF 的`TCPServer.Tick`、`Server.Tick`完全原样保持。其[基础 Actions 37788321913](https://github.com/lly8666/wobuzhidao/actions/runs/37788321913) 已检验repository contract、Linux/Windows Go unit、Linux race、directed fuzz、arm64交叉编译、Windows Npcap hosted、Linux共享TUN nft/iptables和OpenWrt TPROXY privileged/内核fallback全部success；[lifecycle37788321939](https://github.com/lly8666/wobuzhidao/actions/runs/37788321939)success。不是“提交即PASS”。

## 唯一性能样本
本提交只将性能 workflow 的`PRODUCT_SOURCE`冻结为产品SHA `408a5cfb44883d959073c4d41c07c23a767a72db`，唯一 `.github/efficiency-e0-sample.json`设为Game4、4 lane、每方向逻辑3Mbps、TCP/UDP/HTTP(S)真实业务、0%外层loss/300ms单向、300s+3s真实netem drain、FEC20:20、padding0、自动record cap 0、outer1400、profile ON、seed1821。每个Actions run**一个**性能sample，不引入同run AB或matrix；新helper SHA由此次提交的`GITHUB_SHA`绑定。验证真实Linux TPROXY client/server encrypted raw 与real shared TUN。产出`tick_tcp_flow_snapshot/scan/emit/abort`wall总时间与max>10ms、flow/due frames/abort数，与原game实际复制/去重、ready队列丢弃、C2S/S2C lossless UDP与小probe及TCP/HTTPS hash、CPU/PPS/alloc/PSI/netem各层合读。

前一正式[Game4 run37782210096](https://github.com/lly8666/wobuzhidao/actions/runs/37782210096)原始FAIL：TCPServer.Tick每轮max225.24ms、总16.5756s，服务端ready overflow268478段、C2S UDP1555 missing、小探针两向22/24未回、另socket drop37。旧Game4其他FAIL同样保留。profile ON额外计时不属于真实CPU改善，若本条用户态无损偶尔正常也不能删前次FAIL。下一步先读取本条Actions原始结论，再定点判断scan和emit是否支持一个真正不增加跨业务HOL、等待/无限队列且不削弱Game first-arrival、TCP reordering/repair保障的最小修复候选；任何产品优化先unit/race再四个独立Normal lossless/5205、Game4 lossless/5205门。未过不写E1完成。E6/P6/physical未实施，原80秒S2C E7仍OPEN。

详细身份与参数：[机器证据](../evidence/performance-efficiency-e0-game4-scan-emit-sample-candidate-20261008.json)。
