# 20261007-195131 同源 idle / 最大UDP边界

固定产品SOURCE3a，无代码、qualification ref或sysctl变动，继续普通观测、串行独立300s，均正常退出。

S18/1535设idle30s/keepalive5s/dead45s，helper自身无probe/DNS。严格静默检查FAIL：客户端与服务端在计划第二静默区间重新active。独立metadata observer完整、drop0/unparsed0/cap未触发，记录约10.96s/46.48s/223.63s背景DNS及223.71s起背景HTTPS；约224s双方重建与真实非fixture业务吻合。双方约79s和181s确实physical/active均0，约120s受控业务重建且lease稳定；第二次240s受控冷启动因后台预唤醒未独立覆盖。不能说keepalive单独误唤醒，也不把被污染的静默门改成PASS。业务字节loss C2S0.09864%/S2C0.27432%保留；p99 NOT_EVALUATED，无idle probe，不以0掩盖。

M03/1536为稀疏功能边界，不是吞吐负载。实际Wintun9000/物理1500、outer connection1400。完整300.045s：允许IP分片时UDP8972/8973/65507各171次完整且一秒内回包，穿插96B1362次；DF9000总长170次全回。UDP65508与DF9001/65535等预期MessageSize10040均正确拒绝。坏payload/重复/接收socket错误/invalidresponse0、client始终alive、server raw drop0；outer预算/checksum/固定seq字节/有限capture删除和退出journal/NRPT/firewall清理通过。标PASS_SCOPED_NATIVE_MTU，仅一条不关闭旧M03 missing/1.308s late。

大包尾延迟不能忽略：96B p99约107.956/max117.896ms；UDP65507 p99约319.307/max723.193ms，9001总长允许分片的最大约740.770ms。全部在一秒及时门内不意味着大包延迟与小包一样，不宣称无延迟成本或关闭历史尾延迟。源端计时完整保留，稀疏样本不能替代Normal10/Game3。

新独立DNS互备D02/D03已经按lease-only FORWARD损伤启动，末尾M03再重复一条。原来两条Normal残余loss、全部普通server接收pressure、native匹配低载RTT基线和其他NOT_RUN都保留。现有6条完整样本不等于6条全PASS，更不是全P7完成。证据 `docs/evidence/lane-duplex-3a594a3-physical-20261007.json`。
