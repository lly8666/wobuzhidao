# TLS-like Record V1 — 已决定的实现规范

此规范是新产品唯一稳态 wire。开发 agent 不再比较不同加密选项。使用成熟 Go ChaCha20-Poly1305 和 ChaCha20 primitive；这不意味着采用标准 TLS 的按序接收器。

## 1. 会话建立与 keys

继续现有真实 TLS/Reality-like 建连及账户 admission。在受保护的应用请求/应答中使用新协议版本，携带 record_version=1、双方 record 上限、服务端生成的 16 字节 incarnation nonce。未知版本明确拒绝，无 DTLS fallback，无 both 模式。

受保护的参数采用明确字节长度与网络字节序。每方向最大记录大小分别计算。上下文固定编码：`version(u16) || incarnation_nonce(16) || TunnelID长度(u16) || TunnelID字节 || client_limit(u16) || server_limit(u16)`。TunnelID 使用既有规范编码，不依赖 JSON 字段顺序。

双方在原 TLS/uTLS 对象上调用 exporter，label `EXPORTER-WBD-TLSLIKE-V1`，context 为上述编码的 SHA-256，输出 32 字节 master。禁止从手工复制的不完整 tls.ConnectionState 派生。

HKDF-SHA256，以 master 为 IKM、incarnation nonce 为 salt，info 分别为固定 ASCII `WBD-TLSLIKE-V1/c2s`、`WBD-TLSLIKE-V1/s2c`，每方向输出 76 字节：前 32 为 K_aead，中间 12 为 IV，后 32 为 K_hp。所有 keys 仅保存在进程内，不写日志/临时配置文件。

## 2. 格式

```text
outer:
  type=0x17       1 byte
  version=0x0303  2 bytes
  body_length     2 bytes, big endian
  protected_PN    8 bytes
  ciphertext      variable (包含 16 字节 tag)

plaintext:
  kind=0x00       1 byte (LINK datagram)
  payload         N bytes
  inner_type=0x17 1 byte
  padding         0..P 个 0x00
```

V1 发送端 padding 固定为 0；接收端支持合法尾部零填充。没有 FRAGMENT/ACK/NACK/RETIRE record kind。控制流量通过已有 LINK 控制数据报承载。未知 kind 只丢本记录并计数。

每个方向独立完整 uint64 PN，从 0 开始；仅新建记录消耗 PN，失败编码可跳号但绝不重用。PN 不从 TCP Seq、Game PacketID 或 FEC BlockID 推导。使用完最大编号后必须拒绝新建记录并触发现有 lane replacement，不回绕。

```text
nonce = IV XOR (四个零字节 || uint64_be(PN))
AAD = outer_header[5] || uint64_be(PN)
C = ChaCha20Poly1305.Seal(nonce, plaintext, AAD)

sample = C[0:16]
mask = ChaCha20(K_hp, nonce=sample[4:16],
                counter=uint32_le(sample[0:4])) 输出的前 8 字节
protected_PN = uint64_be(PN) XOR mask
```

这是参考 ChaCha20 header protection 的 WBD 固定 8 字节包号封装，不声称兼容 QUIC。键分离，sample 包含 tag 的 ciphertext 总长足够；接收先恢复 PN，再构造 AAD/nonce 并 open。没有 expected PN、猜号循环或前包依赖。

最小额外开销 31 字节（5+8+16+1+1），不含内层头。body 最大 min(协商上限减5, 2^14+256)，最小 26 字节。更小的业务容量/最小 LINK 报文要求由统一预算提前校验。

## 3. 边界与重传

一条 record 完整位于一个 FakeTCP payload。接收支持 1..N 条完整记录，首版默认每 payload 一条，不引入合并等待。若组合多条，先确认总长不超预算。

FakeTCP repair 缓存最终不可变 wire bytes；同一 TCP 序列区间重发同一内容，不再次 seal。记录密文中不得增加明文 WBD magic。随机密文偶然包含某串字符不是错误。

包号、明文、keys 与 wire bytes 的生命周期独立。编码完成后 wire 只读；在重传/IO 未完成前不得归还 buffer 池。

## 4. 解码与重复

