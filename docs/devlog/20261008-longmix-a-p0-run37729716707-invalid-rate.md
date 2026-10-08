# A/0%完整300秒助手无业务失效定位：rate argparse浮点与字节限速严格类型不匹配

- 日期2026-10-08，独立分支 investigation/longmix-20261008，产品 SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`，冻结ref未改。助手 SOURCE `9b22df8eb74c144fcaf686e4e1b4e717e2216ae8`，正式子run https://github.com/lly8666/wobuzhidao/actions/runs/37729716707，artifact 11529671540，sha256:0994bb3b498493b7e1be0b6e7b567104a6620fb9a103e5bb116825a98d30fdc7，raw仍判`INVALID`。
- 使用正式b4两端Go进程，日志有client route-mode=all/direct_prefixes=0；两端 AF_PACKET `SO_RCVBUF` requested=524288/effective=1048576/limited=false。netem stage_start->end约300秒、双向0%/0drops，期间qdisc仅少量控制与连接流量。**不能据此声称有了300秒10M样本。**
- bz/target日志同时于 `active_data_sender()` 抛出 `ValueError("invalid Mbps")`，调用栈指向 `bytes_per_second(rate)`。助手 argparse 的 `--rate-mbps` 为float `10.0`，而负载合同仅接受整数。两个业务进程退出，strict脚本 `wait`在写末尾manifest/路由收据前非0，analyzer因manifest缺失输出`INVALID`。这早于业务/TUN/LINK/FEC，因此不是产品MTU、FEC或RTO缺陷；不能赋予CAPACITY_LIMITED。
- 最小修复只在业务CLI边界 `whole_measured_mbps` 校验有限正数且整Mbps，把10.0转整数10，再沿用原完全相同的`Fraction`字节配额。不放宽seed/时间/负载、不能重复同run，额外单测拒绝10.001、负数、nan、inf、bool与str。
- 前次 https://github.com/lly8666/wobuzhidao/actions/runs/37729338574 因sudo ART丢失`INVALID`原样保留。待本次GitHub Actions新preflight完成，再单独dispatch一次A/p0/b4/seed2608101，诊断结果另建证据。B/C与其它11格NOT_RUN，M03/1554约80秒单向失活与1225.578ms大包晚到仍分别OPEN；没动产品/冻结ref/物理机。
