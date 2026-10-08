# 首轮剩余损伤条件单样本Actions逐条申请（每run只一场景）

统一冻结产品SOURCE b4ea061178a6e09b7e7c8587d72b4b8535492567；助手可验证基础SHA `31ac81d2a7904da177a888b61389c16c149c5aa3`，https://github.com/lly8666/wobuzhidao/actions/runs/37733698703 功能与策略preflight PASS。每个独立性能子run只固定一个SOURCE/config/seed/scenario；controller只请求一次，绝不单run A/B或B/A，不能从controller请求推断已经测量。每个后续请求以GitHub commit+同一STATUS/evidence为准追踪；Action原始运行结果后补。不改变冻结qualification ref或其它agent分支，物理NOT_RUN。

- 本次请求A/5%、Normal1 FEC20:20、真实大小UDP9档，双向各目标10Mbps、300ms单向，双向loss5%，300秒+10s drain、seed2608101，与A/0配对。A/0官方 https://github.com/lly8666/wobuzhidao/actions/runs/37731062203 已是全链路FAIL：Linux TPROXY 8936B界限导致8972/8973/65507所有丢失；弱损伤下必须继续把这些legal UDP计入正常损失分母，不以FEC数学预期合理化0%边界。并观察96B独立probe、有效小包、netem实损率/有界资源、大小包对小包的交叉影响。
- B/0重复 https://github.com/lly8666/wobuzhidao/actions/runs/37733833093 与C/0 https://github.com/lly8666/wobuzhidao/actions/runs/37733929493 当前分别独立进行，未写结论；后续损伤条件均保持单独Action。原生M03/1554 80s下行失活及恢复后1.225s UDP最大大包延迟仍各自OPEN。

- A/5%正式子Action [37734052485](https://github.com/lly8666/wobuzhidao/actions/runs/37734052485) 已单独启动，当前只记IN_PROGRESS，尚无数字结论。
- 本次另行请求A/20%，仍同一UDP九档/业务10Mbps每向/seed2608101/Normal1 FEC20:20/300ms单向/netem双向20%/300s+drain。一个新Action一份性能样本，真实netem必须实测；b4 Linux平台流8936硬边界保持已知OPEN，不能将此条件直接替A/0合格。

- A/20% Action [37734152474](https://github.com/lly8666/wobuzhidao/actions/runs/37734152474) 已独立启动，未报告数值前保持IN_PROGRESS；本次单独申请 A/30% exact b4 / Normal1 FEC20:20 / seed2608101 / 两向10Mbps，300ms one-way、两向30% netem、300秒业务+固定drain。不删除未回大包，需独立小包/容量证据与0/5/20配对。

- A/30% [37734257494](https://github.com/lly8666/wobuzhidao/actions/runs/37734257494) 独立正式Action已出现。现在另行请求B/5%、seed2608102、TCP真实双向3长/1Hz短、每方向固定10Mbps不重分配，300ms单程/双向netem5%/Normal1 FEC20:20/300秒+drain；实际kernel TCP MSS/retrans、注入不足、短请求缺失独立评估，原B0原始FAIL保留。

- B/5% 子Action [37734359638](https://github.com/lly8666/wobuzhidao/actions/runs/37734359638) 已创建，判定仍pending。本次B/20%单独请求Normal1/FEC20:20、真实全双向3长TCP与1Hz短TCP，seed2608102、每方向原定10M且不因为TCP背压减额，双向netem20%，300s+drain；检查MSS、TCP_INFO重传、流hash、短连接缺失、独立源是否达到99%注入。

- B/20% [37734464340](https://github.com/lly8666/wobuzhidao/actions/runs/37734464340) 已创建一条独立产品性能Action。现在请求单独B/30%，同源b4+同负载/seed2608102，双向固定30% netem/300ms单程、300秒业务+drain，不降目标10Mbps、不称TCP正常流内重传是外层跨HOL；每向有效source、flow hash、backpressure、实际接口drops/PSI均须审计。

## 两份无损新结果(失败保留原始)

- B/0修正短流后独立repeat https://github.com/lly8666/wobuzhidao/actions/runs/37733833093 原始**FAIL**、artifact11530149511 sha256:d84f7b272e2c5f46e17272d97afffe0a6aea99865e27ef7cb3f25bdbdb736660；短TCP并发助手已使原250/300改善为300/300、返回p99 1228.398ms，但3长TCP各方向仅212506656/212506560B，goodput5.66684Mbps/目标10，source coverage56.67%，stream SHA全部一致。零netem drop，无足够runner饱和证据，不能CAPACITY_LIMITED或PASS。原B0 FAIL仍保留。
- C/0独立正式 https://github.com/lly8666/wobuzhidao/actions/runs/37733929493 原始**FAIL**、artifact11530159636 sha256:7b0f1d95867aae8412020df53b7db81d985fcb52ec6ed6e43939478f98a14114。每向UDP offered185983529B、first valid约55.78MB=1.488Mbps/5M；8972/8973/65507三档全部0有效回包，另96/512/1372三档每个已送包完整。S2C 9142个payload CRC/头错误（恰是大UDP大小档数量），因既知b4 TPROXY MaxPayload8936截断；双向netem实drop0。每向真实TCP3条长连接hash逐条正确、goodput4.92688Mbps/5M目标约98.54%（严格99%未过），300/300短TCP返回p99约1230.845ms，独立96B UDP探针3000/3000 p99约627.108ms，最大UDP10ms无交付桶窗口300ms，能证明大UDP全损时其它独立TCP/小UDP还在交付，不表示原生Windows路径无HOL。总goodput每向6.4144Mbps/10M，FAIL不能隐去大包未返回而拿小包p99说通过。

## C有损矩阵

- B/30 Action [37734569538](https://github.com/lly8666/wobuzhidao/actions/runs/37734569538) 独立正式已启动。现在单独请求C/5%/seed2608103/sourceb4，TCP5M+UDP5M每向不挪配额、真实多TCP+多UDP同一份产品端点，0.3s每方向/双向netem5%/300s+drain，实测loss与Probe/bytes哈希需另审。不能将C0缺失的合法大UDP从C5 loss分母移走或归因30% FEC。
