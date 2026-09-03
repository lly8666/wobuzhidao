# WBD 实机测试包使用说明

> 状态：**PHYSICAL TEST CANDIDATE / NOT RELEASE-QUALIFIED**。Release 中所有平台包必须使用同一个 `SOURCE_SHA`。Windows 11 + 真实 Npcap -> Ubuntu ARM64 实机验收完成前，不得标记为正式发布。

## 1. Release 文件

发布 Action 会上传：

- `wbd-windows-portable-<SOURCE_SHA>.zip`：Windows 11 x64 portable `wbd.exe`，包含受校验的 child runtime。
- `wbd-linux-server-amd64-<SOURCE_SHA>.tar.gz`：Linux amd64 服务端。
- `wbd-linux-server-arm64-<SOURCE_SHA>.tar.gz`：Linux arm64 服务端，Ubuntu ARM64 实机优先使用此包。
- `wbd-test-tools-<SOURCE_SHA>.zip`：Windows physical/application qualification 脚本、exact-source qualifier、受控 UDP/TCP echo server。
- `SOURCE_SHA.txt`：被测产品源码 SHA。
- `PRODUCERS.json`：三个 Actions artifact 的 producer run 与 artifact receipt。
- `SHA256SUMS.txt`：Release 附件内容 SHA256。
- `wbd.example.json`：Windows 配置模板。

不要把不同 `SOURCE_SHA` 的 Windows/Linux 包混用。

Linux 校验：

```bash
sha256sum -c SHA256SUMS.txt
```

Windows 单文件校验：

```powershell
Get-FileHash -Algorithm SHA256 .\wbd-windows-portable-<SOURCE_SHA>.zip
```

## 2. Ubuntu ARM64 服务端

主机需要 systemd、iproute2，以及 nftables 或 iptables；内核需要 raw socket、TUN、netfilter 能力。云防火墙/安全组必须允许你配置的 WBD 公网端口。

```bash
tar -xzf wbd-linux-server-arm64-<SOURCE_SHA>.tar.gz
cd wbd-server-arm64
sudo ./wbd-server install
```

安装会生成 `/etc/wbd/server.env`，并且不会立即启动服务。推荐先设置一个公网端口，例如 40443：

```bash
sudo /opt/wbd/bin/wbd-server set WBD_PORT 40443
sudo /opt/wbd/bin/wbd-server config
```

至少确认下面这些值：

```text
WBD_LISTEN_IP=0.0.0.0
WBD_PORT=40443
WBD_SERVER_NAME=www.cloudflare.com
WBD_DECOY_TARGET=www.cloudflare.com:443
WBD_ROUTE_KEY=<至少16字符，客户端完全相同>
WBD_USERNAME=wbd
WBD_PASSWORD=<客户端完全相同>
WBD_TUNNEL_POOL=10.66.0.0/16
WBD_SHARED_TUN_IF=wbdg0
```

如果修改 `WBD_SERVER_NAME`，执行：

```bash
sudo /opt/wbd/bin/wbd-server regen-certs
```

启动前：

```bash
sudo /opt/wbd/bin/wbd-server doctor
sudo /opt/wbd/bin/wbd-server start
sudo /opt/wbd/bin/wbd-server status
```

`doctor` 必须出现 `WBD_SERVER_DOCTOR_PASS`。日志：

```bash
sudo /opt/wbd/bin/wbd-server logs
```

Linux 产品路径保持一个公网 raw mux、一个 root-namespace shared WBD TUN 和一个 WBD-owned host NAT。不要按 lane 建多个 TUN/NAT。

## 3. Windows 11 配置

要求：Windows 11 x64、管理员权限、真实物理网卡、Npcap 1.88。把 `wbd.example.json` 复制为例如 `C:\wbd-test\wbd.json`。

关键字段：

```json
{
  "server_ip": "203.0.113.10",
  "server_port": 40443,
  "server_name": "www.cloudflare.com",
  "route_key": "REPLACE_WITH_SERVER_ROUTE_KEY",
  "username": "wbd",
  "password": "REPLACE_WITH_SERVER_PASSWORD",
  "verify_server": false,
  "fec": "off",
  "if_name": "WBD",
  "mtu": 1400,
  "route_mode": "Full",
  "dns_mode": "Auto",
  "dns_server": "",
  "lanes": 1,
  "idle_timeout": 900
}
```

配置约束：

- `server_ip` 必须是 Ubuntu 服务端公网 IPv4。
- `server_port` 必须等于服务端 `WBD_PORT`。
- `server_name`、`route_key`、`username`、`password` 与服务端一致。
- Normal 首轮用 `lanes: 1`；Game/弱网测试可用 `2..4`，每条 lane 都有自己的 single-flow。
- 功能/生命周期主测试保持 `fec: "off"`；只允许额外做固定 `20:20` compatibility smoke，不做新的 60/80 或其他调参。
- `route_mode` 可用 `Full`、`Foreign`、`China`；首轮实机验收建议 `Full`。
- `dns_mode` 可用 `Auto`、`System`、`Cloudflare`、`Custom`。

## 4. 受控 UDP/TCP echo 目标

应用路径验收需要一个**不是 WBD 服务端 underlay/escape IP** 的受控目标，而且 Windows 的最佳路由必须选择 WBD Adapter。最简单做法是在第二台可控 Linux 主机运行测试包里的 echo server：

```bash
python3 qualification_echo_server.py --host 0.0.0.0 --udp-port 37001 --tcp-port 37002
```

放通 UDP/37001 与 TCP/37002。记下其 IPv4，例如 `198.51.100.20`：

