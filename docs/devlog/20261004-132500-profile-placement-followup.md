# CPU采样输出位置修正
## 目标和修改
提交审阅发现采样脚本字符串替换也命中PID初始化段，显式profile时会在进程启动前读取不存在的文件。删除初始化段的重复采样输出，仅保留进程正常退出后的pprof。无实际业务算法改动，无本地测试。
## 来源与证据
无复用；ea49首次诊断尚未dispatch，不隐瞒本提交发现。正常profile off路径不执行该段，新固定SOURCE在Actions core通过后才跑一条profile Game5205。
## 风险和下一项
输出位置现经源码审阅，core/真实profile NOT_RUN。接下来DNS/IPv6默认策略和按真实热点窄修；不继承旧资格。
