# ACK entered test Foundation 与 Lifecycle 双绿后单独派发 Game4 0loss 同seed基线（2026-10-09）

仅工作分支 `next/performance-efficiency-20261008`，父HEAD `5e2d3d2295b96e3d64b578eab0c315f2ea18b73e`；规范主线不动，无物理设备，**产品源未变**，仍`ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072`。本轮只把一条 `.github/efficiency-e0-sample.json` 修改为`loss=0`，其它模式Game4/mixed/seed1840/FEC20:20/逻辑3Mbps每方向/300ms/300s+3s/profile OFF/普通尺寸全部与真实5205样本 [37853468730](https://github.com/lly8666/wobuzhidao/actions/runs/37853468730) 配对；正式服务端真实TUN MTU仍须从新artifact读取。

上一条测试修复的最终 [Foundation 37856615069](https://github.com/lly8666/wobuzhidao/actions/runs/37856615069) exact helper HEAD `5e2d3d2295b96e3d64b578eab0c315f2ea18b73e` **SUCCESS**：Linux定向`^TestACKFeedback`重复100次、`go test -race ... -count=30`全部成功，原`go test -race ./... -count=1`成功；Windows单元构建、Linux共享TUN/OpenWrt TPROXY/fallback等通过。同SHA [Lifecycle37856614933](https://github.com/lly8666/wobuzhidao/actions/runs/37856614933) **SUCCESS**，包含生命周期race重复。原 foundation 37853468530测试断言FAIL、workflow非法无job37856284837、过严close断言生命周期FAIL37856286059都在STATUS/evidence保留，不能删除历史失败。没有更改任何产品Go代码或队列/RTO。

本次新性能Action遵循每run仅一个SOURCE/配置/seed/场景和一个测量job，不与其他候选拼接。原始结果还没回来，**PENDING_ACTION / NOT_RUN，不是PASS**。测试目标：无损双向所有合法普通UDP尺寸/独立probe 0缺与0超时、TCP hash和HTTP(S)完整、1s/3s期限/持续交付、客户端与server raw额外socket/interface drops0、真实阶段netem0%、吞吐预算、真实TUN MTU1273预计但不得以预计替实测、CPU/PSI/host strata。任何FAIL原样保留，诊断profile ON不得与 OFF CPU对照；CPU收益仍UNPROVEN。之后再分别 dispatch Normal1/Game2 lossless和真5205（不同run），再诊断/低RTT；旧Game4 446缺、packet140drop、TCP-only hash及大UDP边界、E7 80秒下行OPEN，E6/P6/物理NOT_RUN。

意图与待核实字段见 [evidence](../evidence/performance-efficiency-e4-game4-lossless-seed1840-single-dispatch-20261009.json)。提交时helper身份为该GitHub commit SHA，非此文档父HEAD。
