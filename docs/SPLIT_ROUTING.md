# IPv4地址分流

默认 `route-mode=bypass-lan-cn`：本机/局域网/链路本地/CGNAT/组播保留IPv4及中国IPv4直连，其余通过TLS-like隧道。另有`bypass-lan`与`all`。all仍保留服务器/本地地址必需绕行。Linux/OpenWrt仅处理当前路由器PREROUTING入口；Windows Wintun按内核路由捕获。IPv6边界保持原状（Windows fail-closed，OpenWrt IPv6未实现），不新增域名/geosite规则、DNS嗅探或IPv6直连。

采用v2rayN的private/cn/direct/剩余proxy规则语义参考：[官方路由例](https://github.com/2dust/v2rayN/blob/master/v2rayN/ServiceLib/Sample/custom_routing_white)。本实现没有搬入Xray/v2ray核心，也不复制其全部域名/广告/端口策略。旧项目只定向参考old/internal/windowsruntime/routing_policy.go。

## 数据与更新

内置数据来自[gaoyifan/china-operator-ip](https://github.com/gaoyifan/china-operator-ip)，ip-lists提交c5f638aa2f229909aafcbbd8f220bb48b00c4eaa，更新时间2026-10-03T08:40:43Z；文件SHA256为177c666b94dea7cfa7e5497a4b210680c220aee9ca865b22f58fadb0bda89660。来源记录internal/splitroute/DATA_SOURCE.json，MIT许可随DATA_LICENSE内置源保留。无自动下载/周期任务；离线启动可直接使用快照。IP库是分类数据，不保证实际网站位置或所有路由均最优。

手动更新（Linux/Windows客户端同参数）：

```
wbd-client --update-china-ip china-ipv4.txt
wbd-client --config client.json --china-ip-file china-ipv4.txt
```

第一条无需隧道凭据，下载固定HTTPS官方URL，30s期限/1MiB上限、限制重定向、严格IPv4 CIDR校验和空表拒绝，写同目录临时文件sync/close再rename。更新失败不破坏旧文件。第二条为正常启动，重启后应用快照；运行中不热更新内核路由或已存在业务流。也可用户自行准备CIDR文件；该文件受信任配置，不是通用URL下载器。客户端JSON同名键、CLI覆盖JSON。

## 低开销接线

启动时一次解析、区间排序合并/精确CIDR规范化。Linux/OpenWrt在原owned nft table中增加一个interval set，mandatory underlay/mark/local绕行之后、TCP/UDP TPROXY之前匹配直接return；OpenRuntime规范化不能丢Direct4；全快照验证在任何网络资源修改之前。退出整表清除，不碰外部规则。

Windows对直连地址及server /32取精确补集作为Wintun capture routes。直连地址没有WBD capture route，因此沿用原系统路由而不是强制走同一物理默认网关（LAN、VPN、回环等尤其重要）。现有用户direct4和显式DNS配置保持；dns4的/32 capture优先，是明确隧道DNS策略，不能和server IP重合。无需每包用户态查中国表、direct NAT/回环relay、加密或FEC；直连业务不唤醒DORMANT隧道。

大型Windowscapture快照使用有界临时文本文件而非超长argv；PowerShell一次查询现有capture路由，一次持久化新增ownership intents，再逐条New-NetRoute，避免每条CIM查询与每条全量state重写。仍需有限启动/退出路由安装工作及内核FIB内存；不能声称启动零成本或真实Windows驱动已验证。退出只删自己记录的route/address/NRPT/firewall，已有外部同前缀路由不认领。

## 验收

所有在Actions：Linux/Windows单位边界、内置文件hash、原子更新/替换失败保留、IP补集0/末端/重叠/随机分类、不支持模式/坏文件；Windows真实PowerShell大文件Render与owned Apply/Cleanup模拟验收，物理Npcap/Wintun仍NOT_RUN。Linux四个正式进程功能工况（embedded/lan/all/manual），三个可控目标地址：LAN10.50.0.2、CN223.5.5.5、foreign8.8.8.8；真实UDP-DNS/TCP/102400B内层TLS1.3 HTTPS，观察目标收到的源IP证明direct/proxy路径，双端DORMANT时direct业务不唤醒、代理业务重建；验证nft计数与退出清理。

旧生命周期/配置夹具必须显式route-mode all，因为其10.50私网目标原本用于代理模块验收；不可把默认直连误当隧道通过。严格5205目标改为隔离netns中的8.8.8.8，通过新默认策略真实代理；无外网发包，无速率/损伤门放宽。Normal10M/Game4逻辑3M、FEC20:20、600msRTT各独立Action单条，core通过后执行。旧2b候选的全矩阵/长测不能继承给新源码；本专项只关闭明确的新功能与targeted性能，物理与其它未验边界不变。
