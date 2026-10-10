# Admission V3（N0 阶段性实现；仅 fixed/off+ChaCha 接入）

V3 不向业务记录增加 mode/cipher 明文标记。受真实 TLS 保护的请求保留 WBAD/WBAL、15B 头、TunnelID/user/password、可选 WBAL 17B lease 扩展，头中 version=3，末尾附固定 10B 能力。成功应答保留 26B 头/TunnelID、可选 4B lease，尾部原样回显 10B 能力。无 JSON/可变能力长度；截断、未知版本、字节不一致都拒绝。V2 原有编码、向量不变。

能力字节顺序：
- 0 schema=1；1 transport=1 Normal/2 Game；2 desired_lanes=1 Normal/2..4 Game；
- 3 fec_mode=0 off/1 fixed/2 auto/3 auto-aggressive；
- 4 fixed_parity；5 initial；6 min；7 max；
- 8 cipher=1 ChaCha20-Poly1305/2 AES128-GCM/3 AES256-GCM；9 quality=0/1。

fixed parity 只能是 4/8/10/12/16/20，initial/min/max 必须为 0。off 的四个 parity 字段必须为 0。auto 只 Normal、fixed=0、initial=20、max=20、min=0/4/8/10/12/16/20。Game 不允许 auto。WBAL 的 desired_lanes 必须与能力字段一致。同一策略必须在第一条两向业务前绑定，不能等首个 FEC 包猜测。鉴权成功后才以策略建资源，错误组合失败不回显成功。

V3 exporter context = SHA256(原 V2 上下文原始编码但 version=3，后紧接 10B canonical policy)；exporter label 仍为 `EXPORTER-WBD-TLSLIKE-V1`。HKDF-SHA256 用原 exporter master 32B/nonce salt，info 为 `WBD-TLSLIKE-V3/<suite>/c2s` 或 `.../s2c`，suite 为 chacha20-poly1305、aes-128-gcm、aes-256-gcm。输出 32B AEAD、12B IV、32B HP，AES128 未来用首 16B。动态档位不进入 KDF、不得重置 PN。

**阶段性能力门**：目前 V3 只允许 fixed/off + ChaCha + quality=0，其他语法合法但返回 UNSUPPORTED；N2/N3/N1 运行代码成熟后才依次解锁。V2 保留历史夹具兼容；当前 Linux/Windows 产品 CLI 将既有 `fec-parity` 按 off/fixed+ChaCha 规范化为受保护 V3 请求，未新增自动档参数，服务端为各客户按回显值配置 lane，且共享 TUN 预留 FEC-on 上限。Windows GUI/JSON 仍需单独完整组合验收；多客户端真实业务与 Windows TUN 仍未完成 N0 资格，N0 IN_PROGRESS。

---


# TLS-like Record — admission V2 / 既有 record 封装规范

此规范是新产品唯一稳态 wire。开发 agent 不再比较不同加密选项。使用成熟 Go ChaCha20-Poly1305 和 ChaCha20 primitive；这不意味着采用标准 TLS 的按序接收器。

## 1. 会话建立与 keys

继续现有真实 TLS/Reality-like 建连及账户 admission。在受保护的应用请求/应答中使用新协议版本，携带 record_version=2、双方 record 上限、服务端生成的 16 字节 incarnation nonce，以及 `lane_id(u8)`。lane_id 固定为 1..4，由客户端在 TLS 内请求、服务端原值回显；0/越界或回显不一致明确拒绝。它只用于把每条同 TunnelID 的独立 FakeTCP association 绑定到权威 logical lane / same-ID replacement，不增加公开 SYN/TLS 标记，不恢复额外控制信道。未知版本明确拒绝，无 DTLS fallback，无 both 模式。

受保护的参数采用明确字节长度与网络字节序。每方向最大记录大小分别计算。TLS-like exporter 上下文保持既有固定编码，不把 lane_id 加入静态向量：`version(u16) || incarnation_nonce(16) || TunnelID长度(u16) || TunnelID字节 || client_limit(u16) || server_limit(u16)`。TunnelID 使用既有规范编码，不依赖 JSON 字段顺序。

双方在原 TLS/uTLS 对象上调用 exporter，label 保留 `EXPORTER-WBD-TLSLIKE-V1`（封装/KDF 标签不变；context version=2 区分本次能力），context 为上述编码的 SHA-256，输出 32 字节 master。禁止从手工复制的不完整 tls.ConnectionState 派生。

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

