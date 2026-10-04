# 20261005-080000 Windows诊断页面接线

## 目标、来源与修改

父SOURCE046c0a91860e09d5fe1e58556235771b6ff0800d。Actions GUI run37245396645明确FAIL：新diagnostic字段已加入全集，但MainForm只建原四个section，LoadEditor遇缺少widget的key抛KeyNotFound。属于本轮引入的GUI接线缺陷，不是driver/产品性能根因。将诊断页加入实际section数组，保留真实每字段widget→check-config测试，不放宽全集校验或删诊断参数。无old复用，无协议/数据面变化。

## 测试状态与风险

父SOURCEdefaultnetwork37245396617已SUCCESS，其他core/Normal5205 37245428800/Game5205 37245431382/P6 37245460818等仍运行或排队，不继承给本修正。新HEAD GUI/core/独立Normal与Game5205/P6需按精确SHA验证后才能部署；父GUI失败原日志保留。现部署仍3e3e094，S16 seed1303完整300s已收：generations1..6、最多physical2、exit0、Record/Path错误0；下行9.69393M/3.06%字节损失、71probe超时仍质量未通过，1秒rotation细门未评。当前DNS故障D02 seed1304运行，本源为3e3e094，不是未验诊断HEAD。

## 下一项

验新GUI与core/race/每run单性能样本，再同源配套包复测D01双端诊断。收DNS故障互备证据，仅本lease/53规则，finally删除。继续已授权验收，不扩buffer/FEC/4096、不凭WAN猜测改算法。
