# 可交物理机前的验收入口

唯一当前状态为 STATUS.json；本文定义门，不以框架存在或工作流全绿替代实际样本。P7 物理机由用户安排，永远单独记 NOT_RUN，hosted完成不等于 RELEASE_QUALIFIED。

1. 精确源码 foundation：Linux/Windows core/race、真实 kernel fallback、Linux共享TUN iptables/nft、OpenWrtTPROXY；生命周期36功能样本+aggregate，原失败永久保留。
2. 正式弱网18独立run：Normal1每方向10Mbps、Game4每方向3Mbps，FEC20:20，lossless/5205/5305各3seed；无损必须满速无损，既定有损按允许损失、时延与完整性门，不能恢复HOL或主动丢业务凑指标。
3. 实际配置70功能case：56所有FEC × 1..4lane × 填充开关并验证CLI覆盖冲突JSON；7JSON-only关键旋钮；1省略旋钮的默认；6MTU1280/1400/1500及双向512/768非对称recordlimit。真实正式程序、UDP/DNS/普通TCP/HTTPS、运行诊断和抓包共同证明生效。小record预算允许明确padding headroom skip，普通预算必须产生真实填充；UDP不得被启动填充影响。一个case一个Action，低负载功能测试不作为性能证据。
4. 180s Normal/Game仅框架诊断，不能替代1800s正式长测。正式持续目标速率、周期5/20%loss、600s轮换、60s停流，检查每阶段注入/业务/尾延迟、输入不足、socket/capture drops、内存平台期和FEC/LINK最终退役。无丢包长测之外必须保留压力切换。每性能run一条样本，不在同run A/B或matrix。
5. 同源码P6三目标包：Linuxamd64与Windowsamd64实际--version，Linuxarm64交叉构建明确限制；manifest逐文件SHA256、来源/版本和独立Actions回执。Windows包不包含server/驱动，物理Npcap/Wintun与ARM运行未验证。

## 参数覆盖的解释

PARAMETERS.json是全部CLI/JSON参数清单。基础core负责类型/边界/未知或不适用键拒绝/优先级；70livecase负责FEC、lane、padding、MTU、recordlimit和显式生命周期旋钮。生命周期fullstack负责idle/keepalive/dead-after/reconnect/rotation实际行为及黑洞恢复。foundation特权用例负责接口、TUN、lease、TPROXY、路由/NAT/firewall、账户隔离和退出清理；P6负责version/平台包。凭据字段用测试凭据，不能把真实密码写进日志。

数值配置空间无限，70case不是所有数值都测过。物理接口选择、Windows真实驱动/IPv6fail-closed和真实网站指纹留P7。OpenWrtIPv6未实现、Windowsserver不支持。内层TLS流量特征只有限缓解，不能声明不可识别。真实HTTPS的70case不替代受控网站多证书链/恢复握手流量分析专项；报告中分开已有核心证据和未跑的物理/指纹项目。

## 交付定位

全部hosted门通过后，STATUS标注hosted P5/P6完成、可交物理验收；证据保留真正SOURCE_SHA、文档HEAD、Action原始attempt、artifactdigest、缺失项。失败原样记录，不继承旧源码通过，不以扩大缓存、延长期限或降低速率过关。