发送端默认 padding=0，既有 Seal 与固定向量保持不变；P3 新增显式非零 padding 编码能力，接收端支持合法尾部零填充。编码能力已实现，生产默认仍关闭；可用 tls-startup-padding 显式启用有限策略。padding 加在 inner_type 之后并纳入 AEAD，不另加明文长度字段、不改变 version/kind/PN/AAD。没有 FRAGMENT/ACK/NACK/RETIRE record kind。admission V2 新增独立 health kind=0x01，业务控制仍走既有 LINK。未知 kind 只丢本记录并计数。

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

当前采用每方向最多 65536 项的精确近期 PN 集合，按插入顺序淘汰，无每包全量扫描；这是抑制近期重复的缓存，不是接收窗口。不因 PN 小于历史最高值或缓存范围而丢未知合法包。记录 cache eviction/duplicate/late 指标；先按此固定值实现，不做参数选型。

历史缓存外的重复可能到达上层。LINK 控制幂等、FEC 重复 shard 和 Game/业务去重必须各自测试，不能承诺无限期 exactly-once。后续必要修复不能通过拒绝旧 PN 重新制造迟到首次到达丢失。

长度不能可信确定时丢当前 payload 剩余部分，不跨 payload resync；长度合法而本 record 校验失败可跳过本 record 处理下一条。解析循环按输入长度受限。

## 5. MTU 和分片

P3 只有一个预算来源。operator-visible `connection_mtu` 合法范围固定为 576..9000；可再受更小的本地实际 packet ceiling 约束。steady-state 每方向按实际序列化 IPv4/TCP header 长度、peer effective MSS 和受保护 admission 协商出的 record wire limit 推导，不再有隐藏的 1360/1400/1500 cap。

该预算是外层包约束，不要求所有平台TUN的接口MTU等于它。6181原生Windows读到TUN65535，Linux TUN1400；内层合法IPv4数据报可经LINK封装拆分，不改原数据报的DF语义。DF只约束IP分片。2026-10-05 M01在外层1400下覆盖完整内层包1399/1400/1401/1500/2000/4096/9000B与DF开/关；受控loopback往返不证明互联网目的PMTU或ICMP引用正确。外层实际1290/1340B的短窗口证据见physical-5min-6181db6-20261005.json，不改变本协议或65535数据报上限。

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

`negotiated_record_wire_limit` 必须落在 TLS-like record 可表示范围内；若任一限幅后不足以容纳 TLS-like 固定开销，或扣除启用 wrapper 后 `link_frame_mtu <= 20`，配置阶段明确拒绝。不得生成零/负 fragment payload。

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


## Lifecycle V2（2026-09-23，生命周期功能 PASS，目标速率性能 FAIL_CAPACITY_LIMITED）

真实 TLS 握手/移交流程复用现有实现；TLS 内 admission RecordVersion 从 1 升为 2，V1 显式 version failure，两端须成对升级。V2 exporter context 中 Version=2；record 外层格式、PN/AEAD/HP/MTU和LINK/FEC格式不变，既有纯 record 固定向量保留。

kind=0x01 的加密 payload 固定 9 字节：第 0 字节 health schema=1；随后 uint64 big-endian 为本端距最近业务活动的毫秒数（0～7天，发送端封顶）。记录总 wire 40B。PN 仍使用同 lane 的唯一单调分配器，receive first-arrival 不依赖 PN 连续。必须认证成功且结构合法，过大 idle 值拒绝。peer idle hint 仅接受比上次 hint 更新的 PN；不要求按顺序交付业务。近期重复不刷新 liveness；不是密码学无限历史反重放保证。

health 不经过 LINK/FEC、padding、不建立业务 flow、不补充或消耗专门 repair 队列；正常 pending ACK 元数据有既定界限和到期回收。客户端进入稳态后开始发送；服务端必须先收到对端首条有效加密记录，防止新数据面记录进入对端未完成的 TLS bootstrap。每端主动定时发送，不用 ping/pong 响应放大。自动休眠前允许每lane一次最终 idle hint。

