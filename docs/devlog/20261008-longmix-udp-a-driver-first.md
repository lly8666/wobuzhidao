# Longmix第一阶段预检通过；新增A用真实UDP socket的候选助手

固定产品 `b4ea061178a6e09b7e7c8587d72b4b8535492567`，前一个helper `3ac1c8b38cbc513659274cfd3412639d07b22342`。Actions https://github.com/lly8666/wobuzhidao/actions/runs/37727692438 整run SUCCESS：负载计划5/5、repository-contract、sample-workflow-policy和既有真实Linux内核9000 MTU/DF/65508边界全部通过。之前37727548578仅因sudo环境漏传而FAIL，保留。

新增 `tools/longmix_udp_business.py`，用于未来A正式全链路：真实独立双向UDP发送源和接收器，采用应用字节虚拟公平排程而非固定PPS；用UDP8972/8973/65507合法可分片socket；每端含独立96B probe socket及接收线程；大包发送通过有界数值ACK（非业务全文）关联RTT，不把目标有效收包和回程ACK丢失合一；每尺寸存offered/send/first-accepted/late/错误及返回RTT、10ms接收桶、有效缓冲读回。预留固定8192B/s反向大包ACK和960B/s探针，从双向10M UDP总预算扣除，不自动回补，保守允许未用保留带宽。未执行真实产品或TCP。

本commit追加单元用例与preflight路径触发，必须看新run真实结果再完善严格隔离netns、outer1400/inner9000、固定netem丢包实际计数、生命周期/资源与单样本验证。此时A/B/C 12条均NOT_RUN；不移动冻结ref、无产品修复、无物理动作。80秒下行失活与后续1225.578ms大包迟到仍分别OPEN。
