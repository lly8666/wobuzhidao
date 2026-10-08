# Longmix A/0 首轮运行 INVALID（助手环境缺项）

产品SOURCE b4ea061178a6e09b7e7c8587d72b4b8535492567，helper 6f51248b95281298d96c38cc859cac6b1bd2be23。真实独立run https://github.com/lly8666/wobuzhidao/actions/runs/37729338574：仓库/policy/claim、精确产品两端编译、tc构建及脚本生成均SUCCESS；旧strict路径被跳过。A业务脚本入口line6拒绝 `WBD_STRICT_ARTIFACT_DIR` 缺失（sudo未得到主job的`ART`别名）。因此没有正式endpoint运行，性能阶段0秒、业务/netem数据0，analyzer INVALID，final FAIL。属于harness false start，不能归因产品/大包/runner；保留原始失败。

只改 `env WBD_STRICT_ARTIFACT_DIR="$ART" GITHUB_SHA="$TESTED_SOURCE_SHA"` 的显式赋值，另加static guard。先Actions preflight后重新独立run，未修改产品、冻结ref、FEC、MTU及任何物理环境。A/5/20/30及B/C均NOT_RUN；M03/1554两个原故障仍OPEN。
