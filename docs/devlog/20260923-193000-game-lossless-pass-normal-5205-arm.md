# 20260923-193000 Game lossless PASS，武装 Normal 5205

## Game lossless 有效资格

SOURCE_SHA `50eb31f208b22a49aed172dbea4d797f6196e3bb`，`next-strict-weaknet` run `35845883866` / job `107131808395`：

- 输入：Game4 / logical 3Mbps each direction / lossless / seed601 / FEC20:20 / 300ms one-way；
- one-run-one-sample；
- CORRECTNESS / INPUT_VALIDITY / CAPTURE / ENVIRONMENT / PERFORMANCE：全部 PASS；
- performance_errors：空；
- c2s actual/eventual约3.0003Mbps，s2c同量级，packet/byte loss=0；
- raw pre wall约2.9699Mbps；300ms delay-aligned pre：c2s 2.999483733Mbps、s2c 2.999957333Mbps；门槛仍2.97Mbps；
- probe timeout=0；socket/link drop=0；
- process CPU seconds：client 107.91、server 107.94；
- server AF_PACKET rmem最高约0.859 rb，但drops=0；
- repair outer bytes=0，padding=0；
- transport_hygiene=REVIEW，非门控；主要是多lane duplicate ACK / fresh-seq-regression抓包外观观察，没有 correctness failure。

证据：
- compact summary artifact `10743975832`，digest `sha256:c47530c20af7f864b6f4cc0395f0fc1eb34ea2189625be96d02baf5eb475a75e`；
- full artifact `10744095265`，931356836 bytes，digest `sha256:2dd6100025986057da140ee2fd2234696db10623602a8806dd3d55eaba091d42`。

## 当前判断

Normal 10Mbps lossless run `35845134507` 与本次 Game4 3Mbps lossless 均PASS，满足 `docs/WEAKNET_QUALIFICATION.md §10.2` 进入弱网独立样本的前置条件。

本次Game的outer/app放大约c2s 20.63×、s2c 21.16×，包含四lane复制与FEC20:20，继续作为成本账本观察；它没有触发本地drop、吞吐或正确性失败，因此不在此处顺手改协议。

## 下一条

武装同一个strict workflow的独立 `normal / 5205 / seed601 / 10Mbps / 1lane` 请求。每个Action run仍只执行一条性能样本。

只有5205形成有效结果后再决定是否进入5305：
- PASS：进入独立Normal 5305；
- FAIL：先读取summary、recovery accounting、post5和资源证据，再定向修复，不机械继续。