missing health = UNKNOWN，不能推出业务空闲。客户端仅在本端业务空闲且所有 active lane 最近 <=2个本端发送间隔收到 peer idle>=idle-dormant 的有效提示时发起自动休眠；活动快照二次校验防竞态。服务端不能仅凭周期 idle health 抢先休眠，必须等待全部当前 authoritative lane 的 client PeerFIN（PeerWriteClosed）后跟随；保留65ff2ef的截止点竞态修复。超时失活与 DORMANT 为不同状态。默认15s/90s参数、有限重连、诊断与验收详见 PARAMETERS 和 LIFECYCLE_ACCEPTANCE。

### 2026-10-03：20:20有界长度分组（v1 wire不变）

20:20 encoder按LINK源实际长度选择最多3个固定payload ceilings256/512/SourceMTU组（去掉不低于MTU的额外组）。各组独立沿用20源/N部分源、N校验、首源8ms期限及即时systematic。BlockID由lane共享，在新block首源时分配，uint32自然回绕，不能因不同class复用ID；wire没有class字段，现v1 decoder直接接受交错block。source metadata仍provisional、parity携带该block最终长度表；组只优化校验长度，不改变接收端first-arrival/no-HOL/retirement。20:4/8/10/12/16输出维持原单组，off不受影响。
# 业务TCP代理的FIN位置（2026-10-03）

platformflow仅用于内层TCP代理，并非外层repair协议。TCPData.Payload按Offset字节定位；FIN在最后数据字节后占一个虚拟序列位置。接收方仅在所有前序字节完整并且FIN首次到达后发送ACK=数据末偏移+1；此前ACK=已收数据末偏移，不能释放未收到的FIN。发送方保留独立FIN到该ACK，部分数据ACK可以把带数据FIN缩成原Offset处空payload FIN重传。虚拟位置不增加wire bytes；FIN在已交付的重复数据上首次到达仍执行一次CloseWrite。此修复需双端同版本更新，不能混用此前零长度FIN与数据共享ACK位置的TCP代理端点。外层记录、FEC、Game去重、UDP和health framing不变。

## 2026-10-04：换代接收在途与发送权限分离

Promotion后新业务只由active generation生成并发送；old Ref的FenceOutbound/SendNormal仍失败，旧密文不得套新密钥/新TCP序列空间。接收允许当前active，或owner现有retiring集合中明确登记、尚未Retire的同身份incarnation。retiring有原CloseWrite/FIN及绝对关闭预算、10物理lane上限，不新增期限或候选接收权。记录由该incarnation自己的keys/PN去重/FEC/LINK处理；server lease源地址隔离和Game session/LaneID/共享PacketID去重不变。处理后再次检查接收授权，Retire/DORMANT/Close移除者拒绝；任意历史generation不可恢复。新数据无需等旧分片/缺口。wire字段、admission及用户配置均不变。

## 2026-10-04：受保护的自动IPv4租约扩展

自动请求以WBAL替代受TLS保护的原WBAD magic，其余V2请求头/TunnelID/username/password原布局不变，password后追加InstallationID[16]及DesiredLanes:uint8（1..4，LaneID不得超过它）。旧WBAD请求和回复字节不变。自动成功回复在原V2回复头及TunnelID后追加4字节网络顺序IPv4，客户端解释为/32；截断、unspecified或multicast拒绝。用户名密码验证失败不能调用分配器。记录Version=2与原exporter context、record/FEC/LINK/ACK稳态协议不变；不支持WBAL的端点明确拒绝，自动客户端不退回静态。

自动TunnelID取SHA256("wbd-installation-v1"+NUL+account+NUL+InstallationID)前16字节，account等于通过认证的username，设备ID不属于秘密认证。服务端在池内随机唯一分配，内存期限7天，认证重连续期；健康/retiring owner仍占用时不可复用。每逻辑Tunnel保持初次DesiredLanes，换代不能临时改变模式；不同Tunnel可以Normal1/Game4并存。全旧lane明确FIN或长期没有认证记录后，可在新LaneID1接入时安全关闭旧owner，重新按新DesiredLanes构造，设备/IP租约不变。到期租约回收使用同一安全detach，但只能由7天期限触发，不能把丢keepalive当业务idle。成功受保护回复地址先绑定尚未建业务的placeholder owner，再创建平台网络地址。健康旧lane遇到地址不同的新候选只拒绝候选；所有旧lane失活或DORMANT时要求清理并重建平台owner，不热换地址，不等待旧业务缺口。


## 2026-10-08 接口MTU静态选取（候选）

