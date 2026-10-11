# N2 guarded auto controller SOURCE25f6747 GitHub Foundation PASS

日期2026-10-11; 工作分支 `next/adaptive-fec-aes-tun-20261010`。

GitHub Actions [38120920936](https://github.com/lly8666/wobuzhidao/actions/runs/38120920936) 启用7 jobs全success: repository-contract, active-go-tests Linux/Windows, Linux p2 fallback, p4 OpenWrt TPROXY, p4 Linux shared-TUN nft和iptables。Linux包含go test ./..., go build ./..., linux arm64 cross-build, go test -race ./...,定向TCP与TLS fuzz。无native physical设备核验; 扩展P5/P6 skip不算pass。生命周期 [38120920958](https://github.com/lly8666/wobuzhidao/actions/runs/38120920958) success。

保留首次核心 `47b94777` GitHub [38120812356](https://github.com/lly8666/wobuzhidao/actions/runs/38120812356) failed assert，不改旧结论；修复提交 `25f67474869c107c68f228a5f86b9923a3563f8d` 仅把累计512条已有效发送记录之后允许快升的测试断言修正，产品`controller.go`没改。

N2当前仅纯决策核心且未生产接线：录入唯一匹配GEN/source和非重叠PN范围后，当且仅当有实际首发可信证据、有效k、无capacity、时效内，方可更新EWMA；N1 authenticated echo仅final deficit，不是首发物理loss，必须HOLD。混合partial k分布、首发真证据/保守上界、动态TX组边界切档、旧block RX兼容、rotation与N2 fullstack都尚未开始或未完成。仍保持 admission auto UNSUPPORTED，现有 fixed/off产品原封不动，N1既有原始FAIL不变，physical NOT_RUN。

后续顺序：定向first-attempt witness+不同/尾丢验证 -> 阶段切档不影响fresh或RX迟到 -> Actions one-case each performance；不重复N1夸大合格，不扩展无关工况。
