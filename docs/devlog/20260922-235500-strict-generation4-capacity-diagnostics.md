# 20260922-235500 strict generation4 失败收口与容量诊断

## generation4 identity

SOURCE_SHA:
`7110fd1e22cb2ab3809456657e3fcdf139508a65`

Run:
https://github.com/lly8666/wobuzhidao/actions/runs/35742321744

Aggregate:
- artifact ID: `10701220460`
- zip sha256: `7ccc1ccae9a9b68d27bcebb01e93c4511a8bfbdc3acea8cf3591e0553b223bef`
- sample_count=18
- missing=[]
- result=FAIL
- Normal/Game × lossless/5205/5305 六组全部未关闭

同SHA基础：
- foundation https://github.com/lly8666/wobuzhidao/actions/runs/35742321758 — PASS
- targeted https://github.com/lly8666/wobuzhidao/actions/runs/35742321944 — PASS
- 历史P5/P6/31m低负载soak均skipped

Harness/analyzer parent blobs：
- strict sample `af0f5c183df2797e931d30afc243f935fcb1d5ae`
- active analyzer `db827e12dd4a12b4d1c129c2812b8f6a4552536d`

## generation4真实结论

修正后的qdisc丢失分母与方向恢复账本已经在本轮实际使用，不再沿用generation3错误解释。

共同模式：
- CORRECTNESS：18/18保持PASS模式；未见应用坏payload/重复、同Seq不同密文、意外分片等硬错。
- CAPTURE：18/18保持PASS模式。
- ENVIRONMENT：18/18目标负载均出现产品侧UDP skmem drop。
- PERFORMANCE：18/18未达目标，CAPACITY_LIMITED。
- 无损本身已失败，因此损伤场景没有合法RTT baseline，aggregate未做虚假RTT比较。

代表样本：
1. Normal lossless seed101，artifact `10699878071`, zip sha256 `bd809884a264871038526f7553aa0509f2452484d586e4a12d1828b6364c7ed7`：
   - 注入0%，send约10Mbps/向；
   - C2S pre/stress/post goodput 0.413/0.268/0.263Mbps；
   - S2C 5.484/5.105/5.098Mbps；
   - client/server UDP skmem drop 78022/140804；
   - queue pre p95 9.71s；
   - client/server process CPU 129.03/113.22s；
   - C2S outer约36.7-37.2Mbps，outer/app raw 3.68x；repair近0。
2. Normal 5305 seed101，artifact `10700825862`, sha256 `af659e9d873ecab548225d2b247c59125d24dc547d2663c494bd8b3e1e1695a0`：
   - C2S netem 5.131/29.833/5.081%，S2C 5.054/29.981/4.938%；
   - C2S stress goodput 0.196/10Mbps，S2C 3.257/10Mbps；
   - client/server socket drop 83472/141738；
   - S2C stress receiver FEC recovered_sources约10060，但最终业务packet loss仍约64.1%。
3. Game 5205 seed202，artifact `10701025619`, sha256 `53229093a5156e594877fd8c4c950ee69c70d8f25721221e1009c0c6d527593c`：
   - corrected netem C2S 5.000/20.023/4.973%，S2C 5.023/20.043/4.935%；
   - C2S stress 0.184/3Mbps，S2C 2.266/3Mbps；
   - client/server socket drop 24837/20777；
   - queue pre p95 26.89s；
   - C2S/S2C outer约45-48Mbps/向；
   - outer/app raw约15.2/15.5x；
   - repair outer share约0.5-0.6%。
4. Game 5305 seed202，artifact `10700436535`, sha256 `cfe0a96c632ceb702f637b7758f9aadd089c6924317f918311eff9807028965b`：
   - C2S 5.0/29.97/4.97%，S2C 5.03/29.94/4.96%；
   - C2S stress 0.197/3Mbps，S2C 2.316/3Mbps；
   - S2C stress receiver FEC recovered_sources约54442，但最终业务packet loss仍约22.56%；
   - repair outer share仍约0.5%。

