# 20261003-110000 FEC20:20分长度组减少校验字节膨胀

## 本轮目标和阶段
用户当前要求解决带宽开销过大，优先级高于尚未实现的长测框架。开始HEAD c14591f3936861ef53828a3e9b79b57046dbfcd1；产品基线56eb5413c3cf2e559b82026e8a5783508764e2f4。P5保持IN_PROGRESS。

## 修改与原因
原正式无损Normal样本37065832537，C2S线上IP763,828,672B/原业务150,000,000B=5.09倍，repair=0、padding off；parity481,234,382B是主要额外成本。20:20源/校验数量相近，但混合长度组的每个校验包都按组内最大源长度发送。
新增internal/fec/size_class_encoder.go：20:20最多3个lane-owned固定组，LINK源长度上界256/512/SourceMTU（按MTU去重）。源包Add立即发送，原8ms期限从各组首源独立开始，全组共享连续BlockID分配器，在block开始时分配，支持uint32回绕。原FastBlockEncoder/codec、v1 header、MTU、systematic first-arrival和owned复制全部保留。只额外预分配两个小组，不新增goroutine/动态流分类/等待。
20:4/8/10/12/16保留单组及逐字节原输出，避免改变min(N,R)部分组冗余比例；off完全不进入encoder。Game同PacketID复制、4096 shadow repair及3秒恢复期限、decoder上限不变。统计新增size_classes/pending_blocks，既有分层计数字段仍可由原分析器读取。
测试锁定20:20混合长度校验预算、未修改v1 decoder反序parity-only恢复、跨组无HOL、独立deadline、wrap、部分组恢复/迟到去重、有限pending、低冗余档位逐字节兼容、LINK跨组分片恢复与owned快照复用不变。

## 复用来源
仅复用当前新产品internal/fec/fastblock_encoder.go及codec，未读取/提取old，不改变REUSE_LEDGER。

## Actions证据
新产品尚未运行：unit/build/race与端到端性能全部NOT_RUN；基线资格仅覆盖56eb541。提交后由next-foundation执行Linux/Windows unit/build、Linux race、跨模块基础/真实路径回归。通过后冻结精确候选SHA，先独立Normal1每向10M/lossless单条，再Normal 5205/5305及Game4每向逻辑3M三场景；正式重复均一Action run一条样本。

## 问题、排查与风险
预期减少字节而不是减少20:20源/校验数量；头部/ACK与Game4固有4倍复制仍有成本。分组改变恢复粒度和block分布，必须由真实弱网/RTT/resource门验证，不能继承旧formal18或只凭单测声称提速。低档位暂不启用分组。不能通过关闭FEC/Game、扩大buffer、主动丢包或更改输入构成来制造改善。固定额外encoder内存约65KB/lane，有界；所有测试只在Actions。

## 下一项原子任务
验收候选Linux/Windows unit/build/race后运行精确SHA独立无损Normal canary，核对原业务注入、outer IP、source/parity字节和数量、goodput、RTT、CPU、socket/AF_PACKET drops、repair、完整性。无损满速/0丢失必须通过，再执行独立损伤与Game矩阵并记录每项结果。长测/P6/P7继续NOT_RUN。
