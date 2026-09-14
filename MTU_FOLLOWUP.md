# MTU 后续修复说明（不改变已发布tag）

适用基线：v0.0.0-preview.20260914 / b8ddd5a48a0c1d7dbed0e593004fe290edd86aba。

## 结论

发布时的失败不能证明最近测试中的MTU损坏复发。具体失败来自旧测试把1461和1501硬编码为非法inner MTU；当前协议表示范围已经扩大，这两个值在该函数验证的范围内合法。最近fullstack无完整性错误，只覆盖实际测试的连接/包长组合，不能证明任意协商值都安全。

## 工作一：修正过期测试，不恢复旧硬上限

文件：cmd/wbd-link-server-mux/mtu_policy_test.go。
TestLinkPolicyForInnerMTURejectsOutOfRange 目前测试575、1461、1501。
而linkPolicyForInnerMTU使用gamepath.InnerMTUBounds；该函数由control.CurrentLinkPolicy和40字节Game/WBDP开销推导。当前MaxLinkMTU为65535，返回的inner范围为576..65495。

建议：
- 非法输入测试使用minInner-1、maxInner+1，另加0和负值。
- 合法测试覆盖minInner、maxInner、1461和1501，明确这是兼容配置/协议表示范围验证，并不承诺这些值适用于connection MTU1500。
- 不为了测试绿色把上限改回1460或1500。
- 修订函数注释和CLI帮助，区分connection MTU、LINK明文MTU、inner IP MTU。

## 工作二：审计并补齐协商值与承载预算的连接

已确认的代码事实：linkPolicyForInnerMTU返回接近协议表示上限的通用policy；其本身不读取carrier/interface MTU，也不根据本次FEC模式推导实际承载上限。pathmtu.Derive有产品范围576..9000和分层预算，但不能假定所有独立CLI或远端LINK_INIT都经过它。

需要新agent逐条确认LINK_INIT → policy验证 → association激活的调用链。如果没有其他实际路径预算校验，补在激活前，不要等数据发送时才出现packet too large。这个设计缺口不等于已经在现有1500测试中复现故障。

要求：
1. 统一从连接的实际/配置承载预算推导，复用pathmtu.Derive；不要复制一套减常数公式。
2. 根据连接启用的FEC/Game封装计算。共享server按关联校验，不用一个全局inner默认值覆盖所有客户端。
3. 区分协议表示边界和传输边界。65535能放进字段，不表示再加FEC/DTLS头后仍能放入UDP数据报或单carrier。
4. 超预算提议在协商期明确拒绝。若现有协议允许协商更小值，必须由双方确认新值；禁止服务端静默缩小或截断。
5. 若当前server没有每关联连接预算信息，先明确从哪里取得：本地承载能力/配置和已有协商信息。不能凭空推断远端路径MTU；不要为此直接更改wire format，先提出最小兼容方案。
6. 独立CLI与产品启动器的参数含义保持明确，不把inner1360直接当connection1360，也不把connection1500直接传给TUN或FEC source。

对于connection1500、当前IPv4/TCP carrier、FEC20:20、Game启用：

| 层 | 预算 |
|---|---:|
| connection | 1500 |
| carrier payload | 1460 |
| DTLS plaintext | 1428 |
| LINK plaintext | 1372 |
| inner IP | 1332 |

DTLS的32字节是当前实现预留，不是所有密码套件和record配置的普适常量。必须保留真实native DTLS路径的边界验证。

## 工作三：超限数据不能悄悄破坏已建立通道

审计server serviceLoop：当前Outbound错误后return结束后端读取循环。一个超出已协商预算的后端包是否应终止整个后端循环，应明确约定并测试。
建议对已识别的“单个应用包超预算”按产品策略计数、丢弃并继续处理后续合法包，或明确关闭关联并通知对端；不能留下看似active但后端收包循环已退出的状态。
不要把所有FEC解码/头部不一致错误一概吞掉，也不要用扩大MTU掩盖ownership或framing错误。

## 最小验收

- 修正后的范围测试及existing动态MTU协商测试通过。
- 对当前1500/FEC/Game预算：LINK1372允许，1373拒绝；FEC关闭按对应Derive结果测试。
- 两个不同合法预算的关联同时存在，互不修改对方配置。
- 协商期超限拒绝；后端发送超限包后再发合法包，行为符合明确策略，不产生隐性死通道。
- 经过真实FEC+native DTLS+FakeTCP验证边界包只有一个carrier；不以伪造固定DTLS头的单测替代。
- 不仅测试MTU1500，也测试一个较小的可承载连接预算和一个明确支持的大MTU环境。产品范围576..9000并不保证所有封装组合在整个范围都可行，应以Derive是否成功为准。

提交分开：过期测试修正、协商预算/超限处理、FEC生命周期。保留发布tag，后续修复从新分支提交；不要把文档补充称为源码已经修好。
