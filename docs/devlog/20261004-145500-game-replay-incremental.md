# Profile-guided Game replay retirement

CPU diagnostic SOURCE 697a4189bdd49a5cf11bf53cdb402ad0f730605b, run37180476979 PASS (one Game5205, AMD EPYC7763, 120s workload). Compact ZIP SHA256 bde88882962115df6194b3a779acf526b332a90f2d22cd1aacd66c0908dcd110. First diagnostic37179961433 remains failed before workload: unbound TUN poller bug, fixed in697a418.

Client/server CPU profile: gamelane.Decoder.evictOld cumulative14.36%/14.46%; mapiternext flat12.32%/12.49%. Every new highest PacketID scanned full replay map. This is a concrete longstanding Game hotspot, not evidence newly introduced by IPv4 routing. Original old/new steady packet volumes same, receive calls fewer on newer; Intel8573C versus AMD7763 means historical67.84/62.89 versus105.37/98.92 CPU-s cannot be attributed to code alone.

Change: retire only numeric IDs newly leaving window. One consecutive arrival deletes one old ID; very large jumps clear bounded retained map, sparse gaps scan only actual retained entries if cheaper. Keep exact old window, first-arrival, stale/duplicate/no-HOL and payload ownership. Added randomized semantic oracle with non-power-of-two windows, huge jumps and uint64 upper boundary. No wire/replay-window/FEC/repair change.

DNS SOURCE f0b75af: foundation37180655521, targeted37180655532, lifecycle37180655600, real split37180655707, real default/custom/off DNS+IPv6+cleanup37180655605 all PASS. Follow-up here rejects resolver=underlay, checks Linux IPv6 ownership/cleanup in privileged test and extends Windows mock to owned DNS/firewall rollback with foreign preservation. Physical driver/system NRPT timeout remains NOT_RUN.

Required next: exact-source core/race plus independent Normal10/Game4-3M5205 without profile; optional one separate profiled Game confirms hotspot removal. Functional and historical qualifications do not replace new performance gates. CPU hardware differences report separately; never discard failed or capacity-limited samples.
