# 20261003-202600 精确定位manifest后的整理入口

## 本轮目标和阶段

P5交付前框架，起点b1e2703ab784e81f98d8b358e43a859024a07a00。

## 修改与原因

Actions fixture发现模板同时在清理函数和结束处使用chmod，替换计数正确拒绝模糊锚点。改为唯一的manifest heredoc结束锚点，不修改产品和测试门。

## 复用来源

当前prepare_soak_harness，无old复用。

## Actions证据

b1工具run37122241498 FAIL（模板替换2处，要求1处）；同run30次race PASS。新SHA NOT_RUN，基础门必须重新通过。原失败保留。

## 问题、排查与风险

问题在生成器而非运行程序，不能跳过fixture强制调度。

## 下一项原子任务

基础门通过后调度交付前独立配置/弱网/长测/包。
