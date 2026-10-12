# 2026-10-12 N2 M0：独立理论参考和 recover540 只读诊断候选

工作分支 `next/adaptive-fec-aes-tun-20261010`，开工远端 HEAD `5a1f083605e4882511398bdcd01d78b5f844d13a`（上一个产品 SOURCE `90925f81cfb16e8435c238b060c46492eef017ea`）。本轮未修改 production controller、FEC、质量控制或配置参数；没有运行任何本地 Go/build/unit/race/产品性能；没有触碰主线、qualification ref 或物理部署。容器无仓库工作树且无外网 git DNS，通过 GitHub connector 读取远端并以原子 Git tree commit 更新分支，创建提交前必须核 HEAD 租约。

## 权威与范围

完整读取 AGENTS、PROJECT_CHARTER、STATUS、ADAPTIVE_NETWORK_PLAN、**ADAPTIVE_FEC_CURVE_ACCEPTANCE**、AGENT_CONTINUITY、templates prompt，以 STATUS.next_task=先建立独立曲线再定位 recover540 为准。既有 V3/N1/Normal live auto 不重复实现。N2 曲线合同仍是待验标准；N3～N6 未完成。历史 2.73s off probe 和约 80s S2C 问题不改变 OPEN。

## 只读原始证据

此前 SOURCE `90925f81`、Action `38161148715`、job `114534148614`、artifact `11710956407`，压缩产物 GitHub digest SHA256 `090a5bd2d5e4fda32309a5f37dc616c836c526c71b3adf7eddc3e87d7900ad3f`、内部 summary SHA256 `85163b36ba051a76adf33fef5d748e8a224e0e8d5fac2003270d5aa52bfc8450`。helper derived `b4a669eb3ecf55c915a84749a5a9444d769e05dfb5ac4b54a23bfd70096ef519`、wave-stage `d23aef668756a085ff1a154ae9cf37e57eff3147ef983758a4364a5f134b89cd`，业务seed 2026092201/2026092202；内核 tc seed 未生效（UNSEEDED）。一份场景：两向各 10Mbps、各 300ms、MTU1400、0/1/5/20/5 各60秒、最后干净240秒、再10秒drain、诊断ON。

双向 off→4→20，恢复阶段 C2S 活跃240s始终20、仅drain降16，S2C 在活跃期降off。各方向各1,332,237业务包，C2S缺389、S2C缺272，都在最初1%适应段；probe539/540，返回者RTT p99约621ms，非全部返回者p99。诊断 ON CPU 308.65/312.62s 不能归为普通 OFF 优化；quota UNKNOWN、capacity flags 空，不等价于完整容量归因。本轮不重写原 `SCOPED_OFF_START_BOTH_DIRECTION_RAISE_OBSERVED_UNSEEDED_REVIEW_PERFORMANCE` 或产品 FAIL/OPEN；不上传大原始pcap、业务正文或密钥。

逐秒 control/source 复核：恢复240s，C2S source seq约172→304、last-qualified seq约172→303，S2C约171→303。每60s的近似已资格化/报告序号增量 C2S 为 32/33,34/33,33/32,32/33；S2C 为 27/31,30/32,33/34,34/33。比值偶可大于1，因为一秒快照跨过了多个 source；不能叫精确source接收比。C2S 在约428、429、451、507s分别出现 `M=0,M+R=1,8,5,2`，S2C在367、402s也有此类上界。**这些 R 仅证明 repair attempt，绝不证明首发实际线路丢失。** 256-PN窗口包含较大边缘 cropped source/parity，390s上行典型仅3个完整块及 76 cropped-source/60 cropped-parity；cropped 边界不等于实际组未保护。C2S Wilson upper 约6.8%→5.7%→4.8%→0.59%→0.30%，最后 `CONFIDENCE_RESIDUAL` 阻塞降16；S2C最后约0.047%可降off。仅靠延长干净尾段不能完成效率资格。历史时间戳年龄和独立闭合实际 k 尚无法从已有快照完整重构；记录 UNKNOWN 而不是改写为0。

