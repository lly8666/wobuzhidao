# 20261003-203800 保持Normal密文的本地generation归属

## 本轮目标和阶段

P5/P6交付前并发边界审计。起点70bd257ec249f9e2c7219ea2810ff9209ade6d6a，检查构造密文到外层发送之间的轮换。

## 修改与原因

代码审计发现NormalOutbound的owner fence后只返回无归属WireRecord，SendNormal再次选当前active transport。如果其间promote，旧密文会被放到新association，不符合generation隔离。给WireRecord增加不序列化的本地LaneRef，FenceOutbound在原有校验后盖归属；SendNormal在任何发送前拒绝非当前generation整批输出。不复制payload、不重新加密、不等待、不扩窗口、不重试；已过期的一包交内层恢复，后续新业务独立发出。Game已有显式LaneRef，保持原行为。零Ref仅供已有直接Lane调用API；正式owner业务必有Ref。

## 复用来源

当前datapath owner与runtimeowner generation机制，无old复用，不改wire协议。

## Actions证据

70bd基础37122380595、工具及30race37122380603、targeted37122380661 PASS；该源码全配置/正式长测/18弱网正在运行，仅作为原源码证据。新运行代码资格NOT_RUN。runtime既有端到端测试新增确定性边界：先封装旧输出，promote两端，再发送旧输出必须stale且无外层发包；后续新generation仍真实交付。新SHA需全部hosted门，不继承70bd资格。

## 问题、排查与风险

这是构造与发送间的本地所有权缺口；当前70bd动态样本尚未出现对应损坏，不能说抓包已确认此故障。Ref不进入wire，不影响MTU或带宽；增加小量定长本地metadata，性能仍按独立目标负载验收。发送选中旧transport后再promote可以只发到旧association，不允许把旧密文贴到新transport。

## 下一项原子任务

冻结新源码，基础及race通过后重新执行70配置、36生命周期、18独立弱网、Normal/Game1800s和P6包；原70bd样本保留但不替代新源码。
