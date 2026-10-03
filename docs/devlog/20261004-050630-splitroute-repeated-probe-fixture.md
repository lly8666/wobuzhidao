# 20261004-050630 重复直连探针服务端口复用

## 本轮目标和阶段

SOURCE d11a3c3c575d18e803497bbaf2742eb255d5b967已证明三目标真实路由，但第二次探针失败，修夹具生命周期，不修改产品分流。

## 修改与原因

config_business普通TCP listener设置SO_REUSEADDR，与既有HTTPS listener一致，使同目标重复探针不受前次TCP关闭后的TIME_WAIT端口占用干扰。仍校验全部内容、目标真实peer与休眠/唤醒；不加重试、不忽略失败。

## 复用来源

无。

## Actions证据

d11 splitroute37153742459，embedded artifact11284808090（sha256800a745247c7d7bad461e5894188e0b30492f78ae3246d9e2df9af28e21c46f5）只读下载核验：LAN/CN的DNS/TCP/HTTPS peer均10.40.0.2，foreign均10.50.0.1，body102400B hash匹配；之后dormant-lan DNS成功而TCP ConnectionRefused，前一次同端口服务已退出。all原attempt PASS，其它三个原FAIL保留，不能只报第一组三目标成功。

## 问题、排查与风险

新回执需整个休眠direct→代理wake/清理完整通过。Windows/全core新源码仍须重新验，P7 NOT_RUN，每性能run一条。

## 下一项原子任务

收口新SHA完整功能/core/Windows，再两条独立5205与新P6包。