外层wire原有 `pathmtu.Derive` 公式不变。接口MTU新增 `max(576, configured_budget.LinkFragmentPayloadMTU)`，configured预算由已知配置outer IPv4/TCP头、record limit、FEC/LINK容量计算，而不是假装已知道全部lane的peer MSS。Windows Wintun用server→client的client-record-limit；Linux shared TUN用client→server的server-record-limit。实际lane按真实peer MSS、协商record和可得实际路径约束继续独立计算；静态TUN与有效单record大小不保证相同，如更小则LINK正常有界多片。逻辑IPv4包上限9000及UDP frame 8936不变，DF只约束IP碎片，不禁止LINK分片。静态预算不是路径自动探测或大UDP兼容证明，旧9000数据和FAIL需保留。


## 2026-10-08 外层 MTU 驱动 record/接口 MTU 自动上限（后续优化候选）

每端 CLI `client-record-limit` / `server-record-limit` 默认 **0=auto**，只在处理配置时求一次：
`record_wire_limit = min(configured_outer_ipv4_mtu - configured_outer_ipv4_header - configured_outer_tcp_header, tlsrecord.MaxWireLen, explicit_nonzero_record_cap_if_any)`。当前正常 data header 为 20+20；逐 lane 运行时仍由真实 MSS、实际头长、协商 record 及更低路径约束收紧。record 的 V2 受保护 admission 协商字段和格式不变；未启用新的 PMTUD 或动态共享 TUN 调整。两端 `--mtu` 建议一致，不一致时无双向单 record 保证。

内层 IPv4 包不可能以保留的 WBDLFRG1 8 字节魔数开头，因此能放进 `LinkFrameMTU` 的普通 IPv4 包是**无 LINK 分片头的裸包**。`DerivedInterfaceMTU = max(576, LinkFrameMTU)`，不应使用 `LinkFragmentPayloadMTU` 重复扣 20B；只有超出 LINK frame 后分片的各段才各带 20B 分片头。计算后的 record→TLS-like31B→FEC-on56B→LinkFrame/TUN 是同一原预算链，并不修改 wire、FEC、repair。举例外层1300/1400/1500、FEC off 的双方默认 record 是1260/1360/1460，静态 TUN 为1229/1329/1429；FEC-on 则 TUN1173/1273/1373。合法 IPv4 的 576 下限可能强制 LINK 分片，不作单 record 保证。明确配置比自动小的 record cap 生效；大于外层的 cap 被外层限幅。旧 9000 逻辑 IP 校验/8936 UDP 平台限制不因此改变；不能宣称原生大 UDP 或性能已达标。

## 2026-10-11 N1 V3 encrypted health-quality payload v2 — format RESERVED, not yet live

This is an **N1 format candidate and codec only**. It must NOT be emitted or
accepted by the current shipping connection while N1's rate-limited sender,
receiver, generation fencing, and baseline tests remain unqualified. The
current `tlsrecord.KindHealth` plaintext v1 is exactly 9 bytes:
`version(1)=0x01 | idle_milliseconds(8)`. Existing V1 parser rejects v2;
V2-admission and all currently running V3 fixed/off tunnels continue to use
only v1 until an explicitly coordinated N1 activation.

Proposed v2 body is exactly **104 bytes**, carried **inside authenticated
`tlsrecord.KindHealth` ciphertext** (not LINK/FEC and not an outer TCP header).
All fields network big-endian; no extra field per business packet:

| Offset | Size | Field | Rule |
|---|---:|---|---|
| 0 | 1 | health payload version | 2 |
| 1 | 1 | flags | bits 0 valid window, 1 loss **estimated**, 2 local capacity limited, 3 insufficient, 4 stale; all others reject |
| 2–3 | 2 | reserved | zero |
| 4–11 | 8 | peer business idle ms | 0..7 days, millisecond exact |
| 12–19 | 8 | report sequence | nonzero, monotonically per incarnation |
| 20–35 | 16 | lane incarnation nonce | nonzero; protected generation fence |
| 36–43 | 8 | lane generation | nonzero |
| 44–51 | 8 | PN window first | inclusive, big-endian |
| 52–59 | 8 | PN window last | inclusive, >=first if valid |
| 60–67 | 8 | successfully emitted local DATA records | no control; excludes local unsent/seal failures |
| 68–75 | 8 | authenticated first-unique received DATA records | excludes health itself |
| 76–79 | 4 | late received DATA records | bounded statistic, not a loss verdict |
| 80–83 | 4 | duplicate received DATA records | bounded statistic |
| 84–87 | 4 | FEC-recovered source shards | auxiliary, not first-wire loss |
| 88–91 | 4 | local dropped records | locally observed only |
| 92–93 | 2 | age ms | 65535 = UNKNOWN, not zero |
| 94–95 | 2 | trusted RTT ms | 65535 = UNKNOWN, not business p99 |
| 96–99 | 4 | estimated missing records | only valid with bit1 and nonempty window |
| 100–101 | 2 | max missing burst | <= estimated missing |
| 102–103 | 2 | reserved | zero |

