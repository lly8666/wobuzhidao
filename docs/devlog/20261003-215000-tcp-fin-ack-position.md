# 20261003-215000 独立FIN不能被旧数据ACK释放

## 本轮目标和阶段

P5 TCP半关闭边界，起点3d9ce2cd840b00f459e470b4cdd94ef81ff4e386。

## 修改与原因

继续审计发现TCPTransmit为空FIN没有独立ACK位置，FIN排入后，之前数据的延迟ACK（同一个字节偏移）就会把尚未到达的FIN从pending删除并置finAcked。业务半关闭可能永不交付。按TCP常规做法给内层proxy FIN一个虚拟序列位置；ACK只有在FIN和此前数据实际收齐后+1，旧数据ACK仍保留FIN可重传。接收已交付重复字节上的首次FIN也必须触发一次半关闭，不能把新FIN一并作为duplicate跳过。偏移溢出在Queue修改状态前拒绝。

不增加报文字段/字节、不扩大窗口/超时，不新增outer可靠性；只内层TCP代理需要完整字节流。WIRE_SPEC记录此语义需双端同源码升级，未发布旧候选不可混用。

## 复用来源

当前platformflow TCPTransmit/Receive，无old复用。

## Actions证据

3d foundation37126861628、工具/30race37126861629及定向/lifecycle/padding PASS。真实f10-l3-p0 37127014448 PASS，先前23 HTTPS FAIL保留。3d Normal180仍在原始运行。新FIN源码NOT_RUN。新增测试丢掉独立FIN、注入旧payload ACK、验证仍pending并在既有RTO重传、实际FIN ACK才退休、重复FIN抑制；已交付字节上首次FIN仍半关闭。

## 问题、排查与风险

这是静态确定的零长度FIN ACK歧义，不把它宣称23失败唯一根因。半关闭语义改变，双方必须同时升级。不能只unit过就交付，全部配置真实HTTPS和生命周期需要同源码重新执行。outer无HOL与有损UDP门均不变。

## 下一项原子任务

确认3d诊断PathErrors原因后冻结修复源码，foundation/race后所有hosted门同源码收口；P7物理NOT_RUN。
