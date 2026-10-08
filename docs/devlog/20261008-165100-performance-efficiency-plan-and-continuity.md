# 20261008 性能优化方案、MTU集成及agent入口整理

## 本轮目标和阶段

用户要求在新分支整理逐步CPU/系统效率优化方案与真实Actions测试，并减少新agent继承旧提示词产生负向优化。起点主线82c3c614a824b9b033901077aff26127bb6044cc，新工作树/分支next/performance-efficiency-20261008。仅方案与接手整理；纳入用户已授权其他agent完成的MTU，不在本轮实现新性能优化。

## 修改与原因

- 合并MTU父c480ccef30962358b33da936d9ba19def6061f70，其产品改动为68cd1a4：automatic record caps与配置派生TUN，不回退保留大包的逻辑边界。与主线共同祖先b4ea061；只有STATUS冲突，保留主线最新实机FAIL和全部MTU分支事实。
- PERFORMANCE_EFFICIENCY_PLAN列E0基线/真实负载、E1deadline、E2内存所有权、E3就绪IO、E4有限repair、E5剩余热点、E6冻结资格、E7后置失活；每步core/race+四独立保护场景、完整真实大小TCP/UDP混合、资源分层和容量阶梯。
- 重写AGENTS/README/ROADMAP/DEVELOPMENT_PLAN当前入口，ACCEPTANCE保留正式P1..P7硬门、删除旧日期“下一步”的当前暗示，新增AGENT_CONTINUITY历史导航和防退化清单。STATUS从巨大历史堆叠整理成当前索引，完整之前状态/入口和MTU父状态保存docs/history，旧devlog/evidence原路径保留。
- 更新章程的当前用户取舍，允许有界内存换CPU，高丢包大UDP不统一全收齐，正常业务/完整性/无HOL/资源门不降低。PARAMETERS/物理方案标当前派生MTU与旧9000历史界限。
- 约80秒下行明确OPEN_DEFERRED_UNTIL_AFTER_E6，另1.23秒迟到保留OPEN；不能用优化或MTU改动标修复。生成可复制agent任务模板。

## 复用来源

新MTU来自work/mtu-fec2020-matrix-20261008/c480cce与68cd1a4，没有复制old源码；历史账本来自主线82c3c61。agent/large-mtu-mixed-20261008仅只读发现与后续helper复用入口，不在本轮合入其负载测试或重派样本。

## Actions证据

- MTU父37747140769，exactc480cce，Jobs API39/39 SUCCESS；scope为core及kernel/FEC分层功能，不是300s吞吐/CPU/p99。既有失败37746842830/37746929808及devlog保留，未把失败删除。
- 本轮集成SOURCE基础Actions在push后核验，当前PENDING。没有在开发机运行Go编译/unit/race或性能。
- 新普通性能、CPU收益和本集成SOURCE物理均NOT_RUN。b4 NoHOL Actions用户已接受，但本集成仍需相关core/race回归。b4五条物理和旧11配对RTT等FAIL未被覆盖。

## 问题、排查与风险

正式partial到期8ms与入口100ms检查不等价，部分测试2/10ms不能代表正式时序；方案要求先普通入口测量。CPU宿主波动、不同架构及生成器自身成本分离，不能用一好一坏runner作收益证据。

MTU功能矩阵未覆盖完整WBD/TUN/TPROXY巨大UDP、PMTU或物理；platformflow8936限制与已有longmix失败需要E0核实，不宣称已支持全路径65507。正常TUN IP片路径与平台UDP代理路径分别验。新SOURCE完整配置70/strict18/1800s/P6/物理门仍待验。

用户要求优化之后再解决80秒，不代表失败可忽略。E6若被失活阻塞，保留FAIL和收益限制，进入E7；不通过调门、缓存膨胀、可靠排序或缩期限凑PASS。

## 下一项原子任务

新agent从STATUS的E0开工：核exact集成SOURCE与Actions/core/race，审现有longmixhelper/失败，资格化byte-budgeted正式进程业务，建立普通off Normal/Game基线和热点账本。只有之后才实施E1，不直接上所有优化。每性能Action一条。原聊天不启动物理负载。
