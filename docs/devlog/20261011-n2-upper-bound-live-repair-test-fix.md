# N2 upper bound 首轮真实 Tick 已修复，修正错误测试oracle

父SOURCE `aa09bb3f249b2c67c673ff3edc2070a01fc6847c`，GitHub lifecycle run38121946729、N1codec/race run38121946681 失败，其他历史结果留痕不改变。
错误不是公式或生产丢包，而是测试在整个6s过程中忽略了实际真实runtime client.Tick引发的同wire repair：缺失尾包仍缺失1个，在对端首次缺失和修复后最终交付不可区分下，sender distinctRepair=1，此时只能断言首发丢包在[1,2]，lower5%、upper10%。原测试写成无repair upper5%与实际不符。仅调整该断言，记录上下界证据，产品重传/FEC/质量协议完全不改。后续GitHub Actions必要重新跑；不将此前红色改写绿。
