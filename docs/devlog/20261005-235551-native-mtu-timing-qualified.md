# MTU时间助手Actions通过，同源四路复验

## 本轮目标和阶段

P7，产品仍配套7eeb。开始HEADf965，核验本轮测试工具Actions并开始独立M02 seed1413。

## 修改与原因

本提交只更新状态与日志。原生controller在上传前对实际助手文件与固定helper SHA逐文件核验（换行规范化），保留raw文件hash。助手SOURCE f965，二进制SOURCE7eeb不能混淆。产品未改动/重打包；旧1412迟到证据保留。

## 复用来源

既有MTU助手，无old提取。

## Actions证据

helper f965 next-predelivery-tools run37336237449四job全PASS：Windows CSharp编译/PowerShell解析、Python deterministic timing与发送错误/最大UDP真实socket检查、30重复race和Linux tc fixture。foundation37336237523、targeted37336237483、GUI37336237453均PASS。run链接均https://github.com/lly8666/wobuzhidao/actions/runs/<id>，精确SHA在STATUS.native_mtu_timing.actions。

原生1413已开始准备/测量，NOT_COMPLETED；独立300s Game4/FEC20/outer1400/inner9000。前一次没有逐包时间，不能从新健康样本宣称旧>1s长尾根因修好。

## 问题、排查与风险

两端UTC未验证offset，单机Stopwatch RTT独立有效。旧1412 Npcap write_call max<=18.2ms、sendlock<=7.1ms，没有可证明对应迟到的长调用；启动前quiet造成12.36s readgap，不能当业务处理停顿。五分钟功能探针低PPS，不能替代正式高负载p99/throughput门。

## 下一项原子任务

完成1413与逐序号独立target对账，检查RTT分布/timeout/late/发送调用与同机诊断，然后M03；遇到可定位迟到先报告最早异常边界，禁止盲改缓存。大pcap有界审计后删除。每性能Action一条。
