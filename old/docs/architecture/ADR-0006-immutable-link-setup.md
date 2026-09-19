# ADR-0006: Immutable per-association link setup

Status: **ACCEPTED FOR V2.2 DEVELOPMENT** (2026-08-25)

This ADR supersedes the runtime FEC config-epoch portions of ADR-0005. The fixed-FEC math, account, routing, Persona and later dual-lane decisions in ADR-0005 remain valid unless contradicted here.

## Decision

A WBD data association has one immutable transport configuration chosen during startup. There is no mid-session parameter-control plane.

Conceptual establishment order:

```text
optional TLS Persona preflight
    -> FakeTCP association
    -> DTLS 1.3 association
    -> LINK_INIT(client proposal)
    -> LINK_ACCEPT(server exact acceptance) or ERROR
    -> AUTH / AUTH_OK when configured
    -> Established
```

All link-defining parameters are fixed before the association enters Established. If a client wants different parameters, it closes the association and creates a new one.

## Current LINK_INIT contents

The current wire structure includes:

- protocol version range;
- FEC mode: `off` or `fixed`;
- fixed FEC data/parity geometry;
- fixed FEC scheduler identifier;
- FEC flush interval;
- MTU;
- raw lane count.

The structure is intentionally extensible for future link-defining parameters, but an extension must still obey the one-time setup rule.

Outer FakeTCP parameters that are inherently negotiated by the raw TCP-shaped handshake (for example MSS/SACK/window-scale appearance) remain part of that handshake rather than being duplicated in WBD LINK_INIT. Persona settings needed before DTLS are selected before the Persona preflight. Account credentials are sent during AUTH after LINK_ACCEPT because they authorize the already-negotiated association rather than changing its packet geometry.

## Exact-accept rule

The client owns the requested performance policy. The server owns capability and resource limits.

The server may:

- accept the proposal exactly; or
- reject it with a policy/unsupported error.

The server does **not** silently clamp or rewrite a link proposal. LINK_ACCEPT echoes the exact accepted LinkConfig, and the client rejects an accept whose config differs from its proposal.

This makes captures, bug reports and benchmark receipts reproducible: one association has one configuration known by both endpoints.

## Current admitted live profiles

The protocol format can describe more profiles than the live codec currently supports, but negotiation is capability-gated.

Current live WBD admission:

- FEC off;
- fixed systematic `20:20` tail-RS;
- one raw lane;
- bounded MTU and FEC flush values.

Current rejection:

- Auto FEC;
- fixed `20:10` until the WBD live codec is generalized and qualified;
- micro-block scheduler until implemented live;
- causal/sliding scheduler until implemented live;
- two lanes until the later lane milestone.

The offline FEC simulator may continue to study these rejected values. Simulator support is not wire/runtime admission.

## Immutability after establishment

After LINK_ACCEPT, any second LINK_INIT is invalid. After Established, LINK_INIT, legacy CONFIG, or another attempt to change FEC/MTU/lane parameters returns an unexpected-state error whose product meaning is **reconnect required**.

There is no config epoch, pending config, boundary activation, or old/new coding-window overlap logic in the V2.2 product path.

This intentionally trades a cheap reconnect for a much smaller state machine and a much easier-to-qualify data plane.

## Legacy M3E

The historical M3E one-byte CONFIG/CONFIG_OK codec remains in-tree to preserve earlier evidence and compatibility tests. It is not used by the current product association state machine.

New product work must use LINK_INIT/LINK_ACCEPT and must not add new post-Established transport-parameter frames.

## Consequences

Advantages:

- smaller protocol/state surface;
- no mixed FEC epochs or cross-window ambiguity;
- straightforward packet-capture interpretation;
- easier server multi-session accounting because every session has a stable immutable LinkConfig;
- changing a profile automatically gets a fresh FakeTCP/DTLS/WBD session and clean buffers.

Costs:

- changing FEC or MTU requires reconnect;
- future Auto FEC, if ever developed, would have to choose between reconnect-based adaptation or a new explicitly reviewed architecture. It does not inherit a hidden runtime-control mechanism from V2.2.

## Next implementation gate

1. unit-qualify LINK_INIT/LINK_ACCEPT codec and immutable server state;
2. add client startup use of LINK_INIT before AUTH/application data;
3. wire accepted LinkConfig into the existing live FEC/off path;
4. run end-to-end establishment tests for FEC off and fixed 20:20;
5. verify packet/data behavior remains unchanged after Established and that any parameter change requires a fresh association.
