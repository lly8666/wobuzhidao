# IPv4分流hosted验收

## 本轮目标和阶段
用户要求LAN/中国IPv4默认直连、内置地址表、显式手动更新，采用系统内核分流，保留TLS-like/no-HOL主数据面。产品分支next/tlslike-dataplane；固定SOURCE a67e10fa2875162eeac926b970a0c486a239748d。文档HEAD与实际二进制来源分开。

## 修改与原因
收口证据、参数使用、当前入口、候选包链接及未验边界。不再改产品代码。Linux nft interval set；Windows直连集合补集进入Wintun捕获，直连沿用原系统路由。表启动一次加载，默认不联网；显式更新校验后写文件，重启生效。data MIT来源和许可随包。

## 复用来源
v2rayN官方private/cn/direct规则仅语义参考；old/internal/windowsruntime/routing_policy.go定向源码参考已登记，无旧worker/crypto/协议引入。地址数据gaoyifan ip-lists c5f638a，固定SHA和MIT来源见SPLIT_ROUTING。

## Actions证据
foundation37154526472 PASS（Linux/Windows build+unit，Linux race+fuzz、ARM cross、真实fallback/TUN/TPROXY）；targeted37154526466 PASS。
四模式37154674002 PASS：真实UDP-DNS/TCP/102400B TLS1.3 HTTPS分别验证LAN/CN/foreign出口、默认/lan/all/manual、direct不唤醒、proxy唤醒和owned清理。内置模式还实际无凭据执行HTTPS手动更新。Windows大表Render/1500条owned Apply/Cleanup/失败回滚mock全部PASS，外部路由保留；真实驱动NOT_RUN。
独立5205 Normal37154675854/Game37154677946均PASS，control37154570449 PASS：600msRTT、FEC20:20、混合UDP，双向各10M/3M，stress byte loss0%、socketdrop0，p95约616.58/614.51ms。两独立run单样本，不同VM比较只用于退化门，不承诺CPU固定收益。Normal CPU88.4/90.92s vs旧90.71/92.70；Game105.37/98.92 vs旧67.84/62.89，后者升高的原因未证明，不能写CPU无退化。
生命周期37154675255：36/36+aggregate PASS；功能/生命周期/P6只读control37154570534 PASS，所有原始attempt1，extra dispatch0。
P6 37154676414三目标包PASS；ZIP digest、manifest hash及每文件大小/hash本地只读复核（不执行二进制）。amd64原生version在Actions，ARM仅cross。
证据docs/evidence/splitroute-a67e10f.json。旧2b的严格18/70配置/1800s不继承为本源码完整资格。

## 问题、排查与风险
最初b650 Windows嵌入数据CRLF/空DNS严格模式错误修正；真实direct夹具缺return route及重复TCP监听TIME_WAIT修正，保留全部原始失败。3d/275两次core在新增提交只修改既有日志未新增日志时被契约阻止；补新日志后a67通过，不降低检查门。
Windows真实FIB启动/退出成本、Npcap/Wintun、物理NIC、ARM原生留P7。IPv4地址分流，不含域名规则或IPv6直连。Windows显式dns4继续隧道优先。Game本次CPU升高记录为未归因观测，不能称固定CPU收益/无额外CPU。

## 下一项原子任务
先收齐生命周期回执并更新同一权威STATUS；后由用户安排P7实际平台验证。若要宣称新SOURCE完整交付前资格，再跑新SOURCE严格18/70/1800s，不能继承旧记录。不要无证据改FEC、4096、缓冲或协议。
