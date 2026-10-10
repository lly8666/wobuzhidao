# N0 realpath: 407B S2C outer repair in lossless 300ms, functional scope separation

## Verified predecessor
Current work branch next/adaptive-fec-aes-tun-20261010; parent HEAD `5f0ebeb9c964bdeff5e8d6cef282885eeb34ba44`, tree `03d0d15e85c7be69a6abf9cab3d9c8de4680225a`. This log, source, sole STATUS and evidence enter one new commit; its exact SOURCE is its GitHub commit SHA / Actions GITHUB_SHA (self-hash cannot be included here).
- Parent run: https://github.com/lly8666/wobuzhidao/actions/runs/38063769207, job 114247157088, **FAIL**. Build, one-sample guard, topology prerequisites and formal 5-netns real UDP test succeeded. Validator failed only the "unexpected lossless outer repair" assumption, not delivery.
- Observed from run summary: C2S 987/987 packets, 500080/500080 bytes, 0 corrupt/duplicates, S2C 987/987 packets, 500080/500080 bytes, 0 corrupt/duplicates; 8/8 RTT probes p99 600577289 ns. Netem 300022000 ns C2S, 300021000 ns S2C; every tcpdump kernel drop=0; both processes alive after drain. C2S fresh TCP payload 1548866B repair 0; S2C fresh 1562639B repair **407B** (0.026% of S2C fresh). No Wintun, no AES/auto, no new performance capacity claim.
- Controlled netem has 0 loss but one-way 300ms; finite outer repair timer can fire before a delayed ACK. This is not evidence of a corrupt or blocked first delivery and cannot be labeled a failed business receipt. It remains a nonzero wire overhead observation; separate later same-Seq same-wire and p99/CPU qualification must scrutinize it. Neither old historical validator nor prior FAIL is edited.

## Atomic source change
- In **new** tools/adaptive_n0_realpath.py only: remove zero-repair as a *functional* hard gate. Continue enforcing real bidirectional unique payload/hash, no corrupt/duplicates, recorded capture drops, observed 300ms netem, exact versioned config, authenticated formal processes and one sample guard.
- Add exact observed C2S/S2C repair bytes/fresh bytes/ratio to the new receipt, label any repair as not yet qualified for performance. Do not suppress or re-label the observed 407B as zero.
- The existing REALPATH calibration script and analyzer, history, previous data and pcap hashes remain unchanged. No CPU optimization or transport behavior changed by this commit.

## New SOURCE verification and next steps
- New-source one-sample Actions re-run **PENDING** at author time. Failure remains until a GitHub run returns a positive scoped result.
- Formal 3 concurrent isolated-client IPv4 and per-client asymmetric algorithms not run; current test single Normal 20:4 versus server global 20:20. N0 not marked complete. Capacity/PSI preflight may be available as artifact but no performance inference from 8 seconds 0.5Mbps.
- Preserve all previous failures, the deferred ~80s S2C root cause, ~multi-second late, and Windows native limitations. No local Go/test, machine deployment, mainline merge, credential upload or large pcap.
