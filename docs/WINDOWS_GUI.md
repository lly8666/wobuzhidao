## 2026-10-12 N3-D：在“传输”页接入新 record cipher 与自动 FEC

新版客户端 Linux/Windows `--record-cipher` / JSON `"record-cipher"` **默认 ChaCha20-Poly1305**。受保护 V3 能识别 `aes-128-gcm` 和 `aes-256-gcm`，但 **两个真实 AES 单场景五 netns 收发/CPU/资源证据未过前，WinForms GUI 的“记录加密套件”下拉只列 ChaCha**，不得将 Go 认证单元测试等同产品资格。这个字段位于“传输”，紧邻 Normal/Game、FEC 模式与固定/最小/起步保护，避免用户在 TLS 伪装、安全或网络路由页误配。新增字段在统一 `PARAMETERS.json` 中存在，与 `gui-fields.json` 完全等键，GUI 自测对每字段均经真实 Windows 客户端 `--check-config` 校验；不重复单独维护另一套 AES 选项。诊断状态仅显示已认证且新鲜的 RTT/损失估计/实际发送 FEC，未知保留 UNKNOWN。旧已保存的配置缺少该字段时使用默认 ChaCha，旧固定模式不自动迁移至 AES。物理 Npcap/Wintun 驱动安装仍未测试、不得宣告完成。N3/N4/N6 以 STATUS 的真实 Actions 为准。

# Windows 中文便携客户端

2026-10-05最新原生五分钟进展：固定6181db6未改产品；S01 Normal10与S02 Game4×3达到目标附近但C2S缺74/10包，不能写无损PASS；M01外层MTU1400大包至9000B无坏数据但有1次迟到。D01默认NRPT+10M出现约90秒双向中断、约30.12%业务loss和8/60 DNS失败，明确FAIL；整体P7仍PARTIAL。优先按STATUS.physical_5min复现D01并异常触发抓包定位最早边界，不直接归因DNS/VM或扩大buffer/FEC/4096。完整DNS互备、LAN/CN/IP/IPv6、其他配置/生命周期/弱网未跑。方案PHYSICAL_5MIN_ACCEPTANCE.md、日志devlog/20261005-021317-five-minute-native-capture-matrix.md、evidence/physical-5min-6181db6-20261005.json及压缩原始计数为当前证据入口；每性能Action只一条。原始pcap已删、退出owned清理通过，服务端保留active。

最新配套包固定SOURCE6181db66b67594b07cd989b8b8b5848cedf6ccc3，标签linux-server-rc-20261004-6181db6；新增自动地址与服务端重分配后重建使用上次active配置，GUI208检查通过。当前入口STATUS.windows_gui.latest_paired_release、STATUS.linux_server和evidence/linux-server-6181db6.json。早期985 GUI206是历史独立包，不含自动租约，不与最新server混用。2026-10-05已实测同源CLI的真实Npcap/Wintun及Windows vmxnet3→ARM业务、Normal/Game与owned退出清理，见STATUS.physical_native；这不替代GUI实际点击/UAC/跨服务器切换，后者仍NOT_RUN。整个P7仅PARTIAL，失败与压力计数见最新日志。

当前任务由用户要求插入，属于平台 UI 和便携打包，不重写已验数据面。实时状态和精确测试源码见 STATUS.json 的 windows_gui；旧 d9 默认网络和性能成绩不能代替新 GUI 资格。

## 使用

下载 `WBD-Windows-Portable.zip`，完整解压到一个可写的本地文件夹，例如 `D:\WBD`。右击 `WBD.exe` 以管理员身份运行（程序 manifest 自动请求 UAC）。Windows 10/11 x64 使用系统 .NET Framework 4.8，不安装额外 .NET、WebView2 或后台服务。不要在压缩包内直接运行，也不要将 data/logs 链接到其他目录。

服务器页填写服务端提供的 IPv4、端口、TLS 识别名称、用户名/密码、账户、路由密钥、Tunnel ID 和隧道 IPv4 /32。安装 ID 首次生成并保存；租约与 Tunnel ID 必须使用服务端分配值，不能随意自动生成。点击校验配置，再连接。校验调用真实 `wbd-client.exe --check-config`，不修改网络或安装驱动。

