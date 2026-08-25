# Reality-style target-mirror diagnostic and encrypted demo handoff

## Purpose

This remains an **explicit demo mode**, not the default WBD transport.

The genuine target mirror is used only for the network-treatment preflight:

```text
client TCP -> WBD mirror server -> fixed genuine TLS target
client ClientHello --byte-for-byte--> target
target TLS response --byte-for-byte--> client
```

The client therefore receives the real target's ServerHello/certificate/Finished and validates the genuine target normally. WBD never possesses or forges the target private key.

The follow-on demo does **not** switch this TCP/TLS byte stream into VPN payload. Instead it closes the preflight and binds a fresh WBD DTLS 1.3 association with a short-lived one-time ClientHello witness:

```text
mirror preflight -> witness
fresh FakeTCP/DTLS association
DEMO_BIND -> DEMO_BIND_OK -> LINK_INIT -> LINK_ACCEPT -> AUTH -> encrypted WBD datagrams
```

This keeps WBDC control, account token, FEC metadata and application payload encrypted by DTLS and preserves the unordered/no-HOL data plane.

## Safety / abuse boundary

The mirror is deliberately not an open SNI proxy:

- exactly one `-target HOST:PORT` is configured;
- exactly one `-server-name HOST` is accepted;
- unexpected SNI is rejected before dialing the target;
- default listen address is loopback;
- default session lifetime is 30 seconds;
- default transfer ceiling is 32 MiB per direction;
- default concurrent session limit is 32.

`-witness-dir` is also demo-only. Witnesses are non-secret correlation hashes, stored locally with a short lifetime and consumed once by the WBD startup gate.

## Build

```bash
go build -o wbd-reality-mirror ./cmd/wbd-reality-mirror
go build -o wbd-tls-diag ./cmd/wbd-tls-diag
go build -o wbd-link-proxy ./cmd/wbd-link-proxy
```

## 1. Standalone mirror measurement

Server/VPS:

```bash
./wbd-reality-mirror server \
  -listen :9443 \
  -target TARGET_HOST:443 \
  -server-name TARGET_HOST \
  -session-timeout 30s \
  -max-bytes 33554432
```

Direct target baseline:

```bash
./wbd-tls-diag \
  -addr TARGET_HOST:443 \
  -server-name TARGET_HOST \
  -count 20
```

Same genuine target through the WBD server IP:

```bash
./wbd-tls-diag \
  -addr WBD_SERVER_IP:9443 \
  -server-name TARGET_HOST \
  -count 20
```

The target certificate/SPKI hashes should match because both paths terminate TLS at the genuine target.

Preferred paired comparison:

```bash
python3 scripts/bench_reality_mirror.py \
  --diag ./wbd-tls-diag \
  --direct TARGET_HOST:443 \
  --mirror WBD_SERVER_IP:9443 \
  --server-name TARGET_HOST \
  --pairs 20 \
  > reality-mirror-handshake.json
```

Pair order alternates direct/mirror to reduce short-term drift.

## 2. Explicit encrypted demo handoff

This mode requires all three explicit demo pieces. Without the `-demo-reality-*` flags, normal WBD startup is unchanged.

### Server: start mirror witness producer

Choose a server-local directory that is not shared over the network:

```bash
sudo install -d -m 700 /run/wbd/reality-demo

./wbd-reality-mirror server \
  -listen :9443 \
  -target TARGET_HOST:443 \
  -server-name TARGET_HOST \
  -witness-dir /run/wbd/reality-demo \
  -session-timeout 10s \
  -max-conns 8
```

After a successful mirrored TLS session with target-to-client bytes, the mirror records the exact ClientHello SHA-256 in that local directory.

### Server: start WBD link gate

The DTLS shim remains outside this process exactly as in the normal product path. Point `-listen/-service` at the same local plaintext endpoints used by the normal `wbd-link-proxy`, and add the demo gate:

```bash
./wbd-link-proxy \
  -mode server \
  -listen SERVER_PROXY_UDP \
  -service LOCAL_SERVICE_UDP \
  -expected-token 'DEVICE_SECRET' \
  -demo-reality-witness-dir /run/wbd/reality-demo \
  -demo-reality-server-name TARGET_HOST \
  -demo-reality-ttl 15s
```

A WBD association is rejected before LINK_INIT unless it first presents a matching one-time witness inside DTLS.

### Client: perform genuine TLS preflight and save witness

Use exactly one successful handshake for the handoff:

```bash
./wbd-tls-diag \
  -addr WBD_SERVER_IP:9443 \
  -server-name TARGET_HOST \
  -count 1 \
  -witness-out /tmp/wbd-reality.witness
```

`wbd-tls-diag` performs ordinary target certificate/hostname validation. The witness file contains only the SHA-256 of the exact ClientHello bytes and is written mode `0600`.

### Client: establish the encrypted WBD association

```bash
./wbd-link-proxy \
  -mode client \
  -listen CLIENT_PROXY_UDP \
  -dtls CLIENT_DTLS_PLAINTEXT_UDP \
  -fec 20:20 \
  -token 'DEVICE_SECRET' \
  -demo-reality-witness "$(cat /tmp/wbd-reality.witness)"
```

Startup on the public path is then:

```text
FakeTCP
  -> DTLS 1.3 Finished
  -> encrypted DEMO_BIND
  <- encrypted DEMO_BIND_OK
  -> encrypted LINK_INIT
  <- encrypted LINK_ACCEPT
  -> encrypted AUTH
  <- encrypted AUTH_OK
  -> encrypted WBD application datagrams
```

There is no point where WBDC, the bearer/device secret, FEC parameters or application data are sent in plaintext or inserted into the genuine target TLS stream.

## 3. Normal mode / self-signed certificate

Do not pass any `-demo-reality-*` options. The mirror is not consulted and no witness is required.

A normal deployment may use an operator-created self-signed DTLS certificate. The client must trust that exact certificate as a local trust anchor or validate a fixed SPKI; do **not** disable certificate verification. Self-signed changes the trust root, not the TLS/DTLS security requirement.

## HTTP/data comparison for the mirror itself

A normal HTTPS client can still use the standalone mirror for controlled measurements. `--connect-to` keeps URL authority, Host, SNI and certificate hostname at port 443 while redirecting only the TCP destination:

```bash
curl -o /dev/null -sS \
  -w 'direct code=%{http_code} connect=%{time_connect} tls=%{time_appconnect} first=%{time_starttransfer} total=%{time_total} bytes=%{size_download} speed=%{speed_download}\n' \
  https://TARGET_HOST/TEST_PATH

curl --connect-to TARGET_HOST:443:WBD_SERVER_IP:9443 \
  -o /dev/null -sS \
  -w 'mirror code=%{http_code} connect=%{time_connect} tls=%{time_appconnect} first=%{time_starttransfer} total=%{time_total} bytes=%{size_download} speed=%{speed_download}\n' \
  https://TARGET_HOST/TEST_PATH
```

Keep public-service tests modest; this is a network-treatment diagnostic, not a load generator.

## Interpretation

Useful experiment groups remain:

1. direct genuine target;
2. genuine target through `wbd-reality-mirror`;
3. normal WBD DTLS endpoint;
4. demo-preflight-bound WBD DTLS/FakeTCP data lane.

Group 1 vs 2 tests path/destination-IP treatment while keeping a genuine TLS endpoint. Group 3 vs 4 then asks whether requiring the genuine target preflight before the otherwise identical encrypted WBD data lane changes real observed treatment.

Do not infer a carrier policy from a single run. Alternate samples over the same short interval and repeat during suspected good/bad daily periods.
