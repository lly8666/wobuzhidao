# N2 authenticated first-attempt bound source9b19635 scoped Actions PASS

日期2026-10-11，工作分支 `next/adaptive-fec-aes-tun-20261010`，程序SOURCE `9b196358b464198a325acf2e9c802113757935d2`，该收据提交仅更改docs/STATUS、正式PLAN、devlog及证据JSON。

## 目标和真实证明
原N1认证回声的成熟缺失=final residual而不是原始首发物理丢包。不能区分同Seq同wire首次/修复，所以发送端记录每个source report涵盖的DATA PN是否至少尝试修复一次。每PN重复repair仅计1；在实际 Emit 前登记（包括repairReserve），防止对端提前收到导致少计。联合严格匹配的认证mature V4: M=TxSuccess-ReceiverUnique, R=distinct original data PNs repaired; M<=first attempt lost<=min(TxSuccess,M+R)。因此是可信上下界而非精确测量；到期、报告重叠覆盖失败、localDrops、容量无证据一律不资格。per fresh仅增加shadow pendingRecord.PN，per repair最多16个源report位图检查，内存有界，wire和现有FEC/retrans/业务发送不变。

## 原始失败与定向修复
首轮SOURCE `aa09bb3f249b2c67c673ff3edc2070a01fc6847c`，Actions lifecycle [38121946729](https://github.com/lly8666/wobuzhidao/actions/runs/38121946729) / N1codec-race [38121946681](https://github.com/lly8666/wobuzhidao/actions/runs/38121946681)均FAIL：新增单元期待尾包缺失1且repair0，但真实runtime Tick已尝试同wire repair一次，源报告完整记录该修复，所以数理正确结果是首次可能缺1..2条，预期上界10%而非精确5%。修复SOURCE `9b196358b464198a325acf2e9c802113757935d2`只改测试oracle，未调整产品算法或掩盖红色run。首轮samewire[38121946636](https://github.com/lly8666/wobuzhidao/actions/runs/38121946636)PASS。

## 验证
- [foundation38122076600](https://github.com/lly8666/wobuzhidao/actions/runs/38122076600): 7/7 enabled PASS，仓库规则、Linux+Windows go test/build与Linux race/fuzz、OpenWrt真实TPROXY与linux shared-TUN nft/iptables（不等于native Wintun）；旧P5/P6 skipped不是PASS。
- [lifecycle38122076496](https://github.com/lly8666/wobuzhidao/actions/runs/38122076496) PASS。
- [N1 codec/echo race38122076533](https://github.com/lly8666/wobuzhidao/actions/runs/38122076533) PASS。
- [N0 native three-client38122076520](https://github.com/lly8666/wobuzhidao/actions/runs/38122076520) PASS。
- 每个真正性能run仅一测量SOURCE规则保持，本次没有性能run，没有USER物理试跑。

## 收口
N2只读可信首发损伤上下界已通过**scoped**Actions，不等于确切物理PLR；高修复场景上界可能宽到100%，不可用0%残余假装WAN0%。混合partial-k尚需验证，实际TX从新block改档、旧block/裸LINK RX混合、rotation等尚未开始；FECAuto/FECAggressive仍被正式admission明确拒绝。N2 IN_PROGRESS。N3-N6 NOT_STARTED;80秒S2C、历史probe及N1 off-v2 MEASURED_NOT_QUALIFIED保留；physical NOT_RUN。下一个开发agent遵循STATUS.next_task。
