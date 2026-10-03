# 20261003-211000 长测完整性门与小诊断产物

## 本轮目标和阶段

P5交付前最终检查，起点7b8f9fc5c5d822336b611768c42522ac7928dfa3。运行产品代码保持锁顺序修复版本。

## 修改与原因

长测原门已检查应用CRC/去重/实际损伤/资源/退役，但还应逐行检查每一代lane及transport的RecordErrors/PathErrors，不能只看最终代。追加严格零错误及诊断字段缺失拒绝，fixture覆盖旧代报错、新代归零仍不能PASS。新增单独压缩JSON/JSONL/log/txt诊断artifact，避免调试时必须下载多GiB抓包；完整原始抓包及摘要产物继续保留，所有处理均在测量结束后。

## 复用来源

当前check_target_soak和已有lane/transport计数，无old复用，不新增产品参数。

## Actions证据

新精确源码基础/fixture/race/全配置/18弱网/36生命周期/正式长测/P6均NOT_RUN，待Actions。7b锁边界基础正在运行；旧1a已因真实HTTPS失败阻止资格，原始证据保留，未完成旧源码工作明确superseded。

## 问题、排查与风险

新增门不调低已有业务/时延门。计数仍是运行时采样，不声明捕获所有亚采样时间内行为。不会在负载期间压缩或解析pcap。

## 下一项原子任务

最后冻结该源码，完成所有hosted门、更新唯一STATUS及下载包入口；P7物理仍NOT_RUN。
