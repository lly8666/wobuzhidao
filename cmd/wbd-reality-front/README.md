# wbd-reality-front

`wbd-reality-front` is the Reality-like TLS 1.3 bootstrap front used by the WBD client and server paths. Recognized WBD ClientHello sessions are taken over on the same transport association; unrecognized sessions fall back to the configured target. Sustained VPN payload does not use this bootstrap stream.

The command remains part of the shared Windows portable, Linux server, and reconnect/netem qualification surface. Changes under this directory therefore require exact-source producer and transport qualification rather than mixed-SHA artifact reuse.
