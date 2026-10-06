# 20261006-224324 Windows最新ACK资格与配套部署

## 本轮目标和阶段

精确产品SOURCE d6cb6cee4c241aac8dd2f542a876edc57bf3d7db。ACK worker只在Windows正式入口内部启用；继续P5/P7局部性能验收，不宣称交付完成。

## 修改与原因

本轮归档10个定向Actions（worker race10、foundation Windows/Linux、steady、GUI、preflight、lifecycle、独立Normal/Game5205及同seed lossless），全部PASS。4条性能run各一条120s样本，五分类全PASS；12阶段p95/p99对全PASS，最大增量12.568845/13.958733ms，原200/500ms门不变。注意严格hosted程序走Linux，不启用Windows正式worker；Windows核心/阻塞native Emit模拟/race证明功能，实机性能仍待验。

## 复用来源

沿用本分支ACK决策、native IO gate、generation、异步错误tick出口。无old新复用。原14helpers字节b393、DNSguardprobea280不变，不改变负载模型。

## Actions证据

证据docs/evidence/native-ack-d6cb6ce-actions-package-deployed-20261006.json及receipts压缩保存精确SHA/attempt1、全部run、12RTT、三平台P6文件哈希/manifest/actionsreceipt。P6 run 37480766936四jobs PASS。完整70配置/18弱网/1800s当前源码NOT_RUN，不继承旧024。

## 问题、排查与风险

配套部署Windows WBD-P7-d6cb6ce，ARM /opt/wbd/p7-candidates/d6cb6ce；active服务已同源，9000内层/1400外层及身份配置保留。Npcap不重装、凭据不进入日志/仓库；保留rollback-78b8faf与更早备份。每generation单worker/单pending，无ACK历史/业务缓存，FIN/challenge/repair继续同步；失败锁存并原tick报告，关闭不持锁等待native写。不能将旧78 ACK墙钟209秒当CPU节省。旧instrumented诊断queueoverflow51534/4343downmissing/19probe未回/p99712.93ms FAIL保留；普通be456近10M但37up/1downmissing严格FAIL保留。49普通300s不是49PASS，另2诊断不计数。

## 下一项原子任务

依次跑stageoff/profileoff Normal1双向10M/FEC20/300s seed1480和Game4双向3M/300s seed1481；不得同机并行。实际worker=true/timing=false硬校验，原输入、probe、business、DNS/owned清理门保持，抓包有界分析即删。看worker queued/coalesced/piggybacked/attempts/sent/failures及接收overflow、CPU和p99，保持吞吐不能掩盖少量loss。M03及20配置缺口继续待验。
