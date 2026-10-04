# Windows 中文便携客户端

当前任务由用户要求插入，属于平台 UI 和便携打包，不重写已验数据面。实时状态和精确测试源码见 STATUS.json 的 windows_gui；旧 d9 默认网络和性能成绩不能代替新 GUI 资格。

## 使用

下载 `WBD-Windows-Portable.zip`，完整解压到一个可写的本地文件夹，例如 `D:\WBD`。右击 `WBD.exe` 以管理员身份运行（程序 manifest 自动请求 UAC）。Windows 10/11 x64 使用系统 .NET Framework 4.8，不安装额外 .NET、WebView2 或后台服务。不要在压缩包内直接运行，也不要将 data/logs 链接到其他目录。

服务器页填写服务端提供的 IPv4、端口、TLS 识别名称、用户名/密码、账户、路由密钥、Tunnel ID 和隧道 IPv4 /32。安装 ID 首次生成并保存；租约与 Tunnel ID 必须使用服务端分配值，不能随意自动生成。点击校验配置，再连接。校验调用真实 `wbd-client.exe --check-config`，不修改网络或安装驱动。

左侧支持新增、复制、删除、保存服务器；选择另一项后点击连接/切换。新配置先校验，旧进程完成清理退出，再启动新进程。不能把跨服务器切换当成同服务器 lane 的无损换代；应用连接可能中断。连接中的服务器不能删除，修改设置只在重连后生效。最小化进入托盘；关闭或托盘退出先断开并清理，不直接强杀。若清理超过 30 秒，保留原进程，后续重试；绝不并行抢接另一个客户端。

传输页包含 Normal/Game 1–4 lanes、所有固定 FEC 挡位、MTU、记录上限、本地起始端口和 `tls-startup-padding`。网络页包含默认局域网/中国 IPv4 分流、DNS 接管/双解析器、额外直连 CIDR、自定义中国 IP 表导入/手动更新。生命周期页包含全部保活、失活、重连退避、业务闲置休眠和软年龄轮换配置。IPv6 仍默认捕获后丢弃，不能借 GUI 增加未经支持的 IPv6 传输或 recovery 模式。

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
