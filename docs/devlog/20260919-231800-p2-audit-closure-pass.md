# 20260919-231800 P2 审计返工 Actions闭环

## 产品SOURCE_SHA

079a11a51a8afb7e977f5140d8cdcd21db1c81c5

Run：https://github.com/lly8666/wobuzhidao/actions/runs/35451109678

顶层结果：completed / success。

## 通过项

- repository-contract：PASS
- Windows 2022 go list / go test ./... / go build ./...：PASS
- Ubuntu 24.04 go list / go test ./... / go build ./...：PASS
- Ubuntu race：PASS
- 既有internal/tlsrecord directed fuzz：PASS
- independent P1 reference vector generator：PASS
- reference artifact upload：PASS

## 审计问题1：普通TCP SYN入口

已闭环并进入Windows/Linux unit及Linux race：

- ServerAssociation不再以WBD固定MSS=1360、WS=8、SACK-permitted作为身份准入。
- WBD固定SYN profile只保留为客户端presentation helper。
- 普通合法initial SYN可建立FakeTCP association并继续到ClientHello分类。
- 测试覆盖：
  - MSS=1460 / WS=7 / SACK
  - 无TCP options
  - MSS=1200或600 / 不同WS / 无SACK
- peer未报MSS时effective IPv4 peer MSS为536。
- server bootstrap chunk受peer effective MSS约束。
- SYN-ACK始终带server MSS，但SACK/WS只在peer提出时协商返回。
- 普通非WBD ClientHello实际走到fallback DialContext，证明入口不再在TLS之前被WBD SYN指纹挡住。
- cross-flow ACK隔离与malformed SYN拒绝仍保留。

## 审计问题2：TLS后admission绝对期限

已闭环并进入Windows/Linux unit及Linux race：

- Timeout在EstablishClient/EstablishServer/HandleServerAssociation中成为一个绝对候选建连预算。
- 预算覆盖：
  ClientHello -> TLS -> protected admission -> exporter -> PrepareTransition -> final reply FakeTCP ACK -> DetachTransition。
- TLS helper内部成功后即使清了deadline，外层guard也会rearm原绝对deadline，而不是重新计时。
- context cancel立即推动连接deadline到当前时间，唤醒阻塞I/O。
- 失败关闭client conn或server association；已prepare时transition会abort并清候选buffer。
- 只有成功handoff后清除候选deadline。

专项通过：
- TLS成功后server沉默
- context在admission等待中取消
- client只发半个admission
- server final admission reply发出但FakeTCP ACK永不回来
- 成功handoff后read/write deadline为zero

## 外观结论边界

本轮没有把hosted资格扩大解释。

现在可以准确说：
- 普通合法TCP入口可到ClientHello/fallback；
- 非WBD visitor可以byte-exact replay ClientHello到decoy；
- WBD使用Firefox120风格ClientHello和真实TLS 1.3；
- hosted TCP/IP/TLS格式已有serializer资格。

仍不能说：
- 已识别WBD的本地Go tls.Server在ALPN、服务端扩展、握手长度/分段、session resumption等方面已经与指定借用网站一致；
- 已完成真实网卡/Npcap抓包；
- 已完成普通系统浏览器经真实平台I/O的端到端访问验收。

Session Tickets继续禁用，不为外观破坏阶段切换。

## 阶段结论

审计提出的P2两个硬缺口已修并通过Actions，P2 hosted核心资格重新闭环。

STATUS恢复P3。下一原子任务仍是old/internal/linkdata单数据报fragmentation/reassembly最小闭包；不顺手混入FEC/MTU调参或平台I/O。
