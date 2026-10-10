# 修复微基准YAML解析失败（未执行测量）

- 本次新的 B SOURCE 7fb98fab79834a351a1dbe04eebb207f66bea28b；之前初始微基准 run38033888108 运行成功但属于默认span源码 89fcb5e99ffc6ae63354ea6628d367ada72d6bed。
- 追加 wbd_fec_span 的 YAML 程序化替换时误将shell `-run '^$'` 内的 `$'` 解释为 JavaScript replacement token，使原始workflow自动重复末尾文本，Github run [38034440396](https://github.com/lly8666/wobuzhidao/actions/runs/38034440396) 无job、直接FAIL_WORKFLOW_YAML_PARSE。绝不称为产品测试失败或通过，保留此FAIL。
- 从已运行成功的f6603f micro YAML重新派生，安全函数替换，恢复为default fused、explicit span、scalar及forced fused，仍在相同Actions单CPU三次重复，严格核代码差分和源B；phase/source/nonce校验重新请求 config-only push。
- 增加静态性能工作流策略防止重复上传节点、意外独立-bench行，实测结果仍未得，原其他FAIL和 physical NOT_RUN继续。
