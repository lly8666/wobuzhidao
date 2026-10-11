# N2 第一轮 GitHub Actions 发现测试预期错误，最小修复

时间 2026-10-11，代码SOURCE `47b94777b91f4834854d7bfc049a130eda17d913`，GitHub Actions foundation [38120812356](https://github.com/lly8666/wobuzhidao/actions/runs/38120812356)，Linux go test 失败 `TestHighPhysicalLossEscalatesButOverloadFreezes`，日志实际 `RAISE` ordinary从4升到12，测试预期却先HOLD。

真实设计：512为自上次决策后成功记录总量。85个各256记录窗口期间已足量，capacity期间一份新样本不参与，首次解除capacity的同份认证首发损伤报告应快速升档；而不是另等新512记录。修正测试断言“即时RAISE然后单256不能再升”，产品controller.go一字不改。

旧失败保留，下一提交GitHub Actions重新验证；N2正式接入仍不允许auto，N1 post-repair仍HOLD，physical未执行。
