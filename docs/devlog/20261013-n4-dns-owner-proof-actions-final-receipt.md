# N4 source 9d0c DNS ownership scoped Actions final receipt — 2026-10-13

Working branch `next/adaptive-fec-aes-tun-20261010`. Exact last *code* SOURCE `9d0c2e6c40fd302aed6581366df0c788b1211fd9`; this receipt is a docs-only successor. Old exact SOURCE `c10a0732569af95a2cdabcad2402b457f620496f`.

## Final source-matched Actions
- [N4 matrix 38355293478](https://github.com/lly8666/wobuzhidao/actions/runs/38355293478): `completed/success` including Windows2022, Windows2025, Linux-race and receipt. GitHub artifact11792063029 sha256:c1566b431a2e22710b82f254fc26f0cff56248e6fffc3cbe9507243d6effb4dc.
- [Foundation 38355293359](https://github.com/lly8666/wobuzhidao/actions/runs/38355293359): `completed/success`, seven required jobs Windows/Linux Go, repo contract, P2/P4, P5/P6 extended SKIPPED. Foundation artifact11792032967 sha256:413b38571e237f86d1b5c51b9d7b14333261cbd5c0154ff82e7d7ea72c25df5d.
- [Lifecycle 38355293275](https://github.com/lly8666/wobuzhidao/actions/runs/38355293275): `completed/success`.
- Previous SOURCE c10a: N4 38354764587 success artifact11791879792 sha256:c21e321bd09c33fce23b04c9d32ecc9873faf239911304c01714ff05aeb18b77; Foundation38354764412 success artifact11792126998 sha256:79fbd36581d99e77973424428cff819bfb7894b0618294ca1e0af1a5a52d5124; lifecycle38354764394 success. Hashes are GitHub artifact metadata digests, not plaintext test receipt hashes.

## What passed and hard limits
Source9d0c ensures native DNS Set request cannot proceed solely on persistent guard and arbitrary GUID: it now checks an independently proven live process-created Wintun adapter interface index and the Windows read-only index→GUID mapping. Hosted tests cover missing/borrowed/stale proof and negative request cases, without calling actual Windows DNS setter. Sourcec10a enforces exact bounded /1 host captures with physical server /32 escape placed first in native route batch. Neither source implements a crash-durable native network Apply/Stop transaction; Wintun DHCP/static lease/DAD, MTU, DNS restore, WFP multi-tunnel/exclusive and real administrator driver Apply/Stop/Crash all OPEN/NOT_RUN. Current cmd/wbd-client still launches PowerShell/.ps1 and uses complement CIDRs. No performance/physical qualification, no local Go tests, no deployment, merge or qualification-ref changes.

Next N4 task exactly as STATUS.next_task; N2 C04/C05 efficiency FAIL, C10 GE p99 and ~80s tail OPEN, N6 P6/manifest/hash NOT_STARTED, physical NOT_RUN.
