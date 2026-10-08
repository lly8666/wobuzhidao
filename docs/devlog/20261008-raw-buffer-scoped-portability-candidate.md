# 程序内 raw 缓冲平台补偿候选与 c853 实机启动

用户2026-10-08授权：审计新版，系统设置阻碍raw缓冲时直接在程序处理，适配平台，并物理复测；p99需要结合FEC理论能力判断。新用户授权覆盖上轮提示词中“不用SO_RCVBUFFORCE”的历史实验约束，但不授权修改全局sysctl/增加权限或破坏项目主旨。

独立核验固定c853935e5356a9bc99d380befb5a8ac8e0c08d97的14条exact-source Actions run/job、P6三平台manifest全部size/hash、六份原始小摘要的probe coverage/p99，全部对应。三平台包已配套部署Windows→ARM，保留旧3a程序和配置回滚。受控串行1541 Normal请求0、1542请求524288、M03最大UDP1543/1544、Game4轮换1545进行中，每条300秒，使用同源qualified助手。原生未通过前不记PHYSICAL_PASS，实际队列/读取/CPU/PSI/p99和退出owned清理照旧保留。

ARM系统rmem_max/default仍212992。一次短生命周期AF_UNIX内核选项探针：普通请求524288读回425984，只有该探针fd的SO_RCVBUFFORCE读回1048576，errno0，前后全局值相同。该探针只证明现有权限下内核选项可用，不是产品实效/性能测试。服务已有cap_net_admin/cap_net_raw，不增加capabilities。

新候选在Linux raw接收fd初始化中，先普通设置和读回；只在limited时尝试一次SO_RCVBUFFORCE，再读回。使用已有CAP_NET_ADMIN；失败/旧内核不支持保留普通有效值和limited，增加force_attempted/forced/force_error诊断。0仍不调用设置；Windows路径完全不引入该选项；OpenWrt等Linux走同一实现并诚实降级。稳态不增加任何系统调用、分配、日志或锁。设置的仍是用户声明且原验证有界的请求值，不扩FEC/4096，不改MTU、RTO或wire。

新增确定性初始化分支测试覆盖inherit、足够、权限补偿、权限拒绝、选项不支持、强制读回失败；Linux server privileged门增加真实AF_PACKET读回、既有BPF和系统默认不变测试。CLI/JSON参数名、默认和范围不改，help/参数清单/部署说明同步。当前候选Actions NOT_RUN，不能部署未验源码；物理正在测的是独立固定c853，不混候选。

FEC理论只作条件参考：独立随机p=30%、完整k20/r12时理想指定源残余q约5.7724%，k20/r20约0.130108%；partial k=r=1则9%。实际k/r、partial比例、碎片共同成功、丢失相关性及三秒期限必须从账本判断，不能用名义档位解释全部p99；56/60 probe返回时返回项p99约2.18s不描述所有请求。r12尾部仍OPEN，不通过增加重传强度/排序/恢复期限造零loss。

依据Linux socket(7)：SO_RCVBUFFORCE依赖CAP_NET_ADMIN，只覆盖该socket的rmem_max约束，https://man7.org/linux/man-pages/man7/socket.7.html 。所有产品测试/编译继续Actions，每性能run单一样本，profile默认off。精确SOURCE和每轮资格继续STATUS/devlog/evidence留痕。
