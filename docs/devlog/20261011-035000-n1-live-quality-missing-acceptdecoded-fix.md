# N1 live control candidate compile FAIL; missing authenticated mailbox method fixed

Exact failed SOURCE: `af74c18b3313b0cf4e5acc369e7aa6ddbc67446a`. ALL six triggered GitHub Actions concluded **FAIL** before validating the new runtime:
- [next-lifecycle #38073834018](https://github.com/lly8666/wobuzhidao/actions/runs/38073834018) FAIL (official entries compilation).
- [next-foundation #38073833955](https://github.com/lly8666/wobuzhidao/actions/runs/38073833955) FAIL (both Linux and Windows active Go compilation).
- [next-adaptive-n1-codec #38073833994](https://github.com/lly8666/wobuzhidao/actions/runs/38073833994) FAIL (N1 production-runtime race package compilation).
- [next-adaptive-n0-multi #38073834004](https://github.com/lly8666/wobuzhidao/actions/runs/38073834004) FAIL (official Linux binaries build before running native nine-netns).
- [next-adaptive-n0-samewire #38073834041](https://github.com/lly8666/wobuzhidao/actions/runs/38073834041) FAIL (runtime Go focused race build).
- [next-adaptive-n0-negotiated #38073834029](https://github.com/lly8666/wobuzhidao/actions/runs/38073834029) FAIL (three-client negotiation Go focused race build).

**One identical root cause in the raw logs**:
`internal/runtimeowner/quality.go:111:25: mailbox.AcceptDecoded undefined (type *datapath.QualityFeedbackMailbox has no field or method AcceptDecoded)`.
The original mailbox read the 104-byte plaintext within `Accept`; when wiring authenticated receive, a decoded report API was intended but omitted from the committed file. The root cause is a **source compile defect**, not WAN capacity or runner timing. None of the six Action failures qualify real N1 live behavior, and prior PASS on different SOURCE does not migrate to this SHA.

Minimal fix in `internal/datapath/quality_feedback_v3.go`: `Accept` now strictly decodes byte input and delegates to `AcceptDecoded`, which revalidates the structure and uses the exact same local-lane-ref/shared authenticated nonce/pinned *remote* generation/report sequence replay gates. The production Lane decoder calls `AcceptDecoded` only after authenticated KindHealth and 104-byte strict decode, avoiding pointless encode→decode at receipt. No change to 104B format, PN, encryption, counters, tick interval or N0 business routing. Caller must never use decoded API for unauthenticated traffic.

Fix SOURCE = containing commit; all six original FAIL receipts retained in sole `STATUS.json` and companion evidence. Automatic branch/path Actions must rerun and be checked individually; `N1 IN_PROGRESS` and no performance/native Windows/physical/AES/auto claims. No local tests, no host deployment, no mainline merge. User's Actions-first, final-one-physical-phase rule applies.
