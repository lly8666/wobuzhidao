# 20261006-221000 客户端接收分段诊断

## 本轮目标和阶段

开始HEAD a8cef6d61ce8f0c15a326b0798728ae1e25604a6，next/tlslike-dataplane/P7诊断。只新增默认off、有界的接收分段观察，不抢先改变ACK算法。当前部署产品仍be456cee72a663a35d7e62fbedf17cc74bcbe410。

## 修改与原因

qualificationdiag验证私有进程环境变量WBD_QUALIFICATION_CLIENT_STAGE_TIMING；Windows入口在网络状态改变前检查，值1必须有diagnostic-jsonl。普通诊断不启用新增逐包计时。runtimeentry内部ObserveTiming只为已Attach/Promote成功的generation开启既有Owner/Deliver/record/FEC/LINK计时，替换保留旧retiring、新wake再启用。runtimeowner额外不可变ObserveFeedbackTiming，仅当现有timing.enabled与该标志同时true才读时钟/累计原子；服务端普通ObserveTiming不自动增加feedback开销。ACK反馈计时包含sendACK决策/锁/native同步emit，直接challenge ACK亦计；纯ACK触发的选中repair也覆盖，nil不计，取消选择算stage但不伪造成功。异步timer/tick排除。失败仍原样返回，同Seq密文/交付先于ACK/有限重传和窗口均未改。

新增Actions测试用实际runtime/datapath编码与受控Emit屏障验证blocked ACK之前业务已交付、阻塞期间可并发快照、off/base-only/flag-only不收集feedback、emit错误不吞、pure ACK选中repair原wire及失败、空repair排除和计数边界。既有真实TLS/Game替换-DORMANT-wake测试打开诊断，核验active/retiring/关闭后空快照及新代重新启用。修改的旧文件仅额外gofmt格式整理，无其他逻辑扩展。

## 复用来源

无old复用。使用本分支已有SetTimingDiagnostics、datapath aggregate计时及诊断JSONL；原14 native helpers不改。

## Actions证据

当前NOT_RUN；未在本地编译/单测/race。推送后按精确SOURCE读取foundation/steady/GUI/preflight/lifecycle，独立Normal/Game5205及同seed lossless、P6。每性能Action只一条，无A/B或matrix。未通过不能部署。

## 问题、排查与风险

旧be456实机p99=240.64ms/37up+1downmissing/driver+user+rawdrop0仍严格FAIL。三个一秒相关观察不能证明唯一因果；新增计时为了区分decode/Deliver/ACK反馈/repair。墙钟包含DLL/锁等待不是CPU；异步ACK timer不在新stage总量；sample不等于实际emit数量；字段原子而非事务快照，观测额外开销只允许明确诊断样本，不计普通资格。不开新队列/worker/timer，不扩大4096/FEC/接收缓存，保持无HOL。20项native配置缺口/M03与旧失败全部保留。

## 下一项原子任务

完成Actions门后才同源打包部署，保持14helpers字节b393一致，以外部SYSTEM任务进程局部环境启用一条诊断样本，不设置全局机器环境、不采正文/密钥；用新字段验实际启用，并按16MiB界限和owned cleanup收尾。观察后才决定是否解耦同步ACK反馈，不凭假设改变延迟/重传策略。
