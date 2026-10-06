# 20261006-174943 DNS guard ownership回执同步

## 本轮目标和阶段

开始HEAD b675fd9d8fc6b6a617f12f5d0af151b7596835f1；产品DNSguard未部署。foundation 37445044674在Windows active-go-tests失败，严格保留。

## 修改与原因

internal/windowsclient/splitroute_windows_test.go原来以两条PASS marker断言Apply/rollback，新增五场景后仍要求恰好两条。失败原始输出明确success、route-failure、dns-failure、dns-off、alias-failure五条全部PASS，Go包装却判missing receipts。现改逐个要求五个不同场景唯一回执，再要求总数5；不删除测试、不减门、不改产品网络脚本或任何数据路径。仅gofmt编辑，开发机没有跑编译/unit。

## 复用来源

无。当前PowerShell实际Apply/Cleanup/rollback/foreign ownership检查保留。

## Actions证据

b675 foundation FAIL https://github.com/lly8666/wobuzhidao/actions/runs/37445044674 ，Windows job112207785042；GUI37445044649、predelivery37445044670(新probe AST/vectors)、targeted37445044781、preflight37445044698、default-network37445044675、lifecycle37445044773、splitroute37445044712 PASS。两独立strict5205及P6仍运行/待收。重复手动functional37445152391/37445156663已请求cancel，原因同源push已有primary，保留dispatch/cancelreceipt，不是隐藏失败性能样本。

024 Normal/Game两独立1800s37440874448/37440878333均PASS，最低phase9.9991637/2.999872M，raw/socket/internalqueue0。Normal最大phasep99=619.918224ms，Game=601.968186ms，含600ms基础RTT；真实observed generations记录，不能以配置假定每lane都轮换。新候选不能继承024全量资格。

## 问题、排查与风险

旧回执数断言属于新增场景漏同步，未发现产品DNSguard失败。但不能仅凭该解释部署b675；修正HEAD重跑相应精确源码门/P6。Windows真实provider alias绑定与出口阻断还未验证，p99/CPU真实机器待测。M03 missing/late仍PARTIAL，S07/S08失败保留。

## 下一项原子任务

新HEAD Actions core/GUI/predelivery/network与两独立5205/P6，检查自动触发再补缺项，避免重复functionaldispatch；之后同源新包部署，正常DNS60/60、physical53=0、强制物理UDP/TCP off-control/on-guard、实际filters与退出ownedDNSgroup=0。

## S10 seed1447 结果

实际配置Normal1/FEC20:16，300.001317s。状态FAIL，errors=['business_loss']；吞吐{"c2s": 9.999731093333335, "s2c": 9.999980053333333}，业务损失{"c2s_packets": 14, "s2c_packets": 0, "c2s_percent": 0.002222943695218138, "s2c_percent": 0}，探针{"sent": 2979, "received": 2979, "p95_ms": 107.4698, "p99_ms": 172.2905}。产品/助手CPU秒{"client": 294.828125, "server": 155.63, "client_helper": 21.640625, "server_helper": 35.85518056}，server rawdrop最大96，WindowsNpcap{"interface_dropped": 0, "driver_dropped": 0}、user overflow0。输入{"client": {"TxPackets": 579418, "SendLagSamples": 579418, "SendLagOverflowSamples": 238, "SendLagP99UpperMs": 2.1, "MaxSendLagMs": 111.53029999996988, "PacingMode": "byte-budget-1ms-batch32"}, "server": {"TxPackets": 579419, "SendLagSamples": 579419, "SendLagOverflowSamples": 0, "SendLagP99UpperMs": 1.2, "MaxSendLagMs": 7.719615963692306, "PacingMode": "byte-budget-1ms-batch32"}}，DNS{"total": 60, "success": 60}，selectedNIC plaintext53观察{"state": "PASS_NO_PLAINTEXT_DNS_OBSERVED", "scope": "selected physical NIC only, plaintext IPv4/IPv6 TCP/UDP53; not DoH/DoT/otherNIC", "observations": 302, "dns_frames": 0, "observer_drops": 0}。owned清理/限量raw审计删除门检查保留。

实际encoder parity/source比例client 0.8000704380617507、server 0.800061851596258，accountingPASS_ACCOUNTING；含DNS/探针/setup/drain的全diagnostic窗口，字节比例不是精确业务窗口放大。低档partial仍min(N,R)，不改分组/期限。真实WAN未人工丢包不等于可证明底层0loss；matched native RTT baseline NOT_RUN，不把本条p99伪装成严格增量门PASS。证据docs/evidence/native-s10-0246526-seed1447-20261006.json，完整样本45、unique21/43、其余NOT_RUN22，包括失败和跨源码。下一S11一条独立样本；M03不关闭。

S10 FEC20:16 seed1447完整300s结果FAIL business_loss，上行14包/.00222%字节损失，双向近10M、2979探针全回、p99=172.2905ms，中段p99=203.5333ms；DNS60/60/physicalDNS0、server rawdrop96，客户端CPU294.828125秒。实际parity/source≈.80007/.80006，FECpressure0、freshblocked/abandoned0、record/patherrors0，sourceexpiry1。档位不同且WAN未知，不能唯一归因FEC/CPU，吞吐满速不隐藏尾延迟/接收压力。记录docs/evidence/native-s10-0246526-seed1447-20261006.json；S11 samehosts/defaultFEC20作为下一单样本基线。

024两1800s证据docs/evidence/soak-0246526-1800s-20261006.json与docs/evidence/soak-0246526-1800s-20261006-receipts.json.gz入库；完整70/18/长测PASS仅归024，不归新DNS候选。
