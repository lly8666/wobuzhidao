# 参数与配置：所有 agent 的必读入口

权威清单是同目录 `PARAMETERS.json`，按 Linux 客户端、Windows 客户端、Linux 服务端分别列出每个参数、类型、默认表达式、说明和源码。不要只读旧日志、示例或某个平台的 main。`tools/parameter_catalog.py` 从实际 CLI 定义生成清单；Actions 比对清单和源码，参数新增、删除、默认值或说明变化未同步即失败。

开发流程：修改 CLI/默认常量 → `python tools/parameter_catalog.py --write` → 更新本文的语义、限制、示例 → 补 Actions 覆盖 → 更新 STATUS 和开发日志。新增开关不能只写进提示词。配置加载只有 `internal/configfile` 一套，禁止另写 JSON/YAML 参数模型。

## 使用方式

Linux/Windows 正式入口均支持 `--config 路径.json`。JSON 是扁平对象，键名和 CLI 去掉 `--` 后完全一致；时长使用字符串，例如 `"30s"`、`"5m"`。优先级为显式 CLI > JSON > 内置默认。`--tls-startup-padding=false` 可以覆盖配置中的 true。

配置片段（需合并本机接口、身份、密钥、地址等必需字段后运行）：

```json
{
  "tls-startup-padding": true,
  "fec-parity": 20,
  "lanes": 1,
  "keepalive-interval": "15s",
  "idle-dormant": "5m"
}
```

客户端另可设置：

```json
{
  "dead-after": "90s",
  "reconnect-min": "1s",
  "reconnect-max": "30s"
}
```

服务端不能读取客户端专有键。未知键、重复键、null、数组、嵌套对象、尾随内容以及超过 1 MiB 的文件拒绝启动；不递归加载 config，不从配置文件触发 version。失败信息不回显配置值。只在启动时读取，没有热加载；凭据文件自行限制访问权限，不上传到日志/artifact。

## 生命周期与外观参数

| 参数 | 默认 | 范围/语义 |
| --- | --- | --- |
| `tls-startup-padding` | false | 有限内层 TLS 启动填充；FEC off 也生效。细节见 TLS_STARTUP_PADDING.md；不等待凑包，不填充 parity/保活，不重新随机重传。 |
| `keepalive-interval` | 15s | 两端各自配置；1s～1h。每条 active lane 一个独立加密 health record，不走 FEC，不创建业务 flow。0 在内部 API 表示默认值；CLI 0 同样使用默认，不是关闭。部署建议两端一致。 |
| `dead-after` | 90s | 客户端；至少本端 keepalive 的 3 倍，最多 24h；0 使用默认。还应大于对端发送间隔及预期弱网迟到窗口。无有效加密接收才怀疑失活，TCP ACK 不算健康证明。 |
| `reconnect-min` | 1s | 客户端，至少 1s；候选失败后的最短间隔。0 使用默认。 |
| `reconnect-max` | 30s | 客户端，至少 min、最多 10m；指数增长、有限随机抖动。0 使用默认。 |
| `idle-dormant` | 0 | 0 禁用自动休眠；正值按业务活动判断。必须同时有新鲜、经过认证的对端空闲证据；保活缺失不构成空闲证据。保活、外层 ACK、FEC timer、repair 不刷新业务时钟。 |
| `rotate-min` / `rotate-max` | 0/0 | 客户端定时轮换，配对启用；和黑洞恢复共用一次一个候选的串行调度，不按高丢包下首次成功验收。 |
| `fec-parity` | 0 | 固定档位：0(off)、4、8、10、12、16、20；数据分片 20，单 incarnation 不热切档。 |
| `lanes` | 1 | 1=Normal，2～4=Game。逻辑 lease 不因轮换/休眠/重连改变。 |
| `diagnostic-jsonl` / `diagnostic-interval` | 空/1s | 当前 Linux 两端已有；包含新增 health、pressure、客户端 recovery 计数。Windows 暂无此诊断文件开关，不能宣称 CLI 全平台完全一致。 |

休眠后由客户端新业务触发唤醒。服务端没有脱离现有 lane 的反向唤醒通道；不能保证两端已经完全休眠后的服务端主动推送。需要这种持续接收能力时保持 idle-dormant=0。默认不自动休眠。

内部固定边界不是 CLI 参数：4096 outstanding、3s repair horizon、重传新流量补充比例 1/5、128KiB 启动 credit、64 代/3s FEC 生命周期、padding 预算、最大物理 lane 等仍以正式设计及源码为准。此次只移植接收速率/RTT 压力退役，不把这些常量伪装成可配置开关。

## 协议兼容

本轮 lifecycle-capable admission record version 为 **2**；真实 TLS/uTLS 建连、外层 0x17/0x0303 记录格式不变，新增的是加密内部 health kind。必须成对升级端点。V1 端点在 admission 被明确拒绝，不允许混用后静默反复重连。历史 V1 的通过证据仍保留历史 SHA，不能视作 V2 已通过。
