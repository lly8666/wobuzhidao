# Windows PowerShell本地化状态读取修正

## 本轮目标和阶段

P7 diagnostic，起始6cead8c。产品9211b24不变；D04已留功能通过/质量PARTIAL证据。M03 seed1418仅准备失败，没有开始UDP负载，不计入300s样本。

## 修改与原因

WindowsPowerShell5读取无BOM UTF8的PS源码时按本机ANSI解释，原中文“没有运行/未运行/无”字符串变成乱码且regex无效。把这些literal改成ASCII源码的显式Unicode码点拼接，English/CN识别语义相同；不改变capture filter、component、ownership或容量。Actions dry fixture新增真实中文状态/无filter的码点输出，覆盖已在实机发现的本地化边界。

## 复用来源

本轮Pktmon helper，无old提取。

## Actions证据

SOURCE5c94366 predelivery37409438238 fourjobs/core37409438233/GUI37409438245 PASS，旧PSfixtureEnglish通过不能证明中文原生状态；本修正NOT_RUN，提交后先资格再实机。Go及已部署二进制未变。

## 问题、排查与风险

1418在monitor preflight invalidregex停止，没创建Pktmon capture/filter，没启动M03 workload，不能作为UDP缺包新样本。已有client/echo target是owned准备资源，controller finally正常停止/恢复；结束状态需要核验。D04上行32missing/raw187drop、M03旧缺失/迟到与最新full70/strict18/1800s未关闭。

## 下一项原子任务

验本helper中文fixture和core门，再独立seed1419有界reverse-fragment诊断。pktmon已启动时必须验证自有文件再stop，raw审计后删除；如果adapter/coverage不支持报告真实限制，不盲改产品。
