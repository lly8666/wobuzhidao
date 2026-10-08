# 首轮剩余损伤条件单样本Actions逐条申请（每run只一场景）

统一冻结产品SOURCE b4ea061178a6e09b7e7c8587d72b4b8535492567；助手可验证基础SHA `31ac81d2a7904da177a888b61389c16c149c5aa3`，https://github.com/lly8666/wobuzhidao/actions/runs/37733698703 功能与策略preflight PASS。每个独立性能子run只固定一个SOURCE/config/seed/scenario；controller只请求一次，绝不单run A/B或B/A，不能从controller请求推断已经测量。每个后续请求以GitHub commit+同一STATUS/evidence为准追踪；Action原始运行结果后补。不改变冻结qualification ref或其它agent分支，物理NOT_RUN。

- 本次请求A/5%、Normal1 FEC20:20、真实大小UDP9档，双向各目标10Mbps、300ms单向，双向loss5%，300秒+10s drain、seed2608101，与A/0配对。A/0官方 https://github.com/lly8666/wobuzhidao/actions/runs/37731062203 已是全链路FAIL：Linux TPROXY 8936B界限导致8972/8973/65507所有丢失；弱损伤下必须继续把这些legal UDP计入正常损失分母，不以FEC数学预期合理化0%边界。并观察96B独立probe、有效小包、netem实损率/有界资源、大小包对小包的交叉影响。
- B/0重复 https://github.com/lly8666/wobuzhidao/actions/runs/37733833093 与C/0 https://github.com/lly8666/wobuzhidao/actions/runs/37733929493 当前分别独立进行，未写结论；后续损伤条件均保持单独Action。原生M03/1554 80s下行失活及恢复后1.225s UDP最大大包延迟仍各自OPEN。
