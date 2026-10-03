# 20261004-050900 分流候选包来源与许可

## 本轮目标和阶段

新功能在3d4935b已触发core/真实分流资格，P6新包需包含内置地址数据的MIT来源/许可和实际使用说明。本轮包构建器改动不改变数据协议。

## 修改与原因

跟进提交增加四模式功能只读aggregate：必须齐备四个精确SOURCE原始PASS，供固定ref控制收口，不执行任何性能负载。

所有P6目标加入四个非二进制文件：china-ipv4.txt、china-ipv4-source.json、china-ipv4-LICENSE.txt、SPLIT_ROUTING.md。manifest逐文件hash/大小仍强校验，validator严格允许这四角色并复核CIDR数据hash与provenance；ARM交叉构建、Windows仅client、物理NOT_RUN保持。

## 复用来源

MIT地址快照c5f638a，许可与SOURCE记录已固定；无新旧代码迁移。

## Actions证据

包改变提交前NOT_RUN。d11foundation37153742463 PASS（包括Windows大表/1500owned成功与回滚mock），但不能继承成新SOURCE完整资格。3d真实分流37154014911进行中；此前b650/d11失败永久保留。新正式包由新冻结ref在Actions构建/原生amd64version/manifest/hash校验，不本地执行。

## 问题、排查与风险

旧2b包没有新增默认分流/更新功能；不能引导用户下载旧包验证新功能。新版default策略变化在说明明确，旧私网代理夹具已显式all。Windows物理route装卸/驱动仍P7，hosted无物理承诺。

## 下一项原子任务

冻结新SHA，等core/真实分流PASS后Normal/Game5205独立run、36生命周期与三目标P6收口；结果/未覆盖完整矩阵和长测分别记录。
