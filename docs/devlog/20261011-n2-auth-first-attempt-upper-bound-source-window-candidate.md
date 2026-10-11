# N2 source-correlated 首发损失上下界：代码与定向Actions候选

日期 2026-10-11，父源码 `a030dbfcd5336b745651ff8346dab7111c33dcdd`，分支 `next/adaptive-fec-aes-tun-20261010`。
本阶段复用已认证KindHealth V4回声，新增发送端**本地**每DATA PN至少一次修复记录。首发与修复复用完全相同TLS-like wire，接收端仅能算经过3s成熟后的剩余缺口m。若同PN至少修复过一次即可能由修复救回，首发真缺失在[m, m+repaired_unique]；多次repair只计一次，归属必须对照sender自身source window。未计算出完整bitmap、窗口已被新的DATA覆盖或出现本地失败就拒绝上界，绝不冒充zero WAN loss。

修改`pendingRecord.pn`保留已有record.PN；修复Emit两条路径中每次在发送前登记original PN，成功或失败都作为保守repair（避免跨线程接收的提前到达导致低估）；只有quality enabled和KindLINK记录增加O(16)的修复次扫描，正常首次DATA发送只有原有常数赋值。当前每source修复mask仅4个64bit word，16份历史共512B位图，无不受控内存。读取时source identity+10s age+sender TxSuccess window+receiver mature echo必须匹配，否则HOLD。

定向测试包括report前/后修复、同PN重试不重复、报告范围外修复忽略、窗口轮转丢失时拒绝、generation错误拒绝、完全尾部缺失和已有真实runtime N1 auth echo一包尾丢。所有Go编译/test/race只能由GitHub Actions运行，本地未执行。正式 FECAuto仍SupportedNow拒绝；N2动态FEC尚未交付，N1旧FAIL与物理NOT_RUN仍保留。
