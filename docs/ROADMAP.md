# 唯一路线图

本轮N0→N1→N2→N3→N4→N5→N6，详细方案ADAPTIVE_NETWORK_PLAN，实时步骤只看STATUS。P0..P7保留产品资格含义：本轮处于P5开发验收，P6打包后交原聊天P7物理，不继承旧通过。

|阶段|退出条件|
|---|---|
|N0|每客户端明确协商，双向首包正确，错误/旧版本/身份冲突拒绝|
|N1|方向/样本/水位正确，有界非阻塞，UNKNOWN/HOLD/休眠边界|
|N2|Normal自动/激进、所有切档迟到语义、Game隔离，动态弱网真实路径|
|N3|三AEAD/HP、硬件与generic、TLS actual值、同wire、混合客户|
|N4|Windows少量routes、TUN direct实际回程、防回环、owned幂等取消清理|
|N5|CLI/JSON/GUI/catalog生效、低开销两秒状态|
|N6|固定Normal/Game保护、组合配置/MTU/弱网/生命周期/三目标包|

之后：单独定位80秒S2C、补Windows驱动/ARMnative物理门。出现严重中断如实留FAIL；任何阶段未完成不标完整交付。历史路线归档，不是当前指令。
