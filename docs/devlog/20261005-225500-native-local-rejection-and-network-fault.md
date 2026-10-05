# Windows失败Wake实机：本机拒绝与网络丢包分离

## 本轮目标和阶段

从9318224继续；产品和配套部署仍固定SOURCE2bf85a1。验收原生1410并修正故障注入语义，属于P7专项诊断，不关闭P5/P7。

## 修改与原因

新增physical_egress_blackhole.py：只在tc clsact egress中用精确TCP source443、peer /32、唯一pref/handle丢包，不修改root qdisc或OUTPUT防火墙。写前ownership，冲突拒绝；删除时核验selector/action并只删自己；外部已有或后来新增filter/clsact保留。predelivery新增独立隔离netns内核功能门，检查raw send成功而接收端确实收不到、UDP/其他端口/其他peer不受影响、清障可交付、foreign ownership不变。不是性能样本，不发业务负载。

未改产品Go/wire/缓存/重传/参数。源码确认SYNACK emit和association retry错误能返回到共享Run，但原裸error无法精确指认1410哪一个调用。不会把管理员阻止发送当成发送成功、吞所有EPERM或放大资源。

## 复用来源

复用正式native生命周期控制器和已验2bf配套包，无old提取。

## Actions证据

既有2bf五独立性能/18RTTpairs保持其原scope；新tc fixture门NOT_RUN，提交后执行Actions。所有性能仍每Action一条。

## 问题、排查与风险

1410客户端完整300s、所有状态观察同PID RUNNING，9次Wake失败、0恢复。fault实际25packet/1072B；服务端22:41:26 EPERM退出，22:41:49 systemd StartLimit，清障后没有运行中的server可回应。最后清理恢复config/service active，owned故障规则不存在。独立target/resource因原serverPID消失而缺失，load COMPLETE不等于验收PASS，p99NOT_EVALUATED。

Linux5.4 netfilter/core.c NF_DROP分支没有显式errno时返回-EPERM，可直接反馈给raw发送调用：https://github.com/torvalds/linux/blob/v5.4/net/netfilter/core.c 。因此OUTPUT DROP包含本机发送拒绝，并非纯WAN黑洞。已记录FAIL_LOCAL_OUTPUT_REJECTION_SERVER_EXIT，不抹掉失败，也不推断VM/FEC/CPU为根因。tc语义必须先由Actions真实内核门验证，ARM5.4仍待实机确认。

## 下一项原子任务

Actions确认tc门PASS后，在相同2bf配套包执行fresh seed1411：原业务时序、故障窗口与同PID/lease/有界重试/清障后新业务恢复门保持。记录两端PID、actual drop、socket/CPU/独立target和清理；无idleprobe则p99仍NOT_EVALUATED。若再出现server退出，以具体错误边界继续窄修。旧Normal醒后损失/rawp99/maxUDP和剩余32工况仍未关闭。
