# 20260919-204500 P2 Real Decoy Fallback

## 原子目标

补齐ACCEPTANCE P2明确要求的真实fallback，不进入P3。

只提取“ClientHello已经由调用者消费/分类之后”的decoy continuation：
- 固定SNI校验
- 固定target dial
- 已消费raw ClientHello逐字节replay
- 同一入站FakeTCP BootstrapConn上的双向splice
- 可选session timeout和每方向byte limit

不迁移demo observer、CLI、ticket、lease或steady-state数据面。

## 单入口与一次分类

新增HandleServerAssociation：

1. 从ServerAssociation.BootstrapConn只调用一次ReadHello。
2. hello.Recognized=true：
   - 调establishServerRecognized；
   - 继续protected admission/exporter/prepare/detach；
   - 绝不调用fallback DialContext。
3. hello.Recognized=false：
   - 不进行TLS takeover；
   - 不PrepareTransition；
   - 把同一Hello.Raw直接交FallbackFromHello；
   - dial decoy、先完整write raw hello，再开始双向copy。

EstablishServer保留recognized-only兼容入口；内部抽出establishServerRecognized，使单入口不会二次读取ClientHello。

## 复用来源

登记：
old/internal/realitymirror/handle_from_hello.go
-> internal/realityfront/fallback.go

保留：
- caller已读Hello后继续镜像；
- fixed target/fixed SNI；
- raw hello先replay；
- 双向copy；
- 半关闭尝试；
- benign EOF/closed/reset/pipe错误；
- 可选byte limit和session deadline。

不迁移observer callback/witness发布。

## 新测试

TestUnrecognizedHelloFallsBackOnSameAssociationWithExactReplay：
- client用错误route key生成真实Firefox120 uTLS hello；
- server route key因此不recognize；
- fallback DialContext注入net.Pipe decoy；
- decoy独立再次读取ClientHello并保存raw；
- decoy把raw replay到真实crypto/tls server完成TLS1.3；
- client经同一FakeTCP association完成TLS、写"ping"并读回"pong"；
- decoy捕获raw必须与fallback入口保存的raw逐字节一致；
- fallback路径TransitionState必须不存在。

TestRecognizedAssociationNeverDialsFallback：
- recognized WBD通过同一HandleServerAssociation进入protected admission；
- injected fallback dial若被调用即报错；
- dial count必须为0；
- exporter keys继续一致。

TestFallbackSNIMismatchDoesNotDialTarget：
- unrecognized hello但SNI与configured decoy identity不同；
- 在dial之前明确拒绝；
- dial count为0。

## 退出条件

新精确SOURCE_SHA必须同时通过：
- repository-contract
- Windows go list/go test/go build
- Linux go list/go test/go build
- Linux race
- 既有tlsrecord directed fuzz/reference

通过前不把fallback列为completed。