Window-invalid summaries must zero PN bounds, data counters and loss/burst
fields; they can still carry local-drop and age/capacity/insufficiency/staleness
information. Invalid version, length, reserved bits, flags, zero
incarnation/generation/sequence, invalid PN range, missing window count,
or inconsistent missing/burst flags cause a hard decode error with no partial
state publication. Nonce/generation/report sequence are checked against the
**currently authenticated lane incarnation** at receive; the codec alone does
not do live authentication and may never authorize a new lease/lane.

The `estimated missing` field is never labelled a precise WAN loss rate:
PN can skip local sends, control consumes PN, RTT/feedback may be stale, the
last missing tail may be invisible, and an older repair can arrive after the
reorder window. Only aligned successful sender watermarks + mature receiver
windows can support a qualified estimate. No feedback or insufficient sample
means HOLD/UNKNOWN. If no reliable basis exists, send flags without a loss
estimate, never zero-as-good. N1 sender/receiver integration must run only for
active lanes, at most about one report every 2s per direction, at most one
latest pending report, no passive timeout wake, no data/FEC batching, no
per-business wall-clock diagnostic, and no additional global locks. The
independent test vector in `internal/datapath/quality_health_v3_test.go`
pins these 104 bytes before any live wire enablement.

## 2026-10-11 N1 quality v2 live transport candidate (Actions pending)

The previously **reserved** 104-byte encrypted Health v2 format is now
provisionally wired **only after authenticated admission V3** in
`runtimeentry`. V2 and older admissions continue the legacy 9-byte health
v1 strict parser and never advertise or send quality reports. A V3 lane
continues to accept legacy health v1 from the peer and, once admitted, also
accepts 104-byte health v2. Both variants consume a fresh independent-record
PN (same protected KindHealth); neither enters LINK/FEC or allocates business
flow. Existing 15s liveness v1 is not removed. Extra v2 reports use the
existing 100ms runtime maintenance tick, at most one per **2s per ACTIVE
authoritative lane/direction**, no new per-lane goroutine, no DORMANT wake.
The first report may send on the first eligible tick after admission.

The nonce in each v2 report comes from the protected TLS/exporter lane
admission; the report's `Generation` is the **sender-local** lane generation
counter. These counters may differ between client and server; after
authenticating the record, the receiver checks its own current local lane
reference, the shared admission nonce, pins the first sender generation for
this incarnation and rejects any subsequent peer-generation change or
non-increasing report sequence. Rotation replaces the mailbox. No
unauthenticated report may establish an owner or lease.

**This first live increment intentionally reports INSUFFICIENT with an
invalid data window, unknown age (0xffff), optional transport SRTT
(also UNKNOWN if unavailable), and zeroed unqualified window counters.
It does not estimate physical loss**: sender-success DATA watermarks and
mature receiver DATA windows still need N1 instrumentation; a metric absent
from a report is UNKNOWN/INSUFFICIENT, not 0% loss. N2 auto FEC must HOLD.
Qualification requires new exact-SOURCE Actions (focused Go race, baseline
Linux/Windows and true nine-netns three-client real Linux paths); until
passing, this is a CANDIDATE only. The earlier paragraph describing the
format as reserved is historical for the prior codec-only source, not the
status of this subsequent live-transport candidate.

## 2026-10-11 N1 bounded DATA PN window candidate — not paired-loss estimation

