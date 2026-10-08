# 2026-10-08 MTU 丢包与大 UDP 矩阵验收（独立证据分支）

- 测试源码 SHA：`db3f27ff7633fabe2c003d9432ba57f6efd1093d`，分支 `work/mtu-loss-matrix-20261008`。
- 第三轮 `https://github.com/lly8666/wobuzhidao/actions/runs/37743578571`：run_attempt 1，**completed / success，36/36 jobs PASS**。
- 场景：外层 1300、1400 × 0%、5%、10% 丢包 × TCP/UDP/UDP-jumbo × 内核 socket/WBD LINK，共 36 独立进程；max-parallel=6。
- 第一轮 `37743142959`：35 PASS 1 FAIL（1300、5% UDP 收包异常，原报告未给计数）；第二轮 `37743460100`：24 PASS 12 FAIL（runner 拒绝 netem seed 参数）；两者均**永久保留 FAIL**，不得重写成绩。
- 最新真实 Linux socket 0% UDP 普通包各 88/88；UDP 大包包括 65507 字节 payload 各 8/8；DF+超限触发 EMSGSIZE。10% 丢包下 UDP 丢片可能导致大包 0/8，不能宣称 UDP 保证投递；接收成功的每包字节与原样相符。TCP 通过真实内核重传完成完整有序交付。
- LINK 路径在 FEC off 和当前较紧方向 record 上限 1250 下，源 LINK frame MTU 1219，0% 丢包下模拟 65507 UDP IP 包 48/48；10% 丢包时实际可能丢失部分大型 datagram，且 LINK 65535B 接口能力**不等于**产品内层入口 9000B/平台业务 8936B 兼容。
- 本次不修改既有 MTU 推导、FEC、repair、冻结版或旧 12 次 longmix FAIL。
- 尚未运行：真实 Windows Wintun、完整 WBD client↔server 隧道、FEC-on、端到端 UDP 8936/更大 payload、CPU/PPS/p99、物理链路。
- 证据：`docs/evidence/mtu-loss-matrix-20261008.json` 记录完整 36 job ID、精确 SHA、三个 run 以及边界说明。原始 job log 由 GitHub Actions 保存，本次无 artifact 上传。
