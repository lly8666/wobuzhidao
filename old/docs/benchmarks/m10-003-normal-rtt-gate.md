# M10-003A normal-network real RTT gate

Status: locally qualified normal-network gate. This checkpoint deliberately does not add loss, FEC tuning, REALITY/Vision, QUIC fault injection, or TUN.

## Definition

The phrase `40-60 ms network` is ambiguous. This gate defines it explicitly as **RTT 40-60 ms**, not one-way delay:

- client -> server userspace fault proxy delay: 20-30 ms per message
- server -> client userspace fault proxy delay: 20-30 ms per message
- loss: 0%
- bandwidth/queue impairment: none
- seeded schedule: 5001
- logical payload: 256 bytes

The 64-sample seeded schedule has a target arithmetic mean RTT of 49.445669 ms.

The fault path uses real `127.0.0.1` kernel TCP/UDP sockets. WBD uses a real kernel TCP carrier through the same schedule, real WBD frame encoding/decoding, `lane.TCP`, and server-side `session.Receiver.AcceptData`. The peer echoes logical DATA immediately after receiver delivery. No logical ACK, protection window, FEC, duplicate, or rescue action gates application delivery.

`Auto` receives a clean logical-delivery observation after each completed sample; it must stay at 1.0x for the whole run.

## Why modes run concurrently

A shared CI/sandbox VM can occasionally deschedule the process for tens of milliseconds. Running TCP, UDP, WBD normal, and WBD Auto sequentially made whichever mode happened to run during a host pause look artificially slower. The qualification matrix therefore runs all four modes concurrently against the exact same deterministic delay schedule.

The report preserves raw arithmetic mean and p95/p99. It also records:

- `MedianExcess`: median of `measured RTT - scheduled RTT` per sample;
- `HostOutliers`: samples whose excess is greater than 10 ms;
- `InlierMean`: arithmetic mean after excluding those explicitly reported host-outlier samples.

This does not hide outliers: they remain in raw mean/p95/p99. The normal gate uses typical excess to detect a systematic WBD wait while avoiding a false protocol failure caused by one unrelated VM suspension.

## 64-sample local result

Report SHA-256: `818a6b807772d1097cbc511b9971a14e9cfe1da816ed080e199d71058d85e6ca`

| Mode | Raw mean | Inlier mean | p50 | p95 | Median excess | Host outliers | Final RBC |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| native TCP | 51.857 ms | 51.078 ms | 51.004 ms | 59.102 ms | 1.354 ms | 1/64 | 1.0x |
| native UDP | 51.862 ms | 51.108 ms | 50.998 ms | 59.143 ms | 1.365 ms | 1/64 | 1.0x |
| WBD normal | 51.857 ms | 51.104 ms | 51.053 ms | 59.167 ms | 1.444 ms | 1/64 | 1.0x |
| WBD Auto | 51.768 ms | 50.989 ms | 50.881 ms | 59.065 ms | 1.399 ms | 1/64 | 1.0x |

The same host pause affected all four concurrent modes near the tail. Once that explicitly reported common-mode event is excluded, WBD normal differs from native TCP in mean by about **0.027 ms**, and Auto is about **0.089 ms lower** in this run. Those tiny differences are scheduling noise; the important result is that WBD does not add a second RTT or a protection/ACK wait.

## Repeated gate stability

The 16-sample gate was run five consecutive times before adding receiver delivery to the server path, and three consecutive times after adding `session.Receiver.AcceptData`; all passed. Typical median per-sample excess was roughly 1-2 ms for every mode. Auto remained at 1.0x.

The automated gate requires:

1. seeded target mean RTT remains 48-52 ms;
2. p50 remains consistent with the 40-60 ms schedule;
3. at least half of samples are non-host-outliers;
4. WBD normal/Auto median excess is no more than 2 ms above native TCP in the same concurrent run;
5. Auto finishes at 1.0x.

## Qualification commands

The local sandbox passed:

```text
go vet ./...
go test ./... -count=1
go test -race ./... -count=1
go test ./internal/protocol -run='^$' -fuzz=FuzzUnmarshalFrame -fuzztime=2s
Linux/386 benchmark cross-build
Linux/mipsle benchmark cross-build
Windows/amd64 benchmark cross-build
Linux/mipsle wbd-rtt cross-build
Windows/amd64 wbd-rtt cross-build
python scripts/verify_handoff.py
python -m unittest discover -s tests -p 'test_*.py' -v
```

Protocol fuzz executed about 138,710 cases in the recorded qualification run.

## Next gate

Do not tune weak-network RBC/FEC yet. The next step is still M10-003, but only after this normal gate stays green:

1. add a real two-lane userspace fault path with a precisely defined RTT schedule;
2. start with a small impairment step (for example 50 ms RTT / ~1%);
3. then test 80-150 ms / ~2%;
4. only then run 150-300 ms / 10% and 20%, correlated burst impairment, and the 250-600 ms / ~30% degraded case;
5. route the pinned local QUIC oracle through an equivalent controlled fault path before M11 policy tuning.
