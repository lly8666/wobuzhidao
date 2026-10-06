# 服务端端口过滤测试编译修正

## 本轮目标和阶段

P7长尾定位，开始HEAD d9b188ad07dad699f4538d5aa59eaaf835fa864c；实机部署仍7eeb。只修正新候选的测试编译，保持端口过滤产品行为和全部运输参数。

## 修改与原因

raw_port_filter_linux_test.go删除未使用的golang.org/x/sys/unix导入；BPF向量通过已有函数获取指令，无需该测试直接引用unix。没有新增运行期开销或协议/缓存/FEC/HOL改动。

## 复用来源

无old复用；沿用当前候选测试与现有Actions。

## Actions证据

失败SOURCE d9b188ad07dad699f4538d5aa59eaaf835fa864c：linux-server [37403877830](https://github.com/lly8666/wobuzhidao/actions/runs/37403877830)、targeted [37403877905](https://github.com/lly8666/wobuzhidao/actions/runs/37403877905)、foundation [37403877865](https://github.com/lly8666/wobuzhidao/actions/runs/37403877865)均有同一Linux测试编译错误，原始失败日志保留。linux-server产品包构建/manifest通过，但测试编译之后的真AF_PACKET kernel/native步骤没有执行，不能继承为PASS。

同SOURCE preflight37403877822、GUI37403877918、startup37403877885、lifecycle37403877809成功；它们不覆盖Linux faketcp测试门。修正源码所有门待提交后Actions重验，性能NOT_RUN，候选NOT_DEPLOYED。

## 问题、排查与风险

编译失败不是runner容量证据，也不是内核过滤正确性结论。端口过滤能否减少实际负担、是否改善最大UDP长尾仍未知；7eeb最大UDP四迟到与历史p99失败保留。

## 下一项原子任务

修正后精确SOURCE core/race/真AF_PACKET和普通TLS多客户端通过，再独立Normal lossless/5205及Game lossless/5205/5305同源RTT配对。每性能Action一条；通过后P6配套部署与timed M03复验，随后DNS D04。
