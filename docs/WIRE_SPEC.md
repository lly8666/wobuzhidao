# TLS-like Record V1 — 已决定的实现规范

此规范是新产品唯一稳态 wire。开发 agent 不再比较不同加密选项。使用成熟 Go ChaCha20-Poly1305 和 ChaCha20 primitive；这不意味着采用标准 TLS 的按序接收器。

## 1. 会话建立与 keys

继续现有真实 TLS/Reality-like 建连及账户 admission。在受保护的应用请求/应答中使用新协议版本，携带 record_version=1、双方 record 上限、服务端生成的 16 字节 incarnation nonce，以及 `lane_id(u8)`。lane_id 固定为 1..4，由客户端在 TLS 内请求、服务端原值回显；0/越界或回显不一致明确拒绝。它只用于把每条同 TunnelID 的独立 FakeTCP association 绑定到权威 logical lane / same-ID replacement，不增加公开 SYN/TLS 标记，不恢复额外控制信道。未知版本明确拒绝，无 DTLS fallback，无 both 模式。

受保护的参数采用明确字节长度与网络字节序。每方向最大记录大小分别计算。TLS-like exporter 上下文保持既有固定编码，不把 lane_id 加入静态向量：`version(u16) || incarnation_nonce(16) || TunnelID长度(u16) || TunnelID字节 || client_limit(u16) || server_limit(u16)`。TunnelID 使用既有规范编码，不依赖 JSON 字段顺序。

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
  kind           1 byte (0x00=LINK datagram; 0x01=health in admission V2)
  payload         N bytes
  inner_type=0x17 1 byte
  padding         0..P 个 0x00
```

V1 发送端默认 padding=0，既有 Seal 与固定向量保持不变；P3 新增显式非零 padding 编码能力，接收端支持合法尾部零填充。此为计划中的 API 能力，不表示当前实现或生产默认已启用。padding 加在 inner_type 之后并纳入 AEAD，不另加明文长度字段、不改变 version/kind/PN/AAD。没有 FRAGMENT/ACK/NACK/RETIRE record kind。admission V2 新增独立 health kind=0x01，业务控制仍走既有 LINK。未知 kind 只丢本记录并计数。

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

P3 只有一个预算来源。operator-visible `connection_mtu` 合法范围固定为 576..9000；可再受更小的本地实际 packet ceiling 约束。steady-state 每方向按实际序列化 IPv4/TCP header 长度、peer effective MSS 和受保护 admission 协商出的 record wire limit 推导，不再有隐藏的 1360/1400/1500 cap。

IPv4 当前公式固定为：

```text
effective_packet_mtu = min(connection_mtu, local_packet_mtu_if_smaller)
packet_payload_mtu   = effective_packet_mtu - ipv4_header_len - tcp_header_len

peer_effective_mss   = advertised_mss
                     = 536 when peer omitted MSS on IPv4

carrier_payload_mtu  = min(packet_payload_mtu, peer_effective_mss)
record_wire_mtu      = min(carrier_payload_mtu, negotiated_record_wire_limit)
record_payload_mtu   = record_wire_mtu - 31

FEC off:
  link_frame_mtu             = record_payload_mtu

FEC 20:4/8/10/12/16/20:
  link_frame_mtu             = record_payload_mtu - 56

