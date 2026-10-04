# 20261005-071000 窗口候选资格回归修正

## 目标与来源

用户授权持续开发测试，分支next/tlslike-dataplane；父SOURCE bde76a9938e3e8fe1269d8544e20045186128543。部署仍6181db6。没有增加第二项产品优化。

## 修改

Actions发现ClientAssociation综合测试同时用同一wantWindow断言握手与detach；握手仍应1024，detach候选已65535。只修正detach断言，握手断言保留。这不是产品逻辑失败或放宽窗口门。

新增harness_sha来源分离使prepare_soak_harness的精确字符串模板保护主动失败。将替换范围缩小到schema和source_sha，保留新harness_sha表达式；已有生成器测试补断言防止丢失独立来源。长期测试仍一Action一条，不改时长、注入和验收门。

## Actions与状态

父提交foundation37242207828、targeted37242207775 FAIL（旧综合detach预期）；predelivery37242207864 FAIL（模板保护），lifecycle37242207792及GUI37242207875 PASS。四独立性能/状态路由run已启动：Normal5205 37242239754、Game5205 37242242521、candidate stateful lossless37242244758、旧SOURCE6181 stateful lossless37242246886，尚未取得结论。完整链接形式https://github.com/lly8666/wobuzhidao/actions/runs/对应ID。

当前提交仅修测试/生成器，将重新验correctness/race；性能父版本与此提交产品Go源码相同，但归属必须记录实际构建SHA，不能称本提交整矩阵PASS。

## 下一项

读取四样本原始结果和新core/race；窗口资格过后修已抓到的source-to-wire换代竞态。实机不部署未经Actions验证的候选。保留两个未满300s诊断与真实客户端fatal，不计五分钟完成数量。