长度检查 -> PN unprotect -> open -> inner type/padding 检查 -> 近期精确去重 -> LINK。认证/结构失败不推进有效 PN 状态、不修改 FEC、不影响下一 payload。

V1 采用每方向最多 65536 项的精确近期 PN 集合，按插入顺序淘汰，无每包全量扫描；这是抑制近期重复的缓存，不是接收窗口。不因 PN 小于历史最高值或缓存范围而丢未知合法包。记录 cache eviction/duplicate/late 指标；先按此固定值实现，不做参数选型。

历史缓存外的重复可能到达上层。LINK 控制幂等、FEC 重复 shard 和 Game/业务去重必须各自测试，不能承诺无限期 exactly-once。后续必要修复不能通过拒绝旧 PN 重新制造迟到首次到达丢失。

长度不能可信确定时丢当前 payload 剩余部分，不跨 payload resync；长度合法而本 record 校验失败可跳过本 record 处理下一条。解析循环按输入长度受限。

## 5. MTU 和分片

最终 outer MTU 扣实际 IP/TCP 头、会发送的选项，并受 peer MSS/接收限制约束，得到 record wire 上限，再扣 31 字节得到 LINK/FEC datagram 上限。FEC parity 最大包同样必须装得下。

使用既有 LINK 业务分片，在 FEC 之前完成。记录层不分片，TLS-like 路径不调用旧 CarrierFragmenter，不发送 WBDFRAG1。oversize 必须提前处理或明确拒绝，后续合法业务继续。

### 5.1 LINK 单数据报分片 wire

P3 复用归档既有 LINK 业务分片，分片发生在 FEC 之前；普通不分片数据报不增加 LINK 头。保留的 fragment envelope 为：

```text
magic="WBDLFRG1"  8 bytes
version=1         u8
flags=0           u8
fragment_index    u16 big endian
fragment_count    u16 big endian
original_length   u16 big endian
PacketID          u32 big endian, non-zero
fragment_payload  1..N bytes
```

固定边界：fragment header 20 字节；原始单数据报最大 65535 字节；fragment_count 为 1..65535；每个 fragment frame 不得超过当前 LINK datagram/MTU 上限。PacketID 仅标识该 lane/path reassembler 内的一个逻辑数据报，发送端递增并在 uint32 回绕时跳过 0。普通 payload 若以保留 magic 开头，即使本来不超 MTU，也必须用 count=1 或必要的多片 envelope 转义，避免被接收端误判。

重组按 PacketID 独立进行，不存在 expected PacketID 或跨数据报接收窗口。乱序允许；相同 index 且 payload 完全相同的重复片幂等忽略；相同 PacketID 的 count/original_length 冲突，或同 index payload 冲突，明确判无效且不得覆盖已经保存的首次片。一个 incomplete 数据报不得阻塞另一个完整数据报交付。

首版资源语义固定为：最多 16 个 incomplete assemblies；单 PacketID 最多 65535 个 fragments；全 reassembler 最多保存 65535 个 fragment entries 和 `16 * 65535` 字节 fragment payload；assembly 绝对 TTL 5 秒；completed/expired/evicted PacketID 最多保留 64 项、retired TTL 10 秒。fragment entry 使用按实际到达稀疏分配，声明 count=65535 本身不能触发 65535 项预分配。owner 必须能在没有新流量时主动执行 expiry，不能把退役依赖在下一包到达。

## 6. 必须固定的测试向量

提交实现时同时提交 exporter/HKDF 方向派生、PN=0/1/跨32位/接近64位上限、空或最小 payload、多条 record、位翻转、深度乱序与重传一致性向量。expected bytes 应有独立计算/审阅来源，不能用待测函数动态生成期望再声称验证。

参考：[RFC 8439](https://www.rfc-editor.org/rfc/rfc8439.html)、[RFC 8446 §5/§7.5](https://www.rfc-editor.org/rfc/rfc8446.html)、[RFC 9001 §5.4](https://www.rfc-editor.org/rfc/rfc9001.html#section-5.4)。本 wire 是自定义独立数据报协议，格式外观参考 TLS，不是标准 TLS record protection。