The **existing 104-byte V3 KindHealth v2 wire format is unchanged** by this
source. The sender can now fill its outbound-window fields using actual
post-`Emit`/successful `EmitBatch` results from one 256-PN bounded KindLINK
history. `WindowFirstPN..WindowLastPN` is a *PN-coordinate* envelope, not a
number of transmitted records: there may be interleaved KindHealth, a failed
seal, and a locally failed send. `TxSuccess` counts distinct successful
fresh KindLINK record completions **within that envelope**; `LocalDrops`
counts locally rejected/failed fresh KindLINK completions inside the same
envelope. FakeTCP retransmission at its original Seq is not a fresh packet
and does not generate another KindLINK PN or fresh completed emission.
Both FEC systematic *and parity* records are KindLINK: count sealed
outer DATA records, not application datagrams or pre-FEC source packets.

If any fresh DATA success exists, the report has `WindowValid |
Insufficient`, real `TxSuccess` and recent `LocalDrops`. If no eligible
success has occurred it remains `Insufficient` without a valid window and
UNKNOWN data age. `AgeMillis` is the time since the most recent successful
local DATA emit in this PN window (clamped to 65534ms), **not** end-to-end
one-way latency. Last-seen SRTT remains transport RTT, not p99 delay.
Out-of-window delayed completion sets `CapacityLimited`; samples that
cannot fit must not silently be counted as if the entire PN span were known.

In parallel, `Lane.QualityReceiveWindow(now)` and runtime's active-only
`QualityReceiveWindow(ref, now)` expose a receiver-**local** 256-PN
diagnostic view: authenticated KindLINK first-unique, late first-unique and
duplicate replay counts under the existing rxMu. Authenticated duplicate
Health records do not increment duplicate DATA. Very late records beyond
this bounded PN span mark `CapacityLimited`. The AEAD decoder remains
the authority for cryptographic authenticity/duplicate suppression; this
small counter is not a second replay filter. Neither query is called per
business packet by the controller.

**The sender DATA PN space is independent from the receiver's own outbound
DATA PN space**. Accordingly the v2 outgoing report's `RxUnique`,
`RxLate`, `RxDuplicate`, `FECRecovered`, `EstimatedMissing` and
`PeakMissingBurst` remain zero in this increment: the report
`WindowFirstPN..LastPN` refers to its outbound direction, not the
opposite-direction local RX window. A future authenticated paired-watermark
exchange must select matching sender/receiver windows, allow sufficient
reordering maturity and tail visibility, then explicitly qualify a bound
estimate or leave it UNKNOWN/INSUFFICIENT. Never calculate `last-first+1 -
TxSuccess` as WAN loss, and never treat `RxUnique=0` in this report as a
0%-loss observation. N2 automatic FEC is still NOT_ENABLED and physical
loss measurements have NOT_RUN. SOURCE-specific Go race and privileged
Actions are required before this candidate may be qualified.

## 2026-10-11 N1 authenticated peer-TX watermark / receiver-local estimate candidate

**No wire-format change:** V3 authenticated `KindHealth` body remains 104
bytes, version 2, plus the legacy 9-byte health format. Its enclosing TLS
record PN is already authenticated by the same lane decoder. On receipt, the
runtime now retains this marker PN alongside the accepted sender-reported
`WindowFirstPN`, `WindowLastPN`, `TxSuccess`, `LocalDrops` and source
generation/nonce/monotonic report sequence. A received v2 record's seal PN
must be **strictly after** that sender window's last DATA PN. This marker is
NOT a receiver outbound PN and is NOT counted as KindLINK.

A new **receiver-local, read-only** `QualityInboundEstimate(ref,now)`
can pair the authenticated peer's latest outbound DATA PN envelope with the
receiver's own authenticated KindLINK window *in the matching C2S or S2C
direction*. It computes the number of first-unique received DATA PNs
inside that exact peer interval (including FEC parity DATA records). It
never subtracts raw PN gaps or counts KindHealth as DATA. A stable sender
window (same PN bounds, successful TX count, local drops and capacity flags)
must have been authenticated at least **3 seconds** ago; newer reports with
the identical window may refresh its last-seen timestamp without resetting
the initial observation time. This covers a limited reordering/RTO horizon
without creating a separate periodic goroutine.

