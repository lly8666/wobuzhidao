# FEC off 受损性能契约更新：文档生效，分析器待迁移

用户2026-10-11授权修订项目考核，并要求正在开发的agent先完成当前这波，再执行。当前docs起点139a8b83；产品SOURCE仍为1c64e4e5163ce80c7e90ee7c8ec4bdb5e3cf3438。不更改源码、workflow、夹具、分析器或正在运行Actions；不dispatch性能、不部署物理。

交付在独立文档分支 `docs/fec-off-acceptance-20261011`，不推进正在开发的adaptive分支HEAD。原因：现有perf relay验证远端分支HEAD必须仍等于冻结harness SHA，直接向工作线push文档也可能阻断该relay后续波次。开发agent结束当前波次后再cherry-pick本次docs提交，处理STATUS冲突时保留其最新phase/SOURCE/Actions/进度，并追加本政策；不能把此文档分支旧STATUS整个覆盖回最新工作线。

新增docs/FEC_OFF_PERFORMANCE_ACCEPTANCE.md，同步ACCEPTANCE、ADAPTIVE_NETWORK_PLAN第9.3节、WEAKNET_QUALIFICATION当前覆盖入口和唯一STATUS。STATUS原active phase/开发步骤/功能资格保留，旧最新进度日志为docs/devlog/20261011-190000-n1-resumed-640pn-pass-realpath-mature-feedback-perf-dispatch.md，只追加验收迁移任务。

旧off/5205 run https://github.com/lly8666/wobuzhidao/actions/runs/38105045359 保留FAIL：产品SOURCE1c64e4e，正确性/输入/环境/抓包PASS，PERFORMANCE FAIL。C2S stress及时goodput6.802587733Mbps、packet loss23.387625%、byte loss31.977452%；pre/post packet loss4.628854/4.827530%，字节loss7.957806/8.108007%。旧分析器要求loss<=p、goodput>=(1-p)*0.99和post5三个1秒窗口；分片与字节分布使这些off通用假设不充分。上述事实不等于该产品已通过或所有异常均合理。

新契约仅修改off受损考核：区分包/字节/及时与最终交付，IID按真实分片m的q=1-(1-p)^m分别加权作为解释参考；看可比基线的goodput/CPU/p99与相对pre5恢复，完整性/隔离/MTU/连续性/无HOL/资源有界不降门。模型或对照不足记MEASURED_NOT_QUALIFIED/REVIEW_REQUIRED，不能删硬公式后自动PASS。lossless、nonoff与formal20旧门不改变。

文档与JSON完整性通过本地阅读/文档处理核对；未执行本地产品构建或任何测试。新分析器实现NOT_IMPLEMENTED，新policy验证NOT_RUN；docs push自动foundation结果须另行记录，不声称已PASS。所有历史FAIL、当前正在跑的旧分析器样本原样保留。

下一步：先收尾现有N1波次，再原子修改profile-off受损分析器和meaningful定向Actions验证；用新版本receipt，旧参考并存。新性能每Action一SOURCE/配置/seed/场景/job，完成后继续N1/N2..N6，physical仍NOT_RUN。
