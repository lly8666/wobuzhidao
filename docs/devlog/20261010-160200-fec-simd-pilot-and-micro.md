# FEC SIMD S3 Pilot 和微基准调度

- 上个准确 helper HEAD d48397ca0c2303eb285b877c5d504e5a7169e056 的新 scoped preflight [Actions 38033774948](https://github.com/lly8666/wobuzhidao/actions/runs/38033774948) SUCCESS，但完全无真实业务。
- 将 config 由 preflight 更新为唯一允许的 pilot（Q1旧A、新B各15秒），新阶段仍同job、同Go1.23.12、两个独立产品二进制，原socket/netns业务。
- 修正 build_seeded_tc 同一个 shell step 写 GITHUB_ENV 不会即时生效的问题；显式 export WBD_STRICT_TC，避免pilot因未注入工具路径产生虚假网络失败。
- 创建新 micro config-only push workflow，对固定B代码源码差分守卫、单Actions AMD host上重复3次短片 32/96/256/512/1000/1400 scalar、auto/完整20:P span/fused，记录host/cgroup、原始bench日志；含tag optin的span标签不是独立span样本，不能以该标签证明收益。
- 本提交前 pilot/micro 均未运行，真实 CPU收益 NOT_MEASURED；Q/L/300s/ARM performance/P6/physical NOT_RUN。历史失败继续保留。