The estimator refuses an ESTIMATED result if its marker is missing or
`markerPN <= WindowLastPN`, the interval spans over 256 PNs, receiver
retention is insufficient, either side reports capacity pressure, sender
DATA age is unknown or over 10s, the report is over 10s stale, at least
16 post-Emit successful sender DATA and eight authenticated receiver DATA
have not been observed, receive count exceeds sender successful sends or
the sender window is still moving. UNKNOWN/INSUFFICIENT/CAPACITY/STALE are
never treated as **zero loss**. Once qualified it returns
`EstimatedMissing=TxSuccess-RxUnique` and
`EstimatedFraction=EstimatedMissing/TxSuccess` with state ESTIMATED.
This is **estimated first-unique arrival deficit** after bounded maturity,
NOT physical WAN drops, an exact loss-burst measurement, a total network
capture or a promised end-to-end application delivery rate. Later repaired
bytes can change the estimate. Subsample confidence and maximum missing
burst are *not* inferred from sparse PN gaps.

**No closed-loop auto-control yet:** this is a local receiver API only.
`QualityFeedback` and transmitted v2 reports retain their explicit
INSUFFICIENT status and never set LossEstimated/EstimatedMissing from this
local paired calculation. The opposite direction has a distinct peer
generation/nonce, independent PN ledger and observation window. Publishing
bidirectional verified feedback into an extended authenticated exchange
is a *later* N1 task; N2 adaptive FEC must HOLD. New-source focused
`-race` tests and exact-source native network Actions are required before
the code can be called qualified.

## 2026-10-11 N1 128-byte correlated reciprocal quality echo (candidate)

V3 protected admission quality now proposes a **second, strict health
plaintext version** (`KindHealth` version 3, exactly **128 bytes**),
alongside the previously qualified original **104-byte version 2** and
legacy 9-byte health v1. The total body remains at or below the 128-byte
control requirement; no new LINK/UDP/app header or FEC data and no extra
per-lane goroutine. Each eligible 100ms maintenance Tick still emits
**at most one new quality control record per two seconds per ACTIVE lane**:
send 104-byte v2 if no authenticated peer report is available, otherwise
send one 128-byte v3 to carry this endpoint's normal outgoing TX summary
**and** separately correlated receiver-local observation of the peer's
opposite-direction TX. Existing v2 strict parser continues to reject v3;
v2/v1 negotiated connections never opt into either N1 quality format.

V3 echo byte layout (big-endian integers, AEAD encrypted/authenticated):

| Byte offsets | Meaning |
| --- | --- |
| 0 | Version 3; original v2 uses 2 |
| 1–67 | Original v2 header/flags, idle, reportSeq, shared 16B admission nonce, **this sender's** local generation, sender TX DATA PN first/last and post-Emit TxSuccess |
| 68–75 | **Echoed peer RX** first-unique KindLINK count in the *peer's own outbound TX* PN window referenced below (uint64) |
| 76–79 | Echoed first-unique late DATA count (uint32) |
| 80–83 | Echoed replayed DATA record count (uint32) |
| 84–95 | Original v2 FECRecovered (currently 0), LocalDrops, this sender's DATA age and RTT; semantics unchanged |
| 96–99 | Echoed *estimated first-arrival missing* (uint32, **not raw WAN loss**) |
| 100–101 | Original v2 peak-burst field (currently 0); never guessed from PN gaps |
| 102 | **Independent echo state**: 1 ESTIMATED, 2 INSUFFICIENT, 3 CAPACITY, 4 STALE |
| 103 | Reserved, must equal 0 |
| 104–111 | Original **peer sender** quality reportSeq being echoed (uint64, nonzero) |
| 112–119 | Original peer sender local generation being echoed (uint64, nonzero) |
| 120–127 | Original peer sender's DATA WindowLastPN being echoed (uint64) |

Fields in bytes 0–67 and 84–95 must describe the reporting endpoint's
**own** TX direction. Fields 68–83, 96–99, 102 and 104–127 describe
the other direction, and cannot be interpreted as counts in the same
PN interval as bytes 44–59. The shared admission nonce in bytes 20–35
binds both directions to the same authenticated lane incarnation, while
the two generation numbers may differ because endpoints own separate
local generation counters. The source reportSeq + source local generation
+ original source DATA lastPN exactly locate a recent source control
report independently of current TX counters.