link_fragment_payload_mtu    = link_frame_mtu - 20
```

其中 31 是 `tlsrecord.FixedWireOverhead`，56 是 FEC v1 header，20 是 `WBDLFRG1` fragment header。所有固定 FEC 挡位的 MTU 开销相同；R 只改变 repair 数量，不改变 shard header 长度。实际 header 长度必须是合法的 IPv4/TCP 4-byte 对齐长度；当前普通 data segment 没有 SYN options 时是 20+20，但预算 API 不把 40 写死。

可选 padding 不改变上述无填充业务容量：`padding_headroom = record_wire_mtu - 31 - len(actual_record_payload)`。显式编码 API 对负值或超过 headroom 的 padding 请求明确报错；策略层在调用前按 headroom、每包上限和累计预算选择合法值，无预算用 0，不等候。基础 payload 本身超限仍按原规则拒绝，不能靠 padding 降级掩盖。`actual_record_wire_len = 31 + len(payload) + padding_len`，必须满足全部限幅。padding 不强迫 LINK 多切一片，不改变 FEC shard 长度；同 Seq 重传原密文和原 padding。

`negotiated_record_wire_limit` 必须落在 TLS-like V1 可表示范围内；若任一限幅后不足以容纳 TLS-like 固定开销，或扣除启用 wrapper 后 `link_frame_mtu <= 20`，配置阶段明确拒绝。不得生成零/负 fragment payload。

LINK 业务分片只发生一次，在 FEC 之前完成。FEC 开启时每个 LINK fragment frame 是一个 systematic source shard；FEC wire 必须完整放进一条 TLS-like record payload，record wire 又必须完整放进一个 FakeTCP payload。记录层不分片，TLS-like 路径不调用旧 CarrierFragmenter，不发送 WBDFRAG1。oversize 必须提前分片或明确拒绝，后续合法业务继续。

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

### 5.2 FEC v1 wire 与全部固定挡位

P3 只复用归档 live LINK 路径实际使用的 FEC v1，不启用归档中另行存在的 profile-v2 试验 wire。固定集合为：FEC off，以及 TailRS 的 20:4、20:8、20:10、20:12、20:16、20:20。K 固定为 20；R 只能取 4/8/10/12/16/20，其他几何明确拒绝。

FEC 挡位是一个 lane incarnation 的不可变建立参数：encoder、decoder、统一 MTU budget 与后续 owner 必须持有同一个 path-level parity 配置，构造时不一致即失败；不得在现有 BlockID/FEC block 中热切换。path-level `ParityShards=0` 只表示 **FEC off**。归档 FEC v1 header 中历史 `parity=0` 的兼容解释仍仅属于 header parser 的既有 20:20 语义，不能被新 owner 当作 off，也不能据此创建第二套 profile 表。P4 如需改挡位，应通过候选 lane/incarnation 替换旧 lane，本阶段不做自动调档或挡位选型。

FEC 开启时，每个 LINK fragment frame 作为一个 systematic source shard；FEC 发生在 LINK 分片之后、TLS-like record 之前。FEC shard datagram 头固定 56 字节：

```text
bytes 0..1   magic="WF"
byte  2      version=1
byte  3      reserved=0
bytes 4..7   BlockID u32 big endian
byte  8      shard_index
byte  9      data_shards=20
byte 10      parity_shards=R
byte 11      data_count
bytes 12..13 shard_size u16 big endian
bytes 14..15 flags u16 big endian
bytes 16..55 original_lengths[20]，每项 u16 big endian
payload       shard_size bytes
```

flags bit0 为 `streaming_systematic`。systematic source 在到达 encoder 时立即发送，不等待 20 个 source 或 flush timer；此时 `data_count=20` 是 provisional 占位，`shard_size` 等于本 source 长度，只有自己的 `original_lengths[index]` 非零。最终 parity shard 的 flags=0，并携带该 block 权威的 data_count、最大 shard_size 和 original_lengths。

full block 发送 R 个 parity。partial block 有 N 个 source 时只发送 `min(N,R)` 个 parity；unused systematic slots 作为已知零 shard，因此不得恢复旧的 “N + R(固定20)” 放大行为。所有挡位使用同一 systematic 20+20 generator 的前 R 个 parity rows，20:20 的既有数学/wire 行为保持不变。

BlockID 只在一个 lane/path FEC 实例内标识 generation，不从 record PN、LINK PacketID 或 TCP Seq 推导。decoder 按 association 已确定的 R 精确配置；收到不同 R、非法 header/reserved/flags、长度不一致或 active state 中同 shard identity 的 conflicting payload 时拒绝该 shard，不覆盖 first arrival。相同合法 shard 重复到达幂等。FEC state 不跨 lane 共享。

heavy reconstruction state 的恢复期限从该 BlockID 第一次进入 heavy decoder state 起固定为 3 秒绝对 deadline；后续 source/parity/duplicate 进展不刷新。owner 必须在停流时仍主动执行 expiry。deadline 到期后释放 heavy parity/reconstruction payload，保留 bounded compact first-delivery 状态；之后迟到 systematic source 仍可 first-deliver 一次，但不重新开启 parity reconstruction。compact retired state 固定最多 8192 个 BlockID；full/heavy block 数量由调用方 maxBlocks 明确限制。

FEC encoder 返回的内部 wire backing slot 可以复用，因此进入统一数据面前必须明确所有权。当前 LINK/FEC 适配层返回 owned immutable wire copies，保证一个大 datagram 跨越多个 20-source block 时，先前待发送 wire 不会被后续 Add 覆写。

## 6. 必须固定的测试向量

提交实现时同时提交 exporter/HKDF 方向派生、PN=0/1/跨32位/接近64位上限、空或最小 payload、多条 record、位翻转、深度乱序与重传一致性向量。expected bytes 应有独立计算/审阅来源，不能用待测函数动态生成期望再声称验证。

参考：[RFC 8439](https://www.rfc-editor.org/rfc/rfc8439.html)、[RFC 8446 §5/§7.5](https://www.rfc-editor.org/rfc/rfc8446.html)、[RFC 9001 §5.4](https://www.rfc-editor.org/rfc/rfc9001.html#section-5.4)。本 wire 是自定义独立数据报协议，格式外观参考 TLS，不是标准 TLS record protection。

TLS启动填充可选策略（2026-09-22）：只改变既有加密内padding长度，不新增record kind/协议字段/协商。默认off；业务检测在FEC前，实际padding在source record seal前，不进入FEC或original_lengths，不增加record/分片；parity与重传不得重新随机。固定预算与旁路条件见TLS_STARTUP_PADDING.md。


## Lifecycle V2（2026-09-23，待 Actions 验收）

真实 TLS 握手/移交流程复用现有实现；TLS 内 admission RecordVersion 从 1 升为 2，V1 显式 version failure，两端须成对升级。V2 exporter context 中 Version=2；record 外层格式、PN/AEAD/HP/MTU和LINK/FEC格式不变，既有纯 record 固定向量保留。

kind=0x01 的加密 payload 固定 9 字节：第 0 字节 health schema=1；随后 uint64 big-endian 为本端距最近业务活动的毫秒数（0～7天，发送端封顶）。记录总 wire 40B。PN 仍使用同 lane 的唯一单调分配器，receive first-arrival 不依赖 PN 连续。必须认证成功且结构合法，过大 idle 值拒绝。peer idle hint 仅接受比上次 hint 更新的 PN；不要求按顺序交付业务。近期重复不刷新 liveness；不是密码学无限历史反重放保证。

health 不经过 LINK/FEC、padding、不建立业务 flow、不补充或消耗专门 repair 队列；正常 pending ACK 元数据有既定界限和到期回收。客户端进入稳态后开始发送；服务端必须先收到对端首条有效加密记录，防止新数据面记录进入对端未完成的 TLS bootstrap。每端主动定时发送，不用 ping/pong 响应放大。自动休眠前允许每lane一次最终 idle hint。

missing health = UNKNOWN，不能推出业务空闲。只有本端业务空闲且所有 active lane 最近 <=2个本端发送间隔收到 peer idle>=idle-dormant 的有效提示才自动休眠；活动快照二次校验防竞态。超时失活与 DORMANT 为不同状态。默认15s/90s参数、有限重连、诊断与验收详见 PARAMETERS 和 LIFECYCLE_ACCEPTANCE。
