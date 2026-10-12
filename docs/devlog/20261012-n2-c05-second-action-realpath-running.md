# 2026-10-12 C05 同模型不对称1%/10% 第二次Action已进入单一真实负载

最初 SOURCE`4aaeaef984f60229eada301bb048bbca5a2b41f4` [Action38241338957](https://github.com/lly8666/wobuzhidao/actions/runs/38241338957) job114784421949因inline模块旧名`adaptive_n2_c05_steady1`测前FAIL；Go/unit与2个正确性模拟均PASS，没有执行真实网损，artifact11742454692 ZIP SHA256`0381302d13293bda15506a85651ebfde982d7acde1c681cad80903fa96b351df`，在原FAIL evidence中保持，不改造为PASS。

纠正一个`.github/workflows/next-adaptive-n2-c05-asym1-10-single.yml` inline import名的SOURCE `758f310b005a26f2610d721f8877a8a13c5f521c`，新独立 [Action38241759581](https://github.com/lly8666/wobuzhidao/actions/runs/38241759581) job114785832142。已通过GitHub repo/performance policy/source claim、Go adaptivefec unit/race、两个新Python源方向冻结/grade parser unit及旧inline null lanes、five netns内核依赖、正式client/server编译与正式派生shell检查。**目前仅唯一240秒固定C2S1%/S2C10%活跃双向各10Mbps/300ms每向、Normal auto initial20/min4、MTU1400 ChaCha，随后10秒drain处于运行中，尚未完成/无产物hash。**完整冻结数学`n2-asymmetric-iid-timeband-v1`输入 SHA256`7907824169a16565ca705bb4d10c72de377c164f56de232e1e51181680ecaec8`不变；C2S last60允许R4..R8、S2C允许R8..R10，均>=90%。Linux tc RNG UNSEEDED且必须实际核rsrv1%/rcli10%的计数。下一次必须取run/job/artifact/summary/full netem/真实业务首次交付和资源后再论结果，不依据workflow Running宣布PASS。

如果C05失败先保留原样、缩窄原因，不随机重复；若C05限定合格才后续另发C06反向10%/1%独立Action，之后GE突发等，N2全面资格仍OPEN；N3–N6未启动，physical NOT_RUN，历史多秒late2.73s/80s S2C OPEN。不动qualification ref/主线、物理试用、部署。
