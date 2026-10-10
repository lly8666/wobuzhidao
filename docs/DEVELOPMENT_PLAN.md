# 当前开发计划导航

唯一详细方案：[ADAPTIVE_NETWORK_PLAN](ADAPTIVE_NETWORK_PLAN.md)。唯一实时进度：[STATUS](STATUS.json)。工作分支next/adaptive-fec-aes-tun-20261010，继承FEC优化产品7fb98fab；当前仅PLAN_READY。

|顺序|内容|复用/新增|
|---|---|---|
|N0|受保护admission V3协商每客户端FEC/密码|复用真实TLS/身份/lease，新增短能力块与KDF上下文|
|N1|双向近似质量/低频摘要|复用PN/ACK/SRTT/health，新有界统计与V3控制payload|
|N2|Normal auto/auto-aggressive|复用SIMD FEC/已有wire，新增控制器与跨档有界RX|
|N3|AES128/256独立记录|标准库AEAD/AES HP，复用record格式/不可变重传，扩展uTLS实际协商|
|N4|Windows TUN分流/网络取消|复用Wintun/Npcap/IP数据/journal，新direct转发/少量routes/幂等Stop|
|N5|GUI/参数/质量显示|复用portable/catalog，新增中文模式和两秒status snapshot|
|N6|真实工况/配置/打包|复用五netns夹具，独立Windowsnative功能与三目标P6|

永久语义看章程，模块按MODULE_MAP定向读取，V2实现与V3提案边界见WIRE_SPEC。所有开发测试在Actions，每性能run一条；本分支不继承历史串行多样本例外。合理新门见本轮方案，不篡改历史FAIL。

原E0..E7/FEC SIMD/FEC开关实验计划已归档，历史scope/结果看父STATUS与evidence；旧任务不自动继续。约80秒下行延期到本轮后诊断，MTU继承改动一并压力测，原OPEN不得删。
