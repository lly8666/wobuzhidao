# Reality-style target-mirror diagnostic

## Purpose

This is an **isolated diagnostic oracle**, not the WBD product data plane.

It reproduces the opening/fallback property that is useful for network-treatment experiments:

```text
client TCP -> WBD mirror server
              -> fixed genuine TLS target

client ClientHello --byte-for-byte--> target
target TLS response --byte-for-byte--> client
then bounded bidirectional splice
```

The client therefore receives the real target's ServerHello, certificate chain, CertificateVerify and Finished. WBD does not possess, copy or forge the target private key.

This experiment answers a narrow question: does a TCP flow to the WBD server IP receive measurably different treatment when its visible TLS handshake and subsequent HTTPS traffic are genuinely supplied by a selected target host?

It does **not** yet carry WBD UDP-like application traffic on the same TCP stream. Doing that would require a separate authenticated protocol design; blindly placing the WBD data plane inside a TLS byte stream would reintroduce stream HOL and violates the current transport constitution.

## Safety / abuse boundary

The server is deliberately not an open SNI proxy:

- exactly one `-target HOST:PORT` is configured;
- exactly one `-server-name HOST` is accepted;
- unexpected SNI is rejected before dialing the target;
- default listen address is loopback;
- default session lifetime is 30 seconds;
- default transfer ceiling is 32 MiB per direction;
- default concurrent session limit is 32.

If exposed publicly for a test, keep the target fixed and the test window short. A generic fallback proxy can otherwise be abused as a relay.

## Build

```bash
go build -o wbd-reality-mirror ./cmd/wbd-reality-mirror
go build -o wbd-tls-diag ./cmd/wbd-tls-diag
```

## Server-side demo

Run this on the WBD server/VPS. Replace the placeholder target with the genuine HTTPS host you want to use as the control identity.

```bash
./wbd-reality-mirror server \
  -listen :9443 \
  -target TARGET_HOST:443 \
  -server-name TARGET_HOST \
  -session-timeout 30s \
  -max-bytes 33554432
```

The process prints `WBD_REALITY_MIRROR_READY` followed by one JSON result per connection. The result includes parsed SNI/ALPN and mirrored byte counts.

For a short controlled throughput experiment, `-max-bytes 0` removes the userspace byte-limit wrapper and lets Go's normal TCP `io.Copy` fast path operate. Keep a short `-session-timeout`, low `-max-conns`, and stop the public listener immediately after the experiment.

## Client-side handshake comparison

Direct target baseline:

```bash
./wbd-tls-diag \
  -addr TARGET_HOST:443 \
  -server-name TARGET_HOST \
  -count 20
```

Same genuine TLS target, but reached through the WBD server IP:

```bash
./wbd-tls-diag \
  -addr WBD_SERVER_IP:9443 \
  -server-name TARGET_HOST \
  -count 20
```

The target certificate/SPKI hashes should match because both cases terminate TLS at the genuine target. Compare connection success, TCP connect p50/p95 and TLS handshake p50/p95.

For the preferred short-window comparison, alternate the two paths so a few minutes of changing network quality cannot systematically favor whichever group was run first:

```bash
python3 scripts/bench_reality_mirror.py \
  --diag ./wbd-tls-diag \
  --direct TARGET_HOST:443 \
  --mirror WBD_SERVER_IP:9443 \
  --server-name TARGET_HOST \
  --pairs 20 \
  > reality-mirror-handshake.json
```

The JSON reports each pair, direct/mirror success ratios, p50/p95 and `mirror_minus_direct` deltas. Pair order alternates `direct -> mirror` then `mirror -> direct`.

## HTTP/data comparison

A normal HTTPS client can use the same mirror. For example, with a target URL that you are permitted to benchmark, direct access is compared with `curl --connect-to`. `--connect-to` keeps the URL authority, Host header, SNI and certificate hostname at normal port 443 while redirecting only the TCP destination to the WBD mirror port.

```bash
curl -o /dev/null -sS \
  -w 'direct code=%{http_code} connect=%{time_connect} tls=%{time_appconnect} first=%{time_starttransfer} total=%{time_total} bytes=%{size_download} speed=%{speed_download}\n' \
  https://TARGET_HOST/TEST_PATH

curl --connect-to TARGET_HOST:443:WBD_SERVER_IP:9443 \
  -o /dev/null -sS \
  -w 'mirror code=%{http_code} connect=%{time_connect} tls=%{time_appconnect} first=%{time_starttransfer} total=%{time_total} bytes=%{size_download} speed=%{speed_download}\n' \
  https://TARGET_HOST/TEST_PATH
```

The target must accept the Host/SNI and request path you choose. Keep public-service tests modest; this is a network-treatment diagnostic, not a load generator.

## Interpretation

Useful experiment groups are:

1. direct genuine target;
2. genuine target through `wbd-reality-mirror`;
3. direct WBD TLS/Persona endpoint;
4. WBD FakeTCP/DTLS data plane.

Group 1 vs 2 isolates the effect of destination IP/path while holding the TLS endpoint identity genuine. Group 2 vs 3 helps separate target TLS appearance from the WBD server's own certificate/handshake. Group 3 vs 4 then isolates the product carrier.

Do not infer a carrier policy from a single run. Alternate direct/mirror samples over the same short interval and repeat during the suspected good/bad daily periods.
