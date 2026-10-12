# 2026-10-12 N2 C04 独立 off 起步曲线 840s 单实网工况预冻结 dispatch 候选

上轮精确SOURCE `08e63965dd9b2910e2e8ea073a22bbc3d0032db6` / Action38212621418 job114689998724 artifact11731170110 长流2³¹故障定向修复后 scoped realpath PASS，但 recover5 上行最后60秒 R12-R20，冻结R8-R10内0%，持续过度保护仍FAIL（不能认为完整N2效率通过）；`docs/evidence/adaptive-n2-c03-longserial-success-recover5-efficiency-fail-20261012.json` 与原C03 41/46秒失交的 historical FAIL 永久保留。本轮不改N2 soft controller、window/slow门、对单方向或seed开特例；下一条跨配置证据选择**预先冻结**的C04 off 起步波形，而非重跑同一个C03 seed挑好结果。

C04 独立 input 为早已冻结的 `docs/evidence/adaptive-n2-curve-freeze-v1.json`，模型 `n2-independent-iid-timeband-v1`，SHA256 `288d005f6b419d41071648dd778191e8236fdc9abe2fa07d64cb0f0086262a8f`。单Normal自动策略，CLI显式`--fec-mode auto --fec-min-parity 0 --fec-start-parity 0`、服务端maxR20、ChaCha、MTU1400、300ms每方向、10Mbps业务每方向、业务seeds 2026092201/2026092202，kernel tc seed UNSUPPORTED明确UNSEEDED。预声明一条损伤曲线0→1→5→20→5→0：前五段各120秒、最后0%240秒，总活跃840秒另10秒drain。完整k20 IID理论示例最低档 R0→4→8→16→8→0，允许档[R0,R4]→[R4,R8]→[R8,R10]→[R16,R20]→[R8,R10]→[R0,R4]；真实partial k按本模型另算，边界crop不能直接当不保护。实际线路/netem样本用于参考分析，不输入产品决策。

新增 `tools/adaptive_n2_c04_stage.py` 和 `tools/adaptive_n2_c04_wave.py`，从已在 Actions 有正式产物的C03 five-netns夹具隔离派生，只更改 off 起步显式选项、840秒和冻结C04型号。原有业务、认证、路径、保护性发包、FEC SIMD和诊断不动。preflight fail-close校验六段起止、source/辅助文件SHA、C04初始和最低parity0、业务MTU和rate、真实netem双向计数；保留每段首次交付/missing/p99、probe返回/缺失、持续空洞、source M/R、实际partial和FEC编码parity字节、总体wire、CPU型号/ISA/配额/PSI。Owned pcaps仅hash并清理，凭据正文不上GitHub。

新增只读 `tools/adaptive_n2_c04_longserial_gate.py` v1及Python模拟健康/历史41-46秒业务空洞的单元反例，在GitHub Actions计入测量前硬门：最后0%活跃最后120秒[720,840)每方向不得连续>=5秒首次唯一业务空洞，wall first-unique/sent至少90%（按1秒桶可能少量跨界>1不是业务复制）；最后0%活跃末60秒档位>=90%落冻结[0,4]，并且**必须实际见到 off**；20%高损末60秒>=90%落冻结[16,20]；原摘要不能有错误或容量旗标、最后阶段实际tc0%，不将弱网5%/20%业务或探针强制全部返回。C04任何阶段实际未满足预先固定的保护/省资源档与B时间带时单列 FAIL/QUALITY_LIMITED，不用全流程绿盖掉；不允许针对C04的结果事后调参考。

新 `.github/workflows/next-adaptive-n2-c04-longserial-regression.yml` 是**单SOURCE、单显式配置、一预声明波形、唯一测量job**，不是A/B多配置串行。执行顺序：仓库及performance单样本检查→冻结input与C04 stage校验→独立Python gate单元→Github Go core/unit/race→构建正式产品→单一840s protected业务→两个只读analyzer→enforcement和仅有界artifacts。CPU诊断ON不跨不同VM证明优化。任何helper错误或产品初发缺口都必须留原Action失败与限制，不重跑两次同失败无新证据。

本提交只登记待执行C04，**绝不预填run/job/artifact或声称实测PASS**。后续先比较C03/C04上行收敛/M-R上界和历史窗口滞留，再决定最小泛化soft控制调整；继续N2 burst/asym/feedback/sparse/idle/rotation/mixedTCPUDP MTU/Game守门，达到合理保护+开销后N3–N6。Physical NOT_RUN，历史2.73s尾和80s S2C仍OPEN。不部署、不移动qualification ref、不合并正式主线。