左侧支持新增、复制、删除、保存服务器；选择另一项后点击连接/切换。新配置先校验，旧进程完成清理退出，再启动新进程。不能把跨服务器切换当成同服务器 lane 的无损换代；应用连接可能中断。连接中的服务器不能删除，修改设置只在重连后生效。最小化进入托盘；关闭或托盘退出先断开并清理，不直接强杀。若清理超过 30 秒，保留原进程，后续重试；绝不并行抢接另一个客户端。

传输页包含 Normal/Game 1–4 lanes、所有固定 FEC 挡位、MTU、记录上限、本地起始端口和 `tls-startup-padding`。

### 当前分支 N2 Normal 自动 FEC 与运行质量（原生 Actions 功能限定通过）

在 Windows GUI 的 **「传输」页**，先选择 Normal 单 lane，再设置「FEC 模式」：默认 **兼容旧配置（legacy）**，继续解释旧「固定 FEC 冗余」；也支持 fixed / off / auto / auto-aggressive。普通自动选择「固定 FEC 冗余 = 关闭（0）」、最小冗余 R4（或其他允许档）和初始 R20，控制器随后由双方认证的首发质量反馈自动选 R 档。若要显式 **从 FEC R0 起步**，必须使用 Normal 单 lane、普通 auto、最低 R0、初始 R0；这会增加损伤早期缺包风险，不是默认配置。自动/激进自动不支持 Game 2–4 lanes；Game 只能固定档或关闭。所有组合先由正式 Go `--check-config` 硬校验，错误不会被界面悄悄更正。

已存在的 JSON 配置使用与 CLI **完全相同**的扁平参数键。例如普通自动默认强保护起步：

```json
{
  "lanes": 1,
  "fec-mode": "auto",
  "fec-parity": 0,
  "fec-min-parity": 4,
  "fec-start-parity": 20
}
```

这是需要与服务器认证配置合并的**示例片段**，不是可单独连接的完整 JSON；旧文件缺省 mode=legacy，不会自行迁移成 auto。传输页原有 `control-padding` 默认开、`control-padding-max` 默认0是低频**加密控制报文**填充，并非给每条业务追加padding。N3 AES-128/256-GCM 尚未完成验收，目前不在 GUI 中提供可用选项，不能用软件假回显。