Receiver of a v3 echo first performs the normal per-direction AEAD
KindHealth authentication, **strictly** decodes 128B and checks the peer's
own generation/nonce and strictly increasing reportSeq via the existing
one-latest mailbox. Then it looks up the referenced source report in a
**16-entry per-incarnation history of successfully emitted quality
reports**, matching its *own* local generation, shared nonce,
source reportSeq and DATA lastPN; history entries older than 10 seconds
are rejected. An ESTIMATED echo additionally requires the stored
source's actual post-Emit successful DATA count >=16, no capacity flag,
receiver unique >=8, unique <= sender success, and exactly
`EstimatedMissing = stored TxSuccess - echoed RxUnique`. Thus neither
unreferenced peer numbers nor any local control-PN gap can become a
loss estimate. Unknown / insufficient / capacity / stale echoes carry
**zero** quoted RX counts and are never treated as 0% loss.

A sender can now read `Runtime.QualityEchoFeedback` to see the
last *authenticated and source-correlated* returned state and fraction;
it expires to STALE after 10 seconds. The regular
`Runtime.QualityFeedback` still describes the **remote endpoint's
outgoing TX** and remains INSUFFICIENT. No N2 FEC auto logic consumes
the new read API in this candidate, even when it is ESTIMATED.
The existing 3s stability gate/256-PN bound and local first-arrival
estimation still apply at the receiver: it is not directly measured WAN
packet loss, a network pcap or an application-level completion metric.
Candidate requires exact-source Actions Go race, old admission/repair,
Linux+Windows and real nine-netns three-client functional checks.


## 2026-10-11 control-only encrypted zero padding / arbiter candidate

The 9B legacy health, 104B N1 V3 quality and 128B correlated echo
**plaintext bodies remain byte-identical**. The KindHealth record shares the
single TLS-like AEAD/PN sealer with the data path, but a new independent
SealHealthWithPadding is KindHealth-only: it encrypts zero padding **after
inner_type**. The existing record opener strips it before strict body-length
and reserved-field validation. The cached complete ciphertext/PN at the
original FakeTCP Seq is retransmitted unchanged. There is no LINK/FEC or new
business-fragment path.

The direction-specific safe single-record padding bound is
`pathmtu.Budget.RecordWireMTU - tlsrecord.FixedWireOverhead(31) -
KindHealthBodyLength`, additionally clipped by an optional internal control
padding max. `RecordWireMTU` already accounts for negotiated record cap,
actual configured IPv4/TCP options, peer MSS, effective outer PMTU. One
control-attempt random draw may choose length zero or a larger legal value.
No packet fragments, extra control records or random bytes in business
hot paths. Unsupported IPv6 outer combinations are NOT_RUN.

Runtime.Tick now uses one per-ACTIVE-lane control arbiter: quality/echo and
timed legacy health no longer each emit within the same maintenance Tick.
Successfully emitted quality carries the real IdleFor and satisfies the
ordinary health timer as the authenticated receive path already consumes
its protected idle hint. The initial bounded quality report may advertise
the first known sample; subsequent reports require genuine TX DATA
watermark/local-drop evidence or qualified peer-window/state change.
A stable source without new observations eventually stops repeating
quality; long-idle ordinary health remains necessary. Control emission
failures never advance successful health time and instead back off 1s.
One-time idle/close control can bypass ordinary jitter. Keepalive jitter
is at most ±10% and sampled only on successfully emitted control.

This is a control-characteristic reduction and byte/interval distribution
change, NOT a claim of indistinguishability, standards-conformant HTTPS
behavior, physical WAN loss measurement, N2 automatic FEC activation or
performance qualification. All new SOURCE Actions remain PENDING until
their actual results are recorded.


## 2026-10-11 N1 bounded control observation schema (pending Actions)

A successful KindHealth Emit adds one sample to the sender-local
8-bucket encrypted **record** length histogram with upper edges
64/128/256/512/768/1024/1280 bytes (last bucket over 1280), one
successful-to-successful interval sample with edges
500/1000/1500/2000/2500/3500/5000ms (last bucket over 5000)
except for the first successfully emitted control, and the sum of
encrypted control record bytes. No raw record bytes, IP/TCP payload,
destination fields, fixed nonce, credential or full pcap is persisted
by this instrumentation. Emit failure does not increment a successful
record or reset its time; no new timer, FEC, LINK or per-DATA operation
is added. The optional diagnostic `lanes[].control` exposes
`control_length_bins`, `control_gap_bins`, `control_wire_bytes`
and reason/merge/padding aggregate counters only after an explicitly
enabled existing diagnostic-jsonl path. These are *record* bytes
excluding outer IP/TCP headers. No per-run frequency or
indistinguishability claim without source-matched actual tests.
