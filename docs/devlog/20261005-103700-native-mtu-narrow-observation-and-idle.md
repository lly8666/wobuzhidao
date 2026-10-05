# 两份最大UDP回程缺失：补观测，继续idle

## 本轮目标和阶段

SOURCE660b370始终不变，开始HEAD15b92cd。M03 seed1366第二份完整300s合法65507/DFfalse169发送、target169接收/echo无错、client168最终精确，其中另1次迟到已重新校验；未全收齐记FAIL，第一份171/170的FAIL保留。小包1352/1352、8972/8973允许分片全回、DF超9000和65508 API明确拒绝，坏数据/其它发送错误/客户端退出0，退出owned清理通过。

## 修改与原因

只补测试助手诊断，不改产品：MTU客户端由已存在bounded8192 pending inventory导出最终MissingSequences；target在同一8192上限内记录收到的sequence，只保存数字不存payload。Windowswrapper保存netstat IPv4前/后计数，须标注全系统、不能直接归属本socket。用于缩小缺失序号是否被target收到、是否有IP重组错误，而不是第三次无新证据盲重跑或改FEC/4096。

## Actions与复用

新helpers提交后先next-predelivery-tools AddType/PSparse/Python/unit门，再下一原生诊断。产品source/正式Actions资格没有变化；运行中的S18使用此前已通过HEAD0af35ea lifecycle helpers，其源hash在开始时固定，不受MTU helper编辑污染。

独立18弱网短样本已收齐，全部CORRECTNESS/INPUT_VALIDITY/CAPTURE/ENVIRONMENT/PERFORMANCE PASS，socket drops0；相同seed独立lossless RTT配对按现成revision2分析器只读原artifact复核，不在开发机运行产品。70配置截至本次刷新51/88整体runs完成、无run FAIL，未完成或未收齐不记PASS。两独立1800s仍IN_PROGRESS。

## 问题、排查与风险

最大UDP在回程经过Linux TUN1400 IP分片，单个完整UDP需许多IP fragments；某片丢失可使整个UDP缺失。但当前证据只限定回程，不足以归因Internet或产品。两次一致质量缺口应重点报告，避免因96B或9000B正常就忽略65507B。mid-window WindowsIP全局reassembly_failures0、Linux累积历史计数不能当本case因果。

## 下一项原子任务

S18 seed1391原生setup/300s业务0–30、120–150、240–300，keepalive5/dead45/idle30实际生效须读product.config，server无dead-after参数不得误写配置；静默阶段夹具不发probe/DNS。验证双方DORMANT physical0、lease稳定、新业务唤醒，再S19/Game和S20纯下行。最大UDP新helper门通过后只跑带序号的定向诊断。每性能Action单样本、raw抓包分析后删除，剩余矩阵持续推进。
