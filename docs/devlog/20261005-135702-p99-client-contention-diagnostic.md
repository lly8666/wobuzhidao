# 客户端上行p99复现与显式阻塞诊断

## 本轮目标和阶段

用户再次明确p99与性能不能退化。开始HEAD2e54c4c，分支next/tlslike-dataplane；部署仍660b370。773四独立样本已全部验原产物，保留原660及新773配对FAIL，不降低500ms门。

## 修改与原因

CPU文件诊断入口增加双重显式启用的mutex/block抽样，仅cpu_profile=true的Action继承env开关。普通启动不加采样、listener、逐包计时；退出恢复mutex原policy、关闭block抽样，文件0600，stop幂等。诊断进程独占采样器，不宣称恢复无法查询的历史block rate。新增测试覆盖空路径不启用、profile文件及mutex恢复，仅在Actions执行。

单样本workflow保存binary profile与top文本；profile样本明确不能代替常规性能资格。原子操作/四元组/4096/FEC/queue/buffer均不变。修复native只读observer的IPv4-without-port误解析，严格验证IPv4四段、数值端口及头部协议，不从payload字符串猜协议；新fixture覆盖非法地址/端口/unknown。此前observer未用于实机。

## 复用来源

复用现有qualificationdiag.StartCPUProfile与单样本strict入口；Go runtime标准profile，无旧架构导入。

## Actions证据

773 foundation37267913480/targeted37267913600/analysis37267913440/predelivery37267913542/GUI37267913529/lifecycle37267913590/fullstack37267913569 PASS。
Game lossless37268226805、Game5305 37268229298同seed1382；Normal5205 37268232056、Game5205 37268234502独立单run单样本。单样本五分类均PASS，但Game5305配对p99 1121.840202-604.260511=517.579691ms超500ms，整体FAIL。Normal5205最差9.995925M/p99616.563ms；Game5205最差2.999889M/p99604.571ms；Game5305最差2.924166M、2探针未到target，不隐藏。
HEAD2e54 targeted37269309707/GUI37269309749 PASS；predelivery37269309705 parser FAIL已修正待验；foundation37269309626 Ubuntu automatic-lease测试Dormant Wake handshake失败保留，需定向复验，不能先判runner/flaky。本提交正确性/race与阻塞诊断尚未运行，本机未执行产品测试或编译。

## 问题、排查与风险

seq43异常RTT1871.914ms拆为上行1571.404、target echo0.030、下行300.480ms；seq44上行821.354ms。client四lane队列max2331.036ms。采集耗时memory1.286/product3.708ms，不能解释此秒级停顿。共享raw send mutex/阻塞syscall只是候选假设，必须用stack证据定位，不直接改锁或扩大缓存。采样会影响测量，只用于归因；诊断未部署。

## 下一项原子任务

先Actions核心/race及助手门、复验准确自动租约测试，后同seed Game5305单Action诊断。明确阻塞stack后小范围修复，再独立无profile lossless/5205/5305及p99配对；吞吐/丢失/CPU/短时空窗与尾延迟一起验。原生S20/S19 observer通过后继续，部署660及失败样本均保留。
