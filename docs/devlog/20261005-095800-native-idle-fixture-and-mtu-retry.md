# 20261005-095800 原生idle/纯下行夹具与MTU重跑

## 本轮目标和阶段

产品SOURCE660b370已同源部署，不改产品；当前M01 seed1362在原生300s重跑。文档起点0318685，补S18/S19/S20夹具，为下一步idle30s、keepalive5s、dead45s实效准备。

## 修改与原因

新增独立native_lifecycle_udp_client.cs/target.py/windows_lifecycle_udp.ps1，不覆盖运行中M01已上传的physical或MTU助手。只支持300s显式idle30或downlink profile：业务0–30、120–150、240–300s，其余静默，发送预算按累计活跃120s计算，静默不累计待发字节，恢复时不补发静默期间假业务。客户端不发UDP RTT探针，夹具不启动DNS；实际keepalive只能由产品自身发。纯下行在协商后客户端不发业务，服务端连续300s，用来验证下行不会误休眠。

静默保持socket但不产生REGISTER/PING假业务，去重/正文校验/bitmap预算沿用现有稳定助手，server另逐秒小计。表中吞吐按实际offered120s或纯下行300s归一，不用300s分母把正常计划静默误判成吞吐退化。新增Actions中双方同一组phase边界fixtures，防止静默累计预算导致唤醒突发；C#/PS编译解析先过再原生。

## 复用来源

当前physical_udp_client.cs/physical_udp_server.py/physical_windows_udp.ps1，无old或产品架构重做。新助手名字与当前M01的精确上传hash分别记录。

## Actions证据

产品660b资格仍windows-tun-mtu-actions-660b370-20261005.json；新生命周期helper当前IMPLEMENTED_NOT_TESTED，提交后查next-predelivery-tools精确HEAD。禁止拿helpers CI继承成原生idle PASS或最新产品全70/18/1800s。

## M01不完整试验与修复

seed1361约91s后外部MTU executor读取controller正在覆写的p7-status.json，JSONDecodeError Extra data导致夹具finally主动停止产品。实得controller reason=requested_stop、exit0、network/NRPT/firewall remaining0，不是产品自动断线；没有完整300s/最终业务回执，不计入full300s数量。原生运行时已实读9000，不能以此补写完整MTU PASS。

修外部executor短暂坏JSON重试，且finally按本case/root/脚本名精确停止远端helper子进程（杀本机SSH父进程并不保证远端助手退出）。不完整case残余两助手已清理、product0/task0，证据保留在受控native-current/660b370/m01-current-300s-seed1361。新seed1362全量重跑，原MTU journal实见Previous65535/Applied9000；接口原本不存在，关闭后可能随产品owned adapter消失，不能将不存在的接口强写成实读还原65535。

## 问题、风险与下一项

静默期间若Windows其它应用经隧道产生活动，则本case应INVALID_INPUT/无法证明闲置，而不是强杀用户程序或把计时误判为产品bug。以双方owner/DORMANT/physical=0、lease稳定和实际活跃段交付为证据。当前MTU先收齐，再M03最大UDP；新helpers Actions后S18/S19/S20分别300s，必要repeat。每性能Action一条，raw bounded capture分析后删除。
