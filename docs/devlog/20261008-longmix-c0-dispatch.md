# C/0%真实TCP+UDP混合业务：独立正式Actions运行申请

正式原冻结SOURCE `b4ea061178a6e09b7e7c8587d72b4b8535492567`，助手准入 `31ac81d2a7904da177a888b61389c16c149c5aa3` / https://github.com/lly8666/wobuzhidao/actions/runs/37733698703 PASS，配置Normal1、FEC20:20、padding/profile off、内层服务TUN9000/客户端Linux TPROXY不冒充Windows、外层1400、300ms单向、netem双向0%、300秒+独立drain10、seed2608103。每方向**TCP5Mbps+UDP5Mbps=10Mbps**；3条双向长TCP+每秒独立短TCP，同时96/512/1372/8972/8973/65507B真实UDP按字节比例10/10/10/15/25/30，额外96B独立UDP探针在UDP配额内，TCP背压不转移余量给UDP。两端都有真实主动Socket sender，client/server仍各唯一一份正式Go进程/AF_PACKET/TUN产品数据面。C runner只有一个业务场景；B/p0重复 [37733833093](https://github.com/lly8666/wobuzhidao/actions/runs/37733833093) 在另一独立Action运行，不是同run A/B。

该提交仅请求独立workflow_dispatch，当前尚无C原始结果不得写PASS。预期b4 Linux TPROXY MaxPayload8936对UDP大包仍存在固定失败；必须完整报告TCP字节/hash、每流背压/真实MSS、UDP大包与小探针缺失/分位数并列，检查TCP持续与大小包是否互相阻塞，不能因C未来失败归因runner，也不能把Windows ARM原生M03长尾套进该Linux入口。原A0/B0 FAIL与原两条A助手INVALID保留；独立产品最小no-truncation提交2f7bb59功能PASS但C仍固定使用b4；无实机操作、冻结ref未移动。
