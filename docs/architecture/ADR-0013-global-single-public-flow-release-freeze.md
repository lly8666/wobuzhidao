# ADR-0013: Global single-public-flow release freeze

Status: **HISTORICAL / SUPERSEDED BY ADR-0014 — 2026-08-31**

## Historical context

ADR-0013 was an earlier attempt to freeze one public transport for the whole connected Logical Tunnel. It was later withdrawn while multipath work resumed. The product owner has now issued a final, more specific requirement captured in ADR-0014:

- exactly one public TCP-shaped WBD connection lineage at a time for one connected Logical Tunnel;
- Reality-like real TLS 1.3 is carried inside that FakeTCP association from the first SYN lineage;
- no preliminary ordinary kernel-TCP Reality connection;
- no FIN/RST/new WBD payload SYN at the bootstrap barrier;
- sustained DTLS/FEC/VPN payload remains datagram-oriented and free of ordinary kernel-TCP HOL;
- no simultaneous second public lane / Game multipath / make-before-break public overlap.

ADR-0014, not this historical ADR, is the current authority.

## Evidence retained from ADR-0013

The single-flow implementation and tests developed during this period remain valuable evidence, especially:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable Reality-like real TLS 1.3 bootstrap
  -> explicit same-flow barrier
  -> DTLS 1.3
  -> LINK
  -> FEC
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

Npcap filtering fixes, same-association bootstrap tests and post-switch no-HOL tests remain part of current qualification where they match ADR-0014.

See `docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md` for the controlling release contract.