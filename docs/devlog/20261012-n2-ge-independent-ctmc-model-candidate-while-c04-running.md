# 2026-10-12 N2 GE 相关突发独立参考：模型候选（绝非性能PASS）

已启动且仍在运行的 C04 SOURCE `c835c6d9986a1a5ce7838c4bb63e1dbcb169c8e0`，Action [38217154364](https://github.com/lly8666/wobuzhidao/actions/runs/38217154364)，job `114704454429` 继续为独立840秒一配置一场景一测量job，模型提交不会触发第二项**性能**Action或更改正在运行的SOURCE。C03长流修复后的交付Scoped PASS和 recover5 C2S过度保护 FAIL 已在唯一STATUS及正式evidence永久记录。STATUS.next_task仍以C04结果为下一性能操作。

现新增预冻结C10相关突发理论基础 `tools/adaptive_n2_ge_reference.py` + `tools/test_adaptive_n2_ge_reference.py` + `docs/evidence/adaptive-n2-ge-ctmc-freeze-v1.json`。独立数学解析器不import产品 controller，不给产品读取netem真值或测试phase，也未运行任何本地构建、unit/race、功能或性能。模型v1从时间驱动Gilbert–Elliott连续马氏链出发：good平均450ms、bad平均50ms（稳态bad占10%）；good擦除0、bad对每个源/奇偶shard独立条件擦除50%，长期平均外层约5%，但**同一组内擦除高度相关**。使用CTMC精确转移矩阵和以发源为目标的动态规划，按20个内的真实组k、配置R经min(k,R)生效、systematic先发而parity在组关闭后发送，精确求P[指定source擦除且总擦除超过真实r]，分组source权重合并。它与生产FEC和控制器算法独立；同时验证k1同瞬间源/一枚parity残余为2.5%（远高于IID对应的0.25%），并覆盖常数条件loss退化为IID二项尾、连续状态正常化、partial混合、不同发包间隔和非法参数等反例。所有测试由新GitHub Actions工作流运行，尚未得到结果不称PASS。

冻结参考先比较四种**明确假设**：full k20 systematic间隔0.42ms/parity0.1ms、full k20源间隔2ms、含少量partial k5的假想组混合、稀疏k1，使用固定0.1%模型目标。**没有把它们写成已观察到的native物理时间线**；真C10必须基于实际源组k/系统时序可观察程度重算并另列缺失的不确定性，条件时间假设的结果不能直接作为FEC“绝对应该升到R20”的命令。原curve验收 C10 仍为 P2，拟采用1ms时间步或等价事件trace，实际injection/trace/burst间隔需要显式冻结并核hash后才运行。不能让全bad黑洞靠加parity解决或要求所有弱网探针返回。

新 `.github/workflows/next-adaptive-n2-ge-theory-only.yml` 仅**一个纯模型unit/reference job（0个性能测量）**，push新模型文件时运行：全Git树check、repo/performance工作流政策、数学反例unit、冻结reference JSON和SOURCE/helper/manifest/output SHA256收据，不产生产品流量、不触碰netem/真机。若模型FAIL需原子保留日志与根因，不可修改阈值追结果。

后续仍按当前STATUS顺序：先C04正式840秒实测并按保护/首次交付/p99/overhead/真实off恢复分判，再补1%/10%双向不对称和相关GE C10，不铺巨大矩阵；达到合理N2保护/成本后N3 AES、N4 Windows TUN、N5 GUI和N6正式包。历史2.73秒探针尾、80秒下行、C03旧41/46秒 FAIL保持记录。Physical NOT_RUN，禁自动部署、移动qualification ref、合主线。
