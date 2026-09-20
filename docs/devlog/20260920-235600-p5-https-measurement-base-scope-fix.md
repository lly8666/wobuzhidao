# P5 HTTPS measurement base scope-fix candidate

- Date: 2026-09-20
- Branch authority: `next/tlslike-dataplane`
- Parent SOURCE_SHA: `74f1b602bc664446f34d5a8c42e5a4d21391439e`
- Failed Actions preserved: `35516466731`
- Previous product qualification remains: P4 `76a20c865bf86edf2098f0ef40c44a31298679dd` / Actions `35510241639` / 7/7 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Second-candidate evidence

Actions `35516466731` completed with 7/8 jobs PASS. Repository contract, Windows and Ubuntu active-go-tests including Linux race, P2 kernel fallback, Linux shared-TUN iptables/nft and OpenWrt privileged all passed. Only the new P5 measurement job failed.

Replacing the original `net.Pipe` fixture with a real loopback TCP pair removed the first fixture ambiguity, but normal HTTP `Connection: close` on the first inner HTTPS connection exposed a different existing lifecycle path: a late frame arrived after the platformflow server flow had been removed, causing `platformflow: malformed frame: unknown TCP server flow`. The server runtime then terminated and the second HTTPS connection observed a TCP reset. The failed raw-job artifact is `10606893150`, zip digest `sha256:24b65c77841ad14b12873bca98d644f72bc9dd09d48a6c75fe11b41807be4b34`.

This is useful evidence, but repairing platformflow close-tail/reliability semantics would widen the first P5 measurement-base atom into a transport lifecycle change. The first atom is therefore kept narrow rather than silently redesigning reliability.

## Narrow scope fix

The third candidate changes only measurement-harness behavior and artifact plumbing:

- each P5 business flow is still a distinct real loopback TCP connection with an independent real TLS handshake and HTTP request;
- the first and second HTTPS connections remain open until both measurements are complete, so the atom measures “first HTTPS flow on the initial outer association” and “subsequent independent HTTPS flow on the already-established lane” without also qualifying inner close-before-next behavior;
- there is no `Connection: close` request header and no early TLS close in the measured interval;
- test cleanup closes the application sockets after runtimeentry/server measurement cleanup;
- `WBD_P5_ARTIFACT_DIR` is made absolute under `${{ github.workspace }}/artifacts/p5-https-base`, because Go tests run with the package directory as their working directory.

The prior close-tail failure remains recorded. This candidate does **not** claim that close-before-next lifecycle is fixed or qualified.

## Atom boundaries unchanged

- production padding remains 0/off;
- FEC remains off;
- no weak-network injection or parameter matrix;
- no performance conclusion or classifier;
- no P7 Windows/Npcap physical run;
- hosted SegmentIO PCAP is not a physical capture;
- OpenWrt IPv6 remains NOT_IMPLEMENTED;
- no `old/` reuse, so `docs/REUSE_LEDGER.json` is unchanged.

## Qualification

The candidate must pass the exact-SHA full `next-foundation` workflow. The P5 gate must produce and validate the raw PCAP, JSONL events, manifest, summary and test log at the exact `GITHUB_SHA`. Existing gates must also pass on that same SHA.

`docs/STATUS.json.last_tested_source_sha` deliberately remains `76a20c865bf86edf2098f0ef40c44a31298679dd` until the new product candidate earns qualification.
