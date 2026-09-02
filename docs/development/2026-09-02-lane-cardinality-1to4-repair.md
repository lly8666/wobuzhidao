# 2026-09-02 Product lane cardinality repair: 1..4

## Scope

Atomic repair only. This phase restores the ADR-0012 product cardinality contract and intentionally does **not** restore candidate/make-before-break orchestration, DORMANT/wake, generation fencing or replacement triggers yet.

Mature FakeTCP/DTLS/LINK/FEC wire code was not changed.

## Starting drift

On live PR #9 after the authority/docs repair, `internal/logicaltunnel/logicaltunnel.go` still contained the global-one-lane rollback:

```text
MinProductPublicTransportLanes = 1
MaxProductPublicTransportLanes = 1
ValidateProductTransportLaneCount: n must equal 1
```

Its direct unit test also rejected 2, 3 and 4 and named ADR-0015 as the cardinality authority.

This contradicted ADR-0012, which requires product active-lane bounds 1..4.

## Change

Code commit:

```text
5140e4bf9b795ec754ff8f7587497896b83aae6f  logicaltunnel: restore product lane range 1..4
```

The contract is now:

```text
MinProductPublicTransportLanes = 1
MaxProductPublicTransportLanes = 4
ValidateProductTransportLaneCount(n):
  accept 1,2,3,4
  reject n < 1
  reject n > 4
```

`ErrTransportLanes` now describes the `1..4` product range instead of exactly one public transport.

Direct test commit:

```text
259c2dfc45132e547d26dc94207d9ed53e6b678e  test: cover product lane range 1..4
```

The unit contract explicitly accepts 1/2/3/4 and rejects -1/0/5/8.

## Preserved code

The lease allocator, stable same-installation reacquire behavior, release semantics and `ValidateIPv4Source` anti-spoof implementation in `internal/logicaltunnel` were deliberately left unchanged.

No change was made to:

- FakeTCP packet/wire/recovery;
- Reality-like per-lane bootstrap;
- wolfSSL DTLS 1.3;
- LINK;
- FEC;
- Windows candidate lane behavior;
- server lane admission;
- DORMANT/wake;
- replacement triggers.

## Qualification state at log creation

Exact code/test HEAD:

```text
259c2dfc45132e547d26dc94207d9ed53e6b678e
```

GitHub Actions for that HEAD were observed as queued/pending at the first refresh after the test commit. Therefore:

```text
Product lane cardinality 1..4:
IMPLEMENTED: yes
AUTOMATED-GREEN: no / pending exact-head CI
PHYSICAL-GREEN: no
RELEASE-QUALIFIED: no
```

Skipped/path-filtered workflows are not counted green.

## Next drift to audit

After recording handoff, audit shipping configuration/runtime/server code for remaining global-one-lane guards, especially:

- `internal/windowsgui/config.go` / profile `Lanes` validation;
- `internal/windowsruntime/dynamic_lane.go` and candidate-lane rejection;
- `internal/windowsruntime/candidate_lane.go`;
- server Logical Tunnel transport admission;
- release-contract tests that require second-lane rejection.

Do not combine all of those into one change. The next atomic repair should restore one bounded contract at a time, with a direct test and a handoff update.
