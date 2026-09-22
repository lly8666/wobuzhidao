# 20260922-191000 启动填充关闭观测与默认off路径

## 目标、来源和修改

基于本轮功能提交d5996203ff0617aca53fcc1dc9e2713cba97bd62做提交后自审。无old复用。Close此前通过重置整个tracker清状态，同时会清掉StartupDetected等计数，不利于测试结束后取证；改为只清map/list，保留累计计数，追加断言。Normal inbound在原有owner锁内读取immutable开关，off时不进入额外观察器锁，保留默认路径开销。补正legacy policy注释。

## 验证与风险

本地仅编辑及Git检查，无编译/测试。代码资格仍待exact-SHA Actions及功能规范中的正式进程验收，不能把本次静态自审写成PASS。本次不改流量、握手、FEC、MTU、repair或预算。

## 下一项

新agent按TLS_STARTUP_PADDING.md检查最后产品SHA的专项/基础Actions并补fullstack，失败定向修复，通过后直接关闭小功能；原主线保持用户暂停。
