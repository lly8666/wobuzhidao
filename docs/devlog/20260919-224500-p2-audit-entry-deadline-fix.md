# 20260919-224500 P2 审计返工：普通SYN入口与绝对建连期限

## 审计结论

基于HEAD 55d522f与P2最后Actions run 35446331156复核后，保留此前已通过的核心结论：
- P1 tlsrecord独立加密、乱序立即交付、有限近期去重不回退。
- P2真实TLS/uTLS exporter、受保护admission、prepare/detach、真实fallback、loss/no-HOL与hosted wire格式证据有效。

但P2“完成”结论撤回，补两个硬缺口：
1. 服务端初始SYN错误地用WBD固定MSS=1360/SACK/WS=8作为准入条件，普通内核TCP SYN可能在ClientHello之前被挡住。
2. TLS握手成功后helper清空deadline，admission读写可无限阻塞；context也没有覆盖整个候选生命周期。

同时明确外观能力边界：真实TLS + Firefox120 ClientHello不等于指定借用网站完整服务端握手指纹一致。

## 修复一：合法TCP入口与WBD身份分离

internal/faketcp/packet.go：
- 新增IsInitialSYN，仅判断合法初始SYN：SYN存在，ACK/FIN/RST/PSH不存在，无SYN payload，端口非零，MSS>0（若提供），WS<=14（若提供）。
- ECE/CWR允许存在，避免拒绝合法ECN SYN。
- IsWBDHandshakeSegment保留，但仅表示WBD客户端当前presentation，不再作为server admission条件。
- MarshalSegment改为按Segment实际SYN options序列化；raw WBD helper MarshalIPv4TCP仍保留固定客户端persona。

internal/faketcp/association.go：
- NewServerAssociation改用IsInitialSYN。
- 记录PeerTCPProfile。
- 对端未发送MSS option时按IPv4 TCP默认MSS 536处理。
- server BootstrapStream chunk = min(DefaultBootstrapChunk=1200, peer effective MSS)。
- SYN-ACK始终发送server MSS=1360；SACK-permitted只有peer提出时才返回；Window Scale只有peer提出时才返回，server自身scale仍为8。

不改变WBD客户端当前发包外观。

## 修复二：一个绝对候选建连期限

新增internal/realityfront/candidate_deadline.go：
- Timeout默认10秒，但在EstablishClient/EstablishServer/HandleServerAssociation中解释为整个候选建连的绝对deadline。
- 若context deadline更早，则取更早值。
- context取消会立即把net.Conn deadline推到当前时间，唤醒阻塞I/O。
- 子阶段只拿Remaining()，不会每进入TLS/admission重新获得完整timeout。
- TLS helper内部成功后会清deadline，因此外层guard立即Rearm回原绝对deadline。
- 只有成功handoff才清deadline；失败路径保留deadline并关闭conn/association。

Client覆盖：
TLS -> admission request -> final reply -> exporter全部在同一绝对期限；任何失败关闭底层conn。

Server覆盖：
ClientHello -> TLS -> partial/full admission -> exporter -> PrepareTransition -> final TLS reply等待FakeTCP ACK -> DetachTransition全部在同一绝对期限；失败关闭association，若已prepare则现有AbortTransition清buffer。

普通访客在ClientHello被判定unrecognized后，不再属于WBD候选：清除候选deadline，然后进入fallback自己的DialTimeout/SessionTimeout。

## 新Actions测试

internal/faketcp/syn_compat_test.go：
- 普通MSS1460/WS7/SACK SYN可建立。
- 无TCP options SYN可建立，effective peer MSS=536。
- MSS600/不同WS/无SACK可建立。
- SYN-ACK option serialization按协商结果，不强塞SACK/WS。
- bootstrap server payload严格受peer MSS限制。
- malformed SYN组合仍拒绝。

internal/realityfront/audit_closure_test.go：
- 上述普通SYN profile可继续发送非WBD Firefox120 ClientHello并实际走到fallback DialContext。
- TLS已完成且server读完认证请求后沉默：client在绝对deadline内退出并关闭连接。
- admission等待期间context cancel：立即退出并返回context.Canceled。
- client只发送半个admission：server deadline内关闭candidate且不prepare transition。
- server已Prepare并发最终admission回复，但FakeTCP ACK永远不来：deadline触发，transition变Aborted，association关闭。
- 成功handoff后client transport的candidate read/write deadline均已清零。

## 方案文件调整

ROADMAP/ACCEPTANCE/DEVELOPMENT_PLAN/MODULE_MAP/PROJECT_CHARTER/README同步：
- P2审计返工完成前暂停P3。
- 普通合法SYN准入与WBD身份识别分层。
- 一个绝对候选建连期限成为P2硬验收项。
- hosted TLS/persona不再表述成“指定借用网站完整握手外观已验收”。
- 真实网站服务端ALPN/扩展、报文长度/分段、会话恢复相似性留到平台I/O后普通浏览器+真实抓包验证。
- Session Tickets继续禁用，不为外观破坏prepare/detach。

## 不变项

- 不修改P1 record crypto/no-HOL。
- 不增加第二公开连接或READY/COMMIT握手。
- 不打开Session Tickets。
- 不提前迁移P3 FEC/LINK或平台raw/Npcap。
- old归档不修改。

## 退出条件

新精确SOURCE_SHA必须：
- repository-contract PASS
- Windows go list/go test/go build PASS
- Linux go list/go test/go build PASS
- Linux race PASS
- 既有tlsrecord directed fuzz/reference PASS

通过后才重新把P2标为完成并恢复P3 linkdata下一任务。
