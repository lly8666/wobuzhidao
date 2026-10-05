# 内核发包压力边界与四lane休眠复测

## 本轮目标和阶段

产品部署SOURCE660b370不变。开始HEAD4d673814。用户要求优雅、低开销修复且守住无HOL。每性能Action严格一条，默认不打开profile。

## 修改与原因

增加显式WAN邻居dynamic/permanent诊断，限定现有owned三namespace与四个WAN邻居；真实读取MAC，PERMANENT写后必须内核读回；默认dynamic只读。保持现有literal $ namespace后缀，拒绝跨sample/角色错位/任意shell文本。业务/TUN/host邻居不改，退出由既有namespace删除。资源采样每秒记录邻居状态；无产品逐包开销、无新增产品CLI。性能Action记录模式、精确product/harness SHA与独立claim，静态邻居结果不能冒充真实动态邻居性能资格。

## 复用来源

无历史代码复用；沿用当前strict netns与资源采样边界。

## Actions证据

HEAD4d67381 foundation37271184684、targeted/race37271184710、GUI37271184688、predelivery37271184697 PASS。两条profile诊断37270295492/37270895113健康p99约632.55/602.25ms、queue max12.88/17.80ms，不替代普通性能资格。新helper正确性Actions NOT_RUN，待本次push后验。

## 问题、排查与风险

两份原失败窗口client raw socket t=tb=212992，两个健康profile t最多960；均非receive r/d增长。共享raw发送锁跨阻塞syscall持有可能扩大卡顿，根因ARP/next-hop仍未经观察。不盲改DONTWAIT造成fresh drop/连接失败，不扩大socket/FEC/4096，不降低p99门。

S19 seed1396完整300s：上行2.999994M/0损失，下行2.950683M/1.643719%损失；两次计划wake成功，quiet释放四lane且无前轮69s多余wake。numeric observer335s captured0/drop0/unparsed0/counter匹配，独立确认无fixture外上行。owner保留lease，退出0与owned清理通过。仍需查重建交付质量，不能写整体PASS；idle探针禁用所以p99未测。22完整样本/11工况/43总计，32 NOT_RUN，混合SOURCE不继承。

## 下一项原子任务

新helper门通过后相同产品/seed dynamic与permanent各独立Action诊断，再按结果决定最小产品修复；同时定位wake缺失与最大UDP序号。两原p99 FAIL和旧S19背景不明样本继续保留。
