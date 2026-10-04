# 20261005-072300 原生helper来源与分片测试边界

## 目标与来源

父SOURCE c5eff80，用户要求连续处理。产品逻辑不变，仅修当前资格测试和原生helper。c5 Normal5205 run37243131174已SUCCESS；其余资格仍在运行，不部署。

## 问题与修改

新增换代测试的runtimeIPv4添加20B IPv4头，但传入9000B payload，使完整包9020B超过现有MaxLeasedIPv4PacketLen=9000。源端正确拒绝，测试等待Emit再超时，而defer Close首次发FIN又撞测试阻塞器导致job长挂。改为8980B payload，即合法完整9000B；增加幂等unblock退出保护和编码提前错误分支。第二fragment失败测试同样按合法完整9000B输入。未扩大产品IP长度或更改拒绝语义；M03依然应拒绝大于产品9000B完整IPv4边界，不能因UDP API允许65507而写隧道也支持。

DNS/MTU helper的SourceSHA由部署binary --version读取，避免升级后回执仍写6181；DNS另外记录实际时长。Actions helper语法检查覆盖这两脚本。产品不重编译于开发机。

## 证据、状态与下一项

c5 predelivery37243114897的助手编译/模板/race三job均PASS；default network37243114921、splitroute37243114923、GUI37243114956、padding37243114959、linux-server37243114893 SUCCESS。上述不能替代blocked core或未结束stateful/Game。c5新增测试卡住属夹具缺陷，保留历史状态，不冒充通过。

提交修正后重验core/race并独立Normal/Game5205和stateful；完整SOURCE统一P6包后才实机D01。当前P7仍PARTIAL，五分钟4/43，旧D01 FAIL保留。
