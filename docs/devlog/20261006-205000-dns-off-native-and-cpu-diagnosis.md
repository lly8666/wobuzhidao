# 20261006-205000 DNS关闭实效与性能诊断

## 本轮目标和阶段

开始HEAD54fab873，分支next/tlslike-dataplane；冻结产品a280566b3a1642d43afe95f29a70ab4daf4e8759，P7实机用户授权。收口D06，不改产品运行路径，随后只进行有界CPU诊断。

## 修改与原因

仅证据/状态/入口。D06 seed1468发送300.0011006s，Normal1/FEC20:20/每向10M/混合96..1372B，实际配置dns-hijack=false、NRPT0、ownedDNSguard0；DNS60/60，强制physicalif6 UDP/TCP1.1.1.1:53都返回有效回复，physicalDNS332帧属于关闭劫持的预期。退出normal0、journal/NRPT/IPv6/新DNS组0，恢复原配置。开关功能限定PASS，关闭开关不应要求物理DNS为0。保留普通DNSon零泄漏分析器不变，新off分析器仅接受D06和实际flagfalse。

## 复用来源

无old移植；十四助手固定b393bc30bcbeff8a6e4922243820649cec31157c，新增guardprobe固定a280。CPU诊断复用已有qualificationdiag默认off/单文件16MiB限制、正常退出StopCPUProfile，不引入队列/凑批/等待/新参数。

## Actions证据

产品仍a280，13定向Actions、4独立strict/12p95+p99配对检查/P6三目标PASS见windows-dns-guard-a280566-actions-20261006.json。文档本轮提交尚未跑Actions，完整70/18/1800s仅024历史；不得继承。编译/unit/race/常规性能仍全部Actions，每run一条性能样本。

## 问题、排查与风险

D06整链路FAIL business_loss：goodput9.999903/9.999923M，两向各missing3，字节loss .0007691%/.0005707%；2979/2979探针收到，p95=230.0011ms/p99=331.9796ms，latephasep99=367.5272ms。输入覆盖579419/579419，lagp99上界2.1/1.3ms；productCPU308.65625/161.24s，helpers21.984375/36.10356s单列，serverrawdrop102、Windowsdriver/interface/useroverflow0。实际FECcount1/1，FreshBlocked/Bypass/Abandoned/RepairEvicted0、FECpressure0、record/patherror0；shadow4096不是fresh门。on/off两轮p99均偏高，没有证据将高p99归因新DNSguard。WAN不同时间未知丢包，不从聚合计数唯一定位包损失，也不把近满吞吐认成延迟合格。

证据native-d06-dns-off-a280566-seed1468-20261006.json及原始小回执gz，boundedrawpcap已分析删除。当前48完整300s样本、23/43工况、20NOT_RUN，跨SOURCE且含FAIL，非PASS数量。M03已有缺包/迟到未关闭。

## 下一项原子任务

单独CPU诊断S01 seed1469正在运行，默认DNSon、相同Normal1/10M/FEC20，CPUProfile明确启用；不计普通性能资格，保留探针丢失及输入/资源/清理。已有profile_top.py仅解码gzip/protobuf、无编译；Windows sampler可能把阻塞foreign/syscall记到权重里，必须按OS productCPU交叉解释，不能拿pprof百分比当真实CPU或与profileoff直接断言收益。先找可复现的项目CPU热点，再做窄优化与Actions验证；不机械扩FEC/4096/接收buffer，不添加HOL或改变finite恢复语义。
