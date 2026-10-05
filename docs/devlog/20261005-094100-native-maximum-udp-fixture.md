# 20261005-094100 原生最大UDP/内层MTU夹具

## 本轮目标和阶段

产品候选冻结660b370815603c50d38b92229abdec13ca77e1b1在Actions验收；实机仍24ff220/S02 seed1354。为随后M01/M03补专用夹具，不改产品，不把helpers HEAD当产品包SOURCE。

## 修改与原因

native_mtu_probe.cs与native_windows_mtu_boundary.ps1覆盖原M01档位或M03 UDP8972/8973/65507/65508，两组DF，每大包后小包验证持续可用。每发送/拒绝最多10 attempts/s、全局8192请求metadata界限、300s+3s drain；逐字节校验、迟到归属原sequence且再次完整校验，不像旧夹具只记迟到而不校验。拒绝10040、other send/receive errors、timely/late、超时、进程仍活均分别保留。功能probe不冒充10M性能资格。

native_mtu_boundary_target.py在受控198.18.0.1:18446接收合法长度、记录真实peer lease、完整校验和echo；65508非法UDP不能到达target。禁止raw payload存盘；临时SO_RCVBUF请求与实际分别报告。predelivery Actions增加C#编译/PS解析/Python语法门，真实API/driver仍需原生。

## 复用来源

沿用当前physical_*标准payload模式/P7M1 echo与部署controller，未导入old。新增名字不覆盖已运行S02上传夹具；下一轮记录此新helper的精确hash。

## Actions证据

helpers当前NOT_TESTED，提交后next-predelivery-tools查精确HEAD。产品660b独立Normal/Game5205与P6另冻结，不因本次helper改动重打未关联的算法性能测试，也不继承977失败为部署资格。

## 问题、风险与下一项

API10040只能证明本地MTU/最大UDP反馈，不证明互联网PMTU。DF=false最大合法UDP需允许OS IPv4分片≤9000进入LINK，再由原IP栈重组，不截断。成功发送却仅超时记INCONCLUSIVE/FAIL，不把没退出当数据成功。先source660全部必需门通过、当前原生窗口结束才部署配套包；读取实际NlMtu9000和退出恢复，再M01/M03各独立300s。每性能Action一条。
