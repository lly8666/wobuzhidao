# 2026-09-20 P2 half-close测试模型修复

## 基线与Actions证据

当前工作基于 `0128163f747e7c1cfd45e4aff6ee2a2309908f4c`。更早实现SHA `5c074f1f2b3610370cae3f851218ff0be6c86699` 的 Actions run [35456984372](https://github.com/lly8666/wobuzhidao/actions/runs/35456984372) 已明确FAIL：repository-contract PASS；Linux/Windows unit都在新增测试的不可比较 `Segment` 编译错误处失败；同时 `TestUnrecognizedHelloFallsBackOnSameAssociationWithExactReplay` 在2秒后超时。该SHA不具备P2资格。

## 根因

1. 第一轮静态修正漏掉SYN-ACK retry处第二个 `retry != first`。
2. fallback超时不是要把产品重新改回full close。新 `BootstrapStream.CloseWrite` 正确只发FIN并保留read方向，但测试 `associationPeerConn` 仍是旧内存模型：它既不会消费/ACK服务端FIN，也不会在客户端关闭发送方向时向association发送FIN。旧测试之所以能退出，是依赖产品层“没有CloseWrite就直接Close”的截断性workaround。

## 修改

- 删除剩余不可比较的Segment比较，改为序列/ACK/flags/window/四元组/payload逐项确认。
- 测试peer新增TCP half-close：服务端FIN消费一个seq并回ACK；客户端 `CloseWrite` 发真实FIN并要求累计ACK前进；full Close在half-close后只关闭测试端等待器。
- fallback测试在读完decoy `close_notify` / EOF后显式half-close客户端发送方向，再等待splice退出。这样测试的是双向独立结束，不再依赖一个方向结束就截断另一方向。
- 产品 `fallback.go` 不回退，P3目录不改。

## Actions / SOURCE_SHA

本提交创建前未在本地运行编译、测试、race、fuzz。资格只绑定承载本日志的新SOURCE_SHA；`0128163f747e7c1cfd45e4aff6ee2a2309908f4c` 当时的run仍独立记录，不作为本轮PASS依据。

## 风险与下一步

需要Actions确认新的测试peer没有改变既有TLS/admission时序。通过后再开始realityfront ticket/ALPN/transition writer收口；P2真实kernel TCP/TLS + pcap门槛仍未运行，因此保持OPEN。
