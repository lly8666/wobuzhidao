# Linux内层MTU分离与M03真实长尾边界

## 本轮目标和阶段

开始HEAD5f105f0，分支next/tlslike-dataplane。继续P7前原生修复；既有SOURCE852与b393助手、完整300s seed1434真实结果先记录，再修一个已有MTU语义错误。

## 修改与原因

cmd/wbd-server入口显式校验外层MTU，shared TUN改用已有lease合法包上限9000。ConnectionMTU仍传原配置，LINK/TLS/FEC/重传无变化。默认正常热路径零新增分支、时钟或锁。旧1400 journal canonical/recovery保持generic builder行为。同步唯一参数清单帮助、正式方案和Linux说明。

tools/test_linux_server_delivery Actions实际检查outer1400时kernel TUN9000、journal9000，outer575/9001 check-config拒绝且无network journal；原SIGKILL/清理/升级/回滚与generic1400 managed fixture均保留。未以新协议、强制可靠、buffer或shadow扩大换通过。

## 复用来源

无old提取。直接用现有logicaltunnel.MaxLeasedIPv4PacketLen，与已验Windows内层限制相同。

## Actions证据

本候选NOT_RUN，提交后冻结精确SOURCE跑core/race/Linux服务实际内核/生命周期/网络，独立Normal+Game5205和相应无损/5305每Action一条；通过后P6同源配套包才部署，不继承852资格。父852的17独立run定向门/18RTT及P6均PASS，详见已有证据。full70/18/1800s未跑。

## 问题、排查与风险

852完整M03所有2014包最终返回、小包1342全及时，最大UDP168全回但3次迟到1.270/1.404/1.905s，最大UDP自身p991404ms（混合p99175ms不能掩盖）。Linux12934元数据行与Windows12934真实写入行逐IPID/offset对应，metadata drop0/无错误写入/内容破坏；48片最大回包全部最终提交。driver call最大约0.552ms，最后片在同Windows时钟1.27..1.91s才提交，delay在最终提交前，非应用收到完整包后拖1秒。跨机UTC未做单程因果推断。

client Rx FECpressure0，无跨数据报HOL证据；repair/RTO变化与迟到共现但未证明唯一根因。shadow峰值远小4096、fresh不blocked，禁止据此改4096。当前仅消除已证实的inner1400导致48片+LINK重复分片，是否救p99要实测。最大UDP仍须自身IP分片，单包自己的重组等待是允许语义。

清理正常STOPPED Exit0、owned NRPT/firewall0、network remainingfalse、所有bounded原始outer capture已分析删除；无原始inner payload capture。稀疏观察onCPU不是吞吐资格。总历史38完整300s/16ID/27NOT_RUN含失败和跨SOURCE，整体P7仍PARTIAL。

## 下一项原子任务

候选定向Actions/独立性能/P6通过后paired deploy，先有效非空观察预检，再两独立300s M03。验实际Linux/Windows inner9000、outer仍1400、最大UDP48→8和record/PPS账本、内容与DF/超UDP拒绝、maxUDP自身p99及小包无HOL、正常清理。失败保留，窄诊断，不盲扩缓存或改恢复参数。