```text
udp_echo_target=198.51.100.20:37001
tcp_echo_target=198.51.100.20:37002
```

不要用 WBD Ubuntu 服务端的公网 IP 充当这两个 target；客户端为 underlay server 建立的 escape route 会使这种测试绕过 WBD，qualification 会故意失败。

## 5. Windows 基础 self-test

管理员 PowerShell：

```powershell
New-Item -ItemType Directory -Force C:\wbd-test | Out-Null
Expand-Archive .\wbd-windows-portable-<SOURCE_SHA>.zip C:\wbd-test\portable
C:\wbd-test\portable\wbd.exe -self-test -profile C:\wbd-test\wbd.json -self-test-log C:\wbd-test\support.jsonl
```

至少要看到整个链路成功，并在 `support.jsonl` 的 FakeTCP child 日志中确认：

- `WBD_FAKETCP_WINDOWS_RAW_SYN_TX`
- `WBD_FAKETCP_WINDOWS_RAW_SYNACK_RX`
- `WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared`
- same-flow Reality-like bootstrap ready
- FakeTCP payload TX/RX
- DTLS / LINK / TUN ready
- DNS / UDP / TCP probes success
- cleanup / self-test pass

不得出现 retired standalone Reality bootstrap，也不得把另一个普通 kernel-TCP WBD flow 当作 bootstrap。

## 6. 路由围栏后的 UDP/TCP payload + sustained 测试

解压 `wbd-test-tools-<SOURCE_SHA>.zip`。先算 portable `wbd.exe`：

```powershell
$WbdHash = (Get-FileHash -Algorithm SHA256 C:\wbd-test\portable\wbd.exe).Hash.ToLowerInvariant()
```

`PRODUCERS.json` 中 `producers.windows_portable.run_id` 就是 `ArtifactRunID`。管理员 PowerShell：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\windows_application_path_qualify.ps1 `
  -QualifierExe .\wbd-windows-qualify.exe `
  -Profile C:\wbd-test\wbd.json `
  -UdpEchoTarget 198.51.100.20:37001 `
  -TcpEchoTarget 198.51.100.20:37002 `
  -PortableDir C:\wbd-test\portable `
  -SourceSHA <SOURCE_SHA> `
  -ArtifactRunID <WINDOWS_PRODUCER_RUN_ID> `
  -WbdSHA256 $WbdHash `
  -Rounds 128 `
  -PayloadBytes 4096 `
  -LogDir C:\wbd-test\application-log
```

脚本会用 `Find-NetRoute -RemoteIPAddress` 检查 UDP/TCP target 的选路 interface index 必须就是 WBD Adapter；之后做 deterministic UDP echo、TCP payload echo 和 128×4096-byte 的中等持续流量。期望最终出现：

```text
WBD_WINDOWS_APPLICATION_PATH_PASS ... route_fence=1 ... cleanup=1
```

## 7. GitHub physical workflow

也可以在仓库 Actions 中运行 **Windows Npcap Physical Qualification**，选择带 `[self-hosted, windows, x64, wbd-npcap]` 标签的 Windows 11 runner。输入：

- `artifact_run_id`：`PRODUCERS.json` 的 Windows producer run ID。
- `source_sha`：`SOURCE_SHA.txt` 的完整 40 字符 SHA。
- `profile_path`：runner 本地绝对路径，例如 `C:\wbd-test\wbd.json`。
- `udp_echo_target`：受控、且经 WBD 路由的 UDP echo 地址。
- `tcp_echo_target`：受控、且经 WBD 路由的 TCP echo 地址。
- `probe_rounds`：默认 128。
- `probe_payload_bytes`：默认 4096。
- `uninstall_after`：必须保持 false；Npcap 属于 runner 环境，不由产品测试卸载。

这条 workflow 会重新核对 source SHA、artifact producer、Npcap、raw SYN/SYNACK receipt、基础 self-test、应用层 route fence 和 cleanup。

## 8. 必做生命周期/网络变化清单

基础路径绿后继续记录：

1. `lanes=1` Normal 持续流量。
2. `lanes=2..4` 弱网/Game，多 lane first-arrival + dedup，无跨 lane HOL。
3. idle -> DORMANT -> wake。
4. lane age rotation。
5. child/LINK failure。
6. planned replacement A -> A+B -> B；候选 B 失败必须保留 A。
7. sleep/resume。
8. 物理 link down/up。
9. DHCP renew。
10. default route change。
11. VPN on/off（环境允许时）。
12. underlay rebind。
13. connected / DORMANT / replacement 全阶段 IPv6 fail-closed。
14. clean disconnect 后无遗留 WBD 进程、WBD-owned route/firewall；再做一次 dirty-exit/startup recovery。

当前没有权威外部公网 NAT mapping reflector。因此 SourceIP、DHCP 或 default-route 变化只能记录为本地 underlay/network transition，**不能**声称为“直接检测到公网 NAT 映射变化”。

## 9. 最终保留证据

归档：Release tag、`SOURCE_SHA.txt`、`PRODUCERS.json`、`SHA256SUMS.txt`、Windows `support.jsonl`、physical workflow 完整日志、application qualification stdout/stderr、Linux journal、UDP/TCP/sustained 统计、lifecycle/network-transition 记录。

只有同一 SOURCE_SHA 的 hosted automation + Windows/Linux artifact + 新鲜 Windows 11 Npcap -> Ubuntu ARM64 实机证据全部为绿，才可以进入 RELEASE-QUALIFIED 判定。