「传输」页的只读 **运行质量（当前连接）** 每两秒通过现有 GUI-owned 客户端标准输出接收轻量统计：活跃已认证 transport SRTT、来自近期已合格认证反馈的**首发损失保守上界**、Normal 单 lane 正在用于新数据组的实际 TX FEC 冗余。首发上界不是物理 WAN 直接测量损失，不能当作丢包后最终业务丢失率。没有近期可信样本、连接断开或多 lane 值不唯一时应显示 **「未知」**，绝不能伪显示0%。默认诊断JSONL仍关闭，GUI不会为状态面板额外打开本地HTTP控制端口或保留业务正文。当前 [Windows 原生功能 Actions 38259485234](https://github.com/lly8666/wobuzhidao/actions/runs/38259485234) 已对代码配置与真实WinForms控件通过258项，但没有进行实机网卡/UAC/驱动的最终物理验收；权威资格仍依照最新 `docs/STATUS.json`。
网络页包含默认局域网/中国 IPv4 分流、DNS 接管/双解析器、额外直连 CIDR、自定义中国 IP 表导入/手动更新。生命周期页包含全部保活、失活、重连退避、业务闲置休眠和软年龄轮换配置。IPv6 仍默认捕获后丢弃，不能借 GUI 增加未经支持的 IPv6 传输或 recovery 模式。

## 文件与驱动边界

压缩包包含 GUI、静态 Go 客户端、网络脚本、官方原版 Wintun DLL/许可、参数清单和说明。运行产生的配置、IP 表、网络状态与临时文件在 `data/`，日志在 `logs/`。GUI以程序目录为根，不以调用者当前目录为根；所有子进程 TEMP/TMP 指向 `data/tmp/`；大路由 capture 文件位于网络状态文件同目录；Wintun DLL 从客户端 EXE 同目录加载。配置导入只读外部文件并复制自定义 IP 表；导出只写 `data/exports/`，用户自行复制。没有 AppData 配置、启动项、自动更新或独立 GUI 网络监听器。

`data/`、`logs/` 设置仅当前用户/管理员/SYSTEM可访问的 ACL；服务器密码/密钥保存在本地 JSON，日志隐藏这两项，导出会明确提示包含凭据。保留明文配置是为了整个文件夹可以迁移，不采用绑定机器的 DPAPI。不要上传整个 data/；迁移给他人前检查凭据。日志上限约 2 MiB × 2，界面最多400行，消息队列256条，定时最多32条，避免 GUI 持续日志导致无界增长。

**应用便携不等于驱动无需安装。** 现有网络入口依赖 Npcap + Wintun。Npcap 安装会修改 Windows 驱动目录。官方 Wintun DLL 首次调用 CreateAdapter 会安装/注册 Wintun 驱动和网卡，Windows 可能写 DriverStore/系统驱动目录；即使 DLL 位于本文件夹，也不能承诺只有 Npcap 写系统。这是保留现有网络架构的真实限制，GUI首次连接有明确驱动说明，不静默隐藏。2026-10-04用户明确接受Wintun任意方式，确认允许此系统驱动例外；Npcap+Wintun保留成熟网络入口。

Windows/CLR/杀毒软件自身的缓存、系统驱动注册及路由/DNS状态不属于应用解压文件，不用静态文件检查冒充整机零写入。首次连接自动按官方机制准备Wintun，非阻塞提示说明驱动注册，不再重复询问已授权的驱动方式。

## 新系统 Npcap 引导

便携与驱动页检测系统 `System32/Npcap/wpcap.dll` 可加载、`pcap_lib_version` 可调用及 `npcap` 服务存在。此检测不是物理 NIC 抓包资格；连接时仍执行现有 Npcap endpoint 实际打开和过滤。缺失时阻止连接并提供官方下载入口与重新检测。

用户在 [Npcap 官网](https://npcap.com/#download) 下载并运行官方签名安装器；不要安装旧 WinPcap，不需要无线监听支持。WBD直接使用Npcap目录DLL，不要求WinPcap兼容复制。免费版不允许随包分发，静默安装仅 OEM 支持，因此本包不带 Npcap 安装器，不下载/偷偷执行外部程序，不请求关闭签名校验。重启要求按官方安装器提示执行，再回到 GUI 重新检测。[官方安装说明](https://npcap.com/guide/npcap-users-guide.html)

Wintun 0.14.1 ZIP 固定官方 URL 和 SHA256；取原版 amd64 DLL 与 ZIP许可一起随包。只使用允许的 API，不修改 DLL、不从中提取内置驱动。[Wintun 原版二进制许可](https://git.zx2c4.com/wintun/plain/prebuilt-binaries-license.txt)

## 参数与开发接手

唯一产品参数源仍是 `docs/PARAMETERS.json` 和 Go flag 定义。`windows/gui/fields.json` 对每个 Windows flag 提供中文标签/帮助/分组，不另定义协议默认。GUI启动检查字段与真实参数全集相等；新增参数遗漏界面映射会失败。CLI默认表达式由清单读取，有限常量由正式值解析；新增表达式必须明确支持。client-record-limit方向与MTU含义保持原有语义。

普通业务字段均可编辑；config/version/update-china-ip/check-config/control-stdin是对应导入、关于、更新、校验、安全停止按钮的操作；state-path/network-script由便携路径管理，不开放指向系统文件夹的编辑框。这些不是漏掉的配置。`check-config`和`control-stdin`只能CLI设置，不能在服务器 JSON 中注入。

GUI与client stdin建立父子所有权：stop或GUI消失导致EOF，取消正在建连或已运行的client，执行现有defer owned-only清理。正常退出不强杀；显式配置检查只在一个无网络副作用的短进程里执行。client错误返回顶层main后才log.Fatal，保证已注册defer清理不会被中途os.Exit跳过。网络脚本Cleanup可仅凭state文件恢复，不再要求旧服务器/网关字段。

## Actions验收范围

`next-windows-gui`在Windows2022真实编译WinForms并生成可下载ZIP。不同CWD、中文空格路径启动；每个普通字段使用不同值，调用真实Go解析和验证，核对有效值；全7个FEC×4lanes组合；非法参数、重复/未知JSON、操作键注入拒绝；秘密不回显；真实控件保存、profile重开、模拟client切换顺序、清理失败不启动新client、缺Npcap阻断、路径越界拒绝、截图。

GUI进程生命周期用显式fake session验证UI顺序；真实stdin stop/EOF单测属于foundation。真实路由/DNS/IPv6脚本既有hosted mock再跑。Npcap下载/安装、实际Wintun创建、真实NIC收发、跨服务器真实业务及UAC操作是P7 `NOT_RUN`，不能因hosted绿色就宣布物理完成。文件逃逸检查只覆盖WBD应用输出，不声称穷举整机OS写入。性能测试仍每Action只跑一条；本工作流是功能/配置检查，没有带宽样本矩阵。

裸 Windows CLI 的 state-path/network-script 默认同样按 EXE 所在文件夹定位；GUI不依赖当前工作目录。异常后连接会在旧进程确认停止时尝试owned状态恢复；恢复按钮查询client同名event，另一个client仍运行/清理时拒绝，避免与迟退旧进程并发修改路由。物理依赖仍待P7。

## 本轮交付回执（2026-10-04）

[预发布与下载](https://github.com/lly8666/wobuzhidao/releases/tag/windows-gui-rc-20261004-9857bdb)，精确SOURCE `9857bdb25115e87521a9d62309d3ec0b67988f16`。ZIP 3,969,589 bytes，SHA256 `4bd0884fc4e1e8d8e43fb889f702030542847804b2a2d10bfddeff9d4b2b5891`；manifest内13个文件哈希逐一核对。解压到可写程序目录，再运行WBD.exe（管理员权限）。

[GUI Action](https://github.com/lly8666/wobuzhidao/actions/runs/37185477501)原始attempt1完成，206项PASS：29个普通配置字段保存后调用正式Go读取有效值、全部managed操作映射、7个FEC×4lane功能组合、错误输入/秘密隐藏、导入/持久化、模拟连接切换与退出、恢复互斥、更新地址表、路径限制和真实WinForms截图。1500条owned路由Apply/Cleanup/rollback在hosted mock PASS。截图经人工读取确认布局、中文与密码遮蔽。

[foundation](https://github.com/lly8666/wobuzhidao/actions/runs/37185477537)7个active jobs PASS（9个历史扩展job SKIPPED），Windows/Linux单测/构建、Linux race/fuzz和平台专项通过；同源默认网络、分流、targeted lifecycle、startup padding及steady targeted亦通过。完整机器证据见[evidence](evidence/windows-gui-9857bdb.json)，历史测试夹具失败保留在STATUS。

用户已接受Wintun系统驱动安装，原版DLL与许可随包；Npcap仍使用官方安装引导。应用便携不意味着Windows驱动注册不写系统。真实安装/UAC/NIC和真实跨服务器业务尚未验收。本轮没有吞吐/弱网性能样本，也未重跑full70/strict18/1800s/36生命周期，不把历史d9结果继承给985。文档HEAD和包来源独立记录。


## 2026-10-12 自适应 FEC 传输页与受控质量状态（Actions 限定）

可用参数、适用范围及互斥组合以同SOURCE客户端 `--check-config` 为准。新建Normal单lane可选择 `fec-mode=auto`、`fec-min-parity=4`、`fec-start-parity=20`，配置既有`fec-parity=0`；如要从无FEC启动，需显式 `auto/min0/start0`。老配置无 `fec-mode` 时仍然 `legacy` 固定档/off，不会因GUI迁移自动改为auto。Game2–4 lanes不允许auto。传输页给出模式、固定R、最低R、初始R及两个既有控制填充字段；质量显示的是受保护且满足样本资格的RTT和源首发丢失上界及当前TX R，未有可信新样本/离线均显示“未知”，不是物理WAN精确loss。

GitHub Windows native [Action38260818090](https://github.com/lly8666/wobuzhidao/actions/runs/38260818090)，源码 `78a2dee07c5d1d1ed847acedc898c3518d03f2b4`，在258项WinForms+Go配置自测中实际选择“传输”Tab、断开质量显示“未知”后生成截图；artifact11751524849 SHA256 `50f14a6077ba4c06cfb7cf0866e7626490c939d8c9a55cd45bca1a86c69faf1c`。这是Windows hosted GUI构建/自测通过，不是人工实体机/驱动联网试验。**AES-128/256仍未接线产品，GUI不显示未经支持的算法**；N3真实认证及加密lane通过后方可开放。
