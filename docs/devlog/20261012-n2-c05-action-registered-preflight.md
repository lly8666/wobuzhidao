# 2026-10-12 C05 非对称线路单性能 Action 已启动，尚未有实测结论

唯一当前性能SOURCE `4aaeaef984f60229eada301bb048bbca5a2b41f4`，[Action38241338957](https://github.com/lly8666/wobuzhidao/actions/runs/38241338957) / job114784421949，固定Normal auto initial20/floor4/max20、ChaCha、MTU1400、300ms每方向、每向10Mbps，C2S路由`rsrv`固定netem1%、S2C路由`rcli`固定10%，两向各一段持续240秒+10秒drain，诊断ON。Linux tc随机loss seed不能设置，实际注入需在真实产物qdisc相应设备确认；业务seeds2026092201/2026092202。模型`n2-asymmetric-iid-timeband-v1`及SHA256 `7907824169a16565ca705bb4d10c72de377c164f56de232e1e51181680ecaec8` 预冻结，C05 末60秒目标C2S R4..R8、S2C R8..R10，期望各>=90%。仅一SOURCE/config/scenario/measurement job。

截至此登记，GitHub performance policy/sample claim通过，Go controller unit/race仍在执行，Python无流量预检/正式构建与真实样本结果**尚不能宣布通过**。新工作流按顺序还要验证派生五netns脚本两个设备方向、原正式client/server、相同source下唯一双向业务，然后严格分析源端PN与FEC parities、业务首次唯一收达/连续断流/p99、实际网损、qdisc队列/捕获drop、CPU/ISA/quota/PSI和总wire。若有fail，完整保存原始Action和小artifact SHA，不能盲重跑挑seed。无物理机器参与。

既有 C04 SOURCE0e3daca5/Action38231341392原始功能保护与最终off业务内均PASS，但S2C recover5 last60 78.3%低于工程screen90%导致Action FAIL；C03新SOURCE272deba9恢复5%上行95%、下行100% scoped PASS；C05/C06数学-only Action38232373616 successful并未进行真实测试。上述区别不覆写。本分支N2整体OPEN，N3–N6尚未开始、physical NOT_RUN，旧2.73s probe多秒尾及80s S2C interruption OPEN，禁止自动部署/qualification-ref迁移或合并主线。
