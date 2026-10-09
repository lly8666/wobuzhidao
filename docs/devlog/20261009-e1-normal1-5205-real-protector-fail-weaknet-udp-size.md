# E1真实Normal1 5/20/5弱网：应用UDP及探测正式FAIL，暂停Game2（2026-10-09）

只在工作分支 `next/performance-efficiency-20261008`，冻结产品SOURCE `622e5a3130ca96a3ca39b42ecae673dfefd5cf26`，helper commit `494a2caa005e228ab7b610e2cc47c697492fb158`。独立300秒[Normal1 staged5205 Action37907829434](https://github.com/lly8666/wobuzhidao/actions/runs/37907829434) **原始workflow FAIL**，原始summary/efficiency ledger均`FAIL`，7条业务层弱网失效issues见JSON evidence。此次[Foundation37907829715](https://github.com/lly8666/wobuzhidao/actions/runs/37907829715)全面SUCCESS，但功能单元绿不取代真实业务失败。原artifact11604943513 SHA256 `112d4494ce71930dc33c8187747e7b2e81639c3b993ac2bf398557bd65910b2d`，绝不覆盖、筛选或重跑择宿主。

真实netem确认三阶段、双向均达到约5%/20%/5%；第75-225秒20% stress阶段每方向发送172835个UDP应用报文，c2s缺失**424**、s2c缺失**460**（超过正式0.1%门，约0.245%/0.266%），整个300s c2s缺425、s2c缺463；按发送方单调时钟归属阶段，未见造假原loss。**报文大小相关性值得聚焦**：stress c2s 1372B缺206/17280、4068B缺187/8640，s2c 1372B缺157/17280、4068B缺263/8640，96B仅各6/86420与10/86420；这是相关性，尚不能判定FEC还是E1 timer根因。独立探测stress c2s缺14/750、s2c缺18/750；全程探测c2s15/1500、s2c19/1495（均必须计入p99之外的失败）。

TCP双向各304流，哈希/长度准确；HTTP/HTTPS20/20，HTTPS10/10证书验证；socket AF_PACKET skmem.d client0/server0，netdev extra drop0，strict_resource.errors[]。所以这是**应用层高弱网场景FAIL而不是本轮内核socket drop**。宿主Intel Xeon Platinum8573C，cpu.psi max31.77%，product CPU250.1s；前一无损PASS在AMD EPYC9V74、CPU242.37s，**不同host型号、不同网络条件，不能算CPU节约或者退化**。

目前Normal1 0loss该SOURCE一场Scoped PASS有效，但Normal1 5205正式FAIL且未达保护门，按用户“每项通过才下一项”暂停Game2 lossless/5205，lowRTT sparse仍NOT_RUN。**不立即推E4/FEC“优化”**：先只读检查已有fastblock/repair/重组逻辑和新E1到期parity timer怎样交互，针对1372/4068及20%丢包提出一个可用小型确定性测试证伪的假设；无因果证据不归罪E1。若证明真正源缺陷，一次原子修改＋core/race，再全套独立300s protector。旧9V45 profileOFF客户端33/server86及ON客户端42 AF_PACKET失败、既有Game2 4068B stress 1包丢失、E7约80秒下行OPEN_DEFERRED都原样保留。

本提交**仅状态/结构化证据/新增devlog**，不改产品Go、测试配置、队列、SO_RCVBUF、MTU、协议或规范主线，故不会触发下一条性能Action。
