# E0 Normal双向10Mbps UDP普通包真正profile-off单样本通过（2026-10-08）

独立源身份：产品SOURCE `bf11fbfbe64d518e7ba189d51bfb4512df4df733`，helper SOURCE `e2551aebbeeed5220dc35666f60879ad54cf7ac1`，精确目标分支 `next/performance-efficiency-20261008`。独立[Actions run 37761141407](https://github.com/lly8666/wobuzhidao/actions/runs/37761141407)，job 113257560841，artifact 11542641955、attempt1、单一业务样本；原始workflow `success` / analyzer `PASS_SCOPED_ACTIONS`，issues=[]。先前run37760789251由于helper缺少import os导致**未运行样本的FAIL**完整保留，绝不追溯改绿。完整原始summary、manifest、runner-host、资源、netem、link、pcap hash删除证明保存在Actions artifact。

## 条件/有效性
- 300秒有效注入+严格3秒drain，Normal1（并非Game4），seed1801、双向每向**总业务字节10Mbps**，UDP包96/256/512/1000/1372/4068B，FEC20:20、padding-off、单向300ms、受控0%外层netem。
- 正式client/server进程与OpenWrt TPROXY -> 加密raw传输 -> Linux共享真实TUN -> 真实socket目标；自动record cap configured0、outer1400、TUN `wbdg0` 实读1273，内部veth9000绝不冒充产品接口。正式默认tick由源代码和manifest标100ms（尚未测到期执行分布，不冒充8ms发出证明）。
- **普通off生效**：client/server诊断JSONL均未生成，resource_report均`present=false`，不运行CPU pprof或per-record timing；CPU/PSI/drop取侧车低频OS资源采样，不依赖产品调试时钟。

## 两向原始业务质量
- C2S：发375002412B（含小probe），合10.000064Mbps；收374858412业务B，9.996224Mbps；每包长档的missing和corrupt均0，send_error和skipped为0。探针1500发/1500回，未回0，幸存p99=602.418384ms；连续10ms主动期无零业务桶。
- S2C：发375001932B，10.000052Mbps；收374858412业务B，9.996224Mbps；各普通档missing0/坏包0、send_error0、skipped0。探针1495发/1495回，未回0，幸存p99=602.328961ms；连续10ms主动期无零业务桶。probe未回0因此此处returned-only p99并未靠隐藏缺失缩小，但不能拿它覆盖迟到/所有业务p99。
- socket和interface drop=0、无资源校验错误。无大UDP档，此scope不包括8972/8973/65507及派生MTU精确±1B，不涵盖TCP、短HTTP/HTTPS、混合或Game。

## CPU与环境，严格禁止跨宿主收益推断
runner ubuntu-24.04 Azure x86_64，**AMD EPYC 7763**、4 vCPU；client CPU 146.66s、server 146.35s/300s，峰值RSS 32.30/32.85MiB。host max busy30.34%、steal0、CPU PSI some avg10最高31.89；cgroup cpu quota没取得足够确定的数值，不推断无限资源。旧run37755799764为**Intel Xeon 8370C**、server计时on、15s drain，其client118.26/server118.02 CPU-s：CPU架构/测量模式/runner时刻不配，不准作CPU收益差值、不得挑最优宿主。当前只建立单条真实业务**基础档**，未达到方案要求的多独立同类机器重复、中位数/离散度，更无alloc/repair/batch细账或E1收益。

该run对应原始二进制SHA256：client `f14409ad6bcc8a0296be59a319de35e35f5a86e21cfbc348c1dde35dc6eeeec5`；server `498728fcea780294643e0dd82273a82824294de9c05d67f641a959513dbefe16`。各原始字段见 [结构化evidence](../evidence/performance-efficiency-e0-udp-normal-off-37761141407.json)。

## 下一步
逐个独立300s Actions：至少四条持续TCP与真实短HTTP/HTTPS（必要补helper与正确证书校验）、TCP+UDP 5+5Mbps混合、Game4逻辑3Mbps双向与大UDP能力边界/不截断，普通off账本和单独诊断账本分开。先完成E0基础而不提前修改产品E1。全程保留首次业务优先、同Seq密文、generation、4096不是fresh gate、原80秒下行失败E7仍OPEN。E6/P6/同源包/物理资格均NOT_RUN；未写PHYSICAL_PASS。