解释：FEC确实救回大量shard，但本机socket/产品queue先失守，恢复收益不能直接等于最终业务收益。Game线上约15-16x采用新的app raw分母，不能和旧稀疏HTTPS attempted-inner-TLS 7.3-7.5x直接横比。当前没有证据表明repair storm是主字节来源；Game复制与FEC source/parity/partial/header是主要成本。

## 原始18 artifact receipts

- game-5205-101: 10701125185 / `71812907167f4c4323de1e8a516e7fc1745a736192d335ce92bc174e0eb2ff43`
- normal-5205-202: 10701110142 / `fde32a3af8f86e2cde6e252a26e5583b6b32dc1c23d4cbede838c20a16f00c3c`
- normal-5205-101: 10701085130 / `b7993e81c5fde03b19865b6a48bfaff1598b76cdfe9ac03c7f4cae593672c441`
- game-lossless-101: 10701065303 / `0f1a8fb9093d62b6bbf31792ebdedf8625c36104ad94d8b76725d8efbe8aaa7b`
- normal-5205-303: 10701030375 / `95c544e7b903f315547023b1dc02a3cc910bc3180e0ff550272418d901c107a7`
- game-5205-202: 10701025619 / `53229093a5156e594877fd8c4c950ee69c70d8f25721221e1009c0c6d527593c`
- normal-5305-101: 10700825862 / `af659e9d873ecab548225d2b247c59125d24dc547d2663c494bd8b3e1e1695a0`
- normal-lossless-303: 10700456967 / `fc3c4513973c2a6bbbb65df6095fdbdf8c0d30677617e56ee83eaec1d4e0e4b9`
- game-5305-202: 10700436535 / `cfe0a96c632ceb702f637b7758f9aadd089c6924317f918311eff9807028965b`
- game-5205-303: 10700362244 / `b82fbe43a188821234b8eb5739dd164a9f5bb31e46c29f20427f6ddba4a66319`
- normal-5305-202: 10700352132 / `1fc4134ce6a4a82d2ef9a4da2ae3a81476ae454b566d58729f5f4173b442adc2`
- game-lossless-303: 10700247624 / `0a3d5e0094914fc2d95442748a6dd7c070acdcb450efb0774840884b155daef6`
- game-lossless-202: 10700237579 / `f191bd6195afeba23cd5cca8d5979ae6122c88a70c475bcd6433c0c4af8d4869`
- normal-5305-303: 10700102505 / `14afe02776b39057c9d6073a6cd99efaaaf99e225b8c625be85d63af137c8cb0`
- normal-lossless-202: 10700017682 / `f3dc3053a7117a0c253a1a8dfa2b4b88de2ef836233f0a0ccaca1331646bf5a7`
- normal-lossless-101: 10699878071 / `bd809884a264871038526f7553aa0509f2452484d586e4a12d1828b6364c7ed7`
- game-5305-303: 10699853154 / `a525ebfd2d8ddd1b67cc15c14f7404bbacb2905e9c7fc5f9f3948fe015d68f21`
- game-5305-101: 10699748506 / `53bf00f453d909cfd7b7ad4296caaf8b4b83b5e3437201a9bdefa6012ffc4073`

## 下一步：容量诊断而非放宽资格

本提交：
1. 将 `next-strict-weaknet` 恢复为workflow_dispatch-only，避免诊断提交自动重跑18个已知失败主样本。
2. 正式产品半速：
   - Normal 1lane 5Mbps/方向；
   - Game 4lane 1.5Mbps/方向；
   - 仍FEC20:20/padding off/MTU1400/300ms/120s/10s drain；
   - 只用于定位，仍用原strict analyzer；成功不替代10/3Mbps资格。
3. 同拓扑旁路：
   - 保留5个netns、同veth、MTU1400、router两向300ms netem、qdisc limit200000、四点tcpdump、同UDP发生器/资源采样；
   - client/server netns只做IPv4 forwarding，不启动WBD产品；
   - 50Mbps/方向、60s+10s drain；
   - 要求发生器99-101%、无内容错/重复/final loss、qdisc/interface/socket/capture drop=0。

若旁路50 PASS而产品半速仍drop，优先定位正式入口读取/调度/队列；若旁路本身FAIL，则先定位Actions发生器/veth/netem/capture容量。无论哪种，均不直接扩大socket/FEC/4096缓存。
