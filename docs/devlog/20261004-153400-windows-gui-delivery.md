# 20261004-153400 Windows 中文GUI便携包交付

## 本轮目标和阶段

用户Windows中文GUI/完整配置/服务器切换/portable请求，用户补充接受Wintun任意安装方式。分支next/tlslike-dataplane，起始及测试SOURCE `9857bdb25115e87521a9d62309d3ec0b67988f16`。本轮只收口文档、记录已完成Actions与固定预发布，不再修改产品代码。

## 修改与原因

STATUS.windows_gui关闭为ACTIONS_PASS_DELIVERED；README/WINDOWS_GUI/ROADMAP/DEVELOPMENT_PLAN/AGENTS/PREDELIVERY/ACCEPTANCE同步交付入口与范围，新增机器evidence。全部Windows参数和managed操作由真实PARAMETERS映射，GUI只是既有Go进程配置/生命周期壳，未新增协议或稳态算法。应用输出固定程序目录；用户接受Npcap/Wintun驱动例外，Npcap安装仍走官方引导。固定包与文档HEAD区别明确，下一步由用户安排物理P7。

## 复用来源

本轮无源码复用。前几轮直接调用当前正式CLI/TLS/FakeTCP/lifecycle与Windows网络脚本，详见同日Windows GUI实现日志与MODULE_MAP；未导入old/或恢复DTLS。

## Actions证据

SOURCE9857bdb所有以下run原始attempt1 PASS。GUI [37185477501](https://github.com/lly8666/wobuzhidao/actions/runs/37185477501)：206项真实GUI控件/正式Go有效配置检查，29字段、全28FEC/lane功能组合；模拟session切换/清理隔离、1500owned路线mock和恢复互斥、地址表更新、路径与截图。截图读取确认中文布局/密码遮蔽。foundation [37185477537](https://github.com/lly8666/wobuzhidao/actions/runs/37185477537)：7active PASS，9历史扩展SKIPPED，Windows/Linux unit/build和Linux race/fuzz及平台专项。其它同源网络/分流/lifecycle/padding/steady run列表见evidence/windows-gui-9857bdb.json。

portable artifact11297076998外层SHA256 `c49a1489d497588076a937b514cb15a08f45b8045c3fd4129ec93041f468eb97`；GUI artifact11296877418外层SHA256 `b9675ce03825e31376d5e8db2d3765f98617fdadc7609c637bb44ac4fee2135c`。只读下载后核对外层ZIP、内层包与manifest13文件。内层包3,969,589 bytes，SHA256 `4bd0884fc4e1e8d8e43fb889f702030542847804b2a2d10bfddeff9d4b2b5891`。

控制源bead7f6的delivery [37185800359](https://github.com/lly8666/wobuzhidao/actions/runs/37185800359)/job111387312744 PASS，独立校验精确985源码原始GUI/core成功回执与artifact/ZIP哈希后发布：[windows-gui-rc-20261004-9857bdb](https://github.com/lly8666/wobuzhidao/releases/tag/windows-gui-rc-20261004-9857bdb)。tag固定985，随后本文件所在docs-only提交不改变包来源。本机只编辑/读取/Git及产物哈希核验，未执行编译、测试或应用。

## 问题、排查与风险

第一轮37184506486失败来自测试TunnelID34hex及PowerShell5中文路径；第二轮37184758075失败来自秘密断言短串与公开TunnelID巧合，已修夹具，未降低产品校验门。此前SOURCE的204项与更新专项PASS仅是历史；本轮206项来自最终985。真实Npcap/Wintun安装、UAC、NIC抓包与跨服务器业务仍NOT_RUN；本轮没有性能样本，d9历史性能与full70/strict18/1800s/36生命周期不继承到985。Wintun注册Windows驱动写系统为用户明确接受的例外，portable只约束应用自身文件。

## 下一项原子任务

用户安排P7真实Windows安装/出口/分流/DNS/停止恢复和跨服务器业务验收；若要求新源码完整性能/弱网资格则独立一Action一条补齐。没有新缺陷证据不再修改FEC/4096/recovery架构。进度以STATUS唯一入口为准。
