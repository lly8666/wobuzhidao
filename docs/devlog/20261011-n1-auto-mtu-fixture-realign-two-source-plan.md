# N1 真实默认 MTU 夹具纠偏，最小两源对比启动

日期：2026-10-11
工作分支：`next/adaptive-fec-aes-tun-20261010`
父提交：`1f90ee01a1270938353067347724f8fe682eee9f`；产品代码没有更改
生产 SOURCE：候选 `1c64e4e5163ce80c7e90ee7c8ec4bdb5e3cf3438`，历史基线 `4f0cf7814dea0a04a0dc69863ae1e56412e0da9f`
测试辅助 SOURCE/harness：本提交（具体 SHA 以 GitHub commit 和每次 Actions manifest 为准）。

## 用户明确要求
删除过度人为指定的 record-limit、写入方案、仅小样本高效对比并结束 N1 非必要拓展。外层 MTU1400 不删。所有运行仅 Github Actions；每个 Action 只允许一个 SOURCE/config/seed/scenario、一个测量 job。

## 根因和具体改动
- 原 `strict_weaknet_sample.sh` 在服务端设置1250客户端1300，和产品两个正式 CLI 的默认值0=auto不符。FEC off 情况下，TLS-like 31B 开销给 C2S LINK 1219B、S2C LINK 1269B；业务 UDP1200B+IPv4/UDP28B=1228B，原 C2S 一部分包被分成2个LINK，S2C不分。这解释了该夹具可重复的业务上下行缺失差异，非自动MTU出错。
- 两处额外 cap 已删除，正式CLI自动派生按外层1400、实际MSS/协商继续收紧；manifest 标明双方 CLI cap omitted_auto。未更动正式产品算法或历史样本的任何二进制/失败结论。
- 老夹具8条 flat1/flat5 与 fixed20 门依旧只在其原始辅助 SOURCE/hash/配置下有效；不要拿来当本次默认MTU基线。
- 仅修改既有 relay 请求为`flat5`/`FEC off`两源各1条，固定seed601 Normal1 10Mbps each-way 300ms one-way 120s 30/60/30相同5%，不启 profile CPU。保留源SHA和runner/资源/注入/尾部与原分析器+独立off-v2。
- 若该两条 Actions 仍因 off-v2 MEASURED_NOT_QUALIFIED 而显示整体红色，这是如实报告资格不足，不把红色或传统旧 PERFORMANCE_FAIL 重新涂成 PASS。确切结论从原artifact采集。

## 开发阶段验证
本提交只是实施和实验分发阶段；GitHub Actions、样本 artifact/run id 此时 **PENDING / NOT_RUN**，未声称新的默认MTU业务loss或 N1 完整资格。收尾必须在后续同分支提交中明确记录两个独立完整收据、各指标、是否成功与不支持范围。N1目前为已验证的scoped内核与待完成最后一对默认MTU样本；N2独立可靠物理首发-loss观测没有完成，继续HOLD。80s S2C及多秒probe保持OPEN。物理NOT_RUN。