机器/实际业务端口/netem/CPU完整细节来自原summary和run，保留原工况/失败。新只读缩减收据在 `docs/evidence/adaptive-n2-recover540-diagnostic-audit-candidate-20261012.json`，随代码的 Actions audit 以后输出更多逐分钟指标；后续回填 run/job/artifact/hash，旧样本原标签不变。

## 预冻结模型、计划和本轮实际改动

新增 `tools/adaptive_n2_curve_reference.py`（v1）：**不引入任何产品包**，直接 math.comb 枚举二项尾概率 `p*P[Bin(k+min(k,R)-1,p)>=min(k,R)]`，支持按实际完整 k 与 source 字节加权；最低档选择只按0.1%模型阈值；k1在p5%下 q=p²=.25% 且全部档不可达，明示 UNATTAINABLE。独立调度预算纳入2s反馈、3s成熟、RTT、512记录/256PN窗口、预声明0.5 qualified-report/s、32历史窗口、fast2s/slow15s、下降30s/入off60s及每档10s。时间带为模型假设和筛查限，不是产品 Step 回放、不是以观察到的C2S慢结果事后拟合。若真实样本不足/反馈丢失/有上界歧义则 QUALITY_LIMITED，设计效率仍 OPEN。

冻结 `docs/evidence/adaptive-n2-curve-freeze-v1.json`：先测P0模型及原540只读诊断；准备 C03 默认 initial20/min4 的 780秒波形、C04 initial0/min0 的 840秒波形，C08 1%、C09 5% 代表工况；分别一配置一SOURCE一场景一测量job，**本轮没有启动性能run**。完整k20 IID仅是预冻结理想对照；实际 partial/长度组必须另算，边缘crop不能冒充真实无保护。普通稳态理论档或高一档，末60s业务活跃时间90%、90/180/240s效率screen及允许时间带均在测前固定。相关突发、黑洞、乱序/抖动、方向非对称、反馈故障、稀疏、idle/rotation、TCP/UDP/MTU、aggressive、Game/fixed/off 的 P1-P3 次序仍按曲线文档，不能用4个IID样本代替。

新增 `tools/test_adaptive_n2_curve_reference.py`（数学全枚举、p=0/1、R=0、k1、IID参考梯度、独立时带、freeze防篡改）及 `tools/adaptive_n2_curve_audit.py`（只读540s已有jsonl/summary、source seq增量、M/R、窗口crop、挡位/置信/阻塞，只有有界JSON输出）。新增 Actions `.github/workflows/next-adaptive-n2-theory-audit.yml`：push新模型文件时自动运行一个**纯模型/只读审计job，无性能测量**；只从已有 run 下载指定artifact，核summary SHA256，只打包所需诊断六件、删除临时原产物，仅上传 model JSON、audit JSON 和哈希收据。所有程序单位验证在 Actions，不将原诊断ON CPU冒充普通OFF。本提交候选在尚未获得 Actions 结果时绝不写PASS。

## 后续检查与风险

1. 看这个确切新提交自动产生的模型 Actions/job/产物，先修任何unit/fixture/YAML问题并留FAIL；若 run 无法读先前artifact则降级 UNSUPPORTED，不能虚构验证。
2. 把M0独立参考与原540逐分钟结果并列，计算 Wilson、R false-positive、历史年龄/组形状的识别界。现有 32 窗口旧历史已随合格报告逐步退出，但“R上界尖峰×边缘按未保护source计价×0.0005强降门槛”可能持续造成高档；先定位并明确修复风险，再考虑对统计/软门做**跨工况**最小调整；绝不把 R 强行设0、按netem相位强降、给方向/seed加特例或更改same-wire/fresh等硬门。
3. 每个生产修复先 Actions core/race，再对应真实路径单条最小反例，基本过关后各拆独立性能Actions扩展到非对称、burst、反馈、稀疏和固定/Game guards。N2未资格化，N3–N6继续NOT_STARTED，physical=NOT_RUN。

本轮 GitHub workspace 状态通过远端 commit/tree 核验；本地没有检出的工作树，不能虚称 `git status clean`。不移动 qualification ref、不部署、不合并。
