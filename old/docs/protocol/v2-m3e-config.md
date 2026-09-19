# V2-M3E fixed protection-mode negotiation

Status: **historical qualification PASS; product path superseded by ADR-0006 LINK_INIT/LINK_ACCEPT**.

M3E originally added a one-shot `CONFIG/CONFIG_OK` exchange after the base WBD session had already reached Established. That evidence remains useful for codec/backward-compatibility regression, but the current V2.2 product architecture no longer permits post-Established transport-parameter changes.

## Historical wire extension

The M3A-D `WBDC` envelope reserves:

- type `9`: `CONFIG`, exactly one mode byte;
- type `10`: `CONFIG_OK`, exactly one echoed mode byte.

`internal/control` retains `MarshalExtended` / `UnmarshalExtended` and `ConfigServerSession` so earlier tests/evidence remain reproducible.

Historical values:

- `1` = `normal`;
- `2` = `weak-1.5x` (`20:10` reference);
- `3` = `weak-2x` (`20:20` reference);
- `4` = reserved `auto`, deliberately rejected.

## Current product rule

ADR-0006 replaces M3E product semantics with immutable startup negotiation:

```text
FakeTCP -> DTLS 1.3 -> LINK_INIT -> LINK_ACCEPT -> AUTH -> Established
```

All link-defining parameters are supplied before Established. The server accepts the exact proposal or rejects it. Once Established, changing FEC/MTU/lane/scheduler parameters requires a new association.

Therefore current product code must not use `ConfigServerSession` to change a live association. A post-setup `CONFIG` received through the new `LinkServerSession` is rejected with reconnect-required semantics.

## Historical qualification retained

The original M3E unit/direct-DTLS evidence remains retained as historical regression coverage: fixed mode round trips, Auto/unknown rejection, state ordering and encoded-wire counters.

This retained evidence does **not** override the newer immutable-link architecture. New product integration tests target LINK_INIT/LINK_ACCEPT instead.
