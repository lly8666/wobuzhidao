# E0 Normal jumbo无损FAIL的真实业务回执；修正stage收尾并派下一条8936/8937边界（2026-10-08）

## 恢复与精确身份
远端branch `next/performance-efficiency-20261008`、本轮父HEAD `03a64401907a847f6f0a107cacdf9330e7f7d9ad`，产品仍固定 `bf11fbfbe64d518e7ba189d51bfb4512df4df733`，含已合入的自动record预算（正式client/server `--record-limit 0`、outer1400、共享TUN实测MTU1273），没有在本轮改产品、物理机、主线、old。上轮Game4诊断 [run37768172504](https://github.com/lly8666/wobuzhidao/actions/runs/37768172504) 与 profile-off [run37766819445](https://github.com/lly8666/wobuzhidao/actions/runs/37766819445)均原始**FAIL**；E0 只取得Normal ordinary UDP及mixed的局部资格。

## 已结束的真实Normal大包结果，不得重标
全链正式client TPROXY -> encrypted raw -> 真实server共享TUN -> 目标socket，单独300s/seed1815、lossless300ms单向、双向每方向10Mbps、Normal1、FEC20:20、padding off、源helper精确 `03a64401907a847f6f0a107cacdf9330e7f7d9ad`。独立 [run 37769772363](https://github.com/lly8666/wobuzhidao/actions/runs/37769772363)，job113286187863、artifact11547309891、原样本/分析器/ledger步骤成功，但原分析器**FAIL**：`PAYLOAD_OR_SEND_ERROR_{c2s,s2c}`和`LOSSLESS_UDP_MISSING_{c2s,s2c}`，最终Action failure。始终保留，不因小包可达而取消FAIL。

C2S发送374968286 B（9.999154 Mbps）、交付84835880 B（2.262290 Mbps）；S2C发送374967806B（9.999141Mbps）、交付同84835880B。两向**96/256/512/1000/1372/4068B全部0缺失，8972B均缺6850、8973B均缺5480、65507B均缺2738，三档0收/0大包回执**；探针C2S1500/1500返回p99=600.938ms，S2C1495/1495 p99=600.978ms，missing0。两端generator send_errors=0；旧统一 `PAYLOAD_OR_SEND_ERROR`涵盖corrupt/malformed和send error，源JSON不上传，因此当前无法仅凭这枚issues认定密文或业务CRC已损坏，新helper会分开具体计数。大包未交付是**真FAIL**，CPU不能按被丢掉约77%业务字节推导降本。宿主Xeon Platinum8573C/4vCPU，client/server CPU33.92/35.14s，busy max12.72%、PSI some最大33.81，steal0、socket/interface drops0、cgroup quota未取到，无可靠容量受限证据。之前Game诊断CPU与此不是同输入/同mode/同CPU架构，也不得比较。

## 独立源码能力审计与新助手整改
`internal/platformflow/frame.go`定义`MaxPayload=logicaltunnel.MaxLeasedIPv4PacketLen-20-44`，`internal/openwrtclient/socket_linux.go:488-515`收到`n>platformflow.MaxPayload`时跳过原始UDP，而不是截短成已验收业务。此前旧large-mtu分支和新run均有大包FAIL，但原E0样本**没有8936/8937精确+1边界**，不能断言哪个字节是平台门限。此轮新增boundaries profile将单独覆盖8964/8965 IPv4 on 9000 inner fixture的UDP payload8936、8937B，同时带96..4068B独立业务和65507B，允许每档按sent/delivered/errors展示，不能把8936通过掩盖8937/65507失败。不能回退自动MTU到固定9000。

新发现另一个HELPER契约瑕疵：原`tools/large_mtu_mixed_business.py`drain_s=3，manifest也声称3，但`tools/large_mtu_loss_stage.py`实际一直保持netem到`start+315s`，真实阶段15s。旧run保持其原有PASS/FAIL但**全链真实stage 3s没有严格资格**。将stage改为`start+303s`，在分析器校验business_start/end约300s、end→drain_end约3s（上下500ms容差），fail closed，不能悄悄沿用旧资格。新助手还将原合并错误拆为`GENERATOR_SEND_ERROR`、`PAYLOAD_CORRUPT_OR_MALFORMED`、`PROBE_SEND_ERROR`及每向数值，不降低任何lossless/CRC/cap硬门。所有本轮helper/source需通过下一独立Actions自身精确SHA。

## 下一唯一正式样本与剩余问题
同一原子提交只将唯一 `.github/efficiency-e0-sample.json`换成 Normal1、UDP ordinary+boundary(8936/8937/65507)混合、每方向逻辑10Mbps、loss0、seed1816、300s和**真实3s stage drain**、profile off，workflow仍仅1 job/1性能sample，绝无matrix/同run A/B；GitHub Actions上先py_compile、source构建再真实netns client/server测量与校验。任何错误/容量不足均原始记录；不能为求绿减少用户支持范围。已有 Game4 4lane C2S用户态server ready队列溢出319041，见[时间轴](../evidence/performance-efficiency-e0-game4-ready-overflow-time-axis-20261008.json)，尚未决定改动不能简单扩队列、等待凑包或让fresh受4096限制。E1–E6及其4保护门、E6 P6同源包/长测、E7原80秒下行、物理全部未跑/未关闭；不写PHYSICAL_PASS。

详细数值及原始文件SHA在[本轮JSON](../evidence/performance-efficiency-e0-jumbo-fail-37769772363.json)。
