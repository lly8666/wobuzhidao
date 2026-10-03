# 20261003-214600 业务TCP本地写出与FIN提交边界

## 本轮目标和阶段

P5交付前，起点5d8777bb87100a9244daeb6cd03d8a766754b4f8。缩小23源码f10-l3-p0 HTTPS内容不完整失败。

## 修改与原因

源码审计发现同一TCP代理flow的handleData在flow.mu下推进rx字节偏移后，释放锁再writeFull/CloseWrite，多lane回调可并发。后一批或FIN可能越过前一批本地写出；同时handleAck可看见rx.FINDelivered并提前close本地socket。两端增加仅该TCP flow的rxMu，保护rx.Push→writeFull→CloseWrite→ACK。单独rxFINCommitted只在本地写完及CloseWrite成功后发布，finished只检查此状态，不以尚未写出的rx偏移判断完成。

不改变outer独立记录/乱序交付，不锁其他业务或lane，不加队列、复制、goroutine、重传预算或窗口；TCP业务本身要求字节顺序，UDP路径不新增锁。ACK不获取rxMu，避免双向同步sink死锁；abort/Close仍可中止阻塞写出。

## 复用来源

当前platformflow TCPReceive、writeFull、FIN机制，无old复用，无wire变动。

## Actions证据

23原f10-l3-p0 run37125010683 FAIL：业务HTTPS客户端收到不等于102400字节的body（旧诊断未打印长度），UDP/record/path完整性0，不允许以TLS建连完成替代body。原失败保留。新测试用有控制的阻塞本地Write，覆盖client/server两端：后到FIN不能先CloseWrite；双边完成ACK不能提前Close；释放后完整顺序write→fin。新源码NOT_RUN，基础/race及真实多laneHTTPS必须验证。

## 问题、排查与风险

并发边界由代码确认，旧动态失败尚无精确body长度或goroutine栈，不能宣称唯一起因。上一提交新增错误长度/hash用于复跑归因。TCP每流交付需要短互斥，阻塞的本地业务只在该flow已有write期间等待，不能拿这个锁来等待FEC或外层缺洞。尚未扩展TCP性能资格，UDP目标性能仍需完整同源码验收。

## 下一项原子任务

基础/race后Normal180和f10-l3-p0单Action各一条，读取具体错误，再全70配置/36生命周期/18弱网/1800sNormalGame/P6。若出现其他错误继续精确修复，不重复盲跑。
