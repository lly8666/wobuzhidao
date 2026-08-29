#!/usr/bin/env python3
"""Two-session 100 Mbit/s characterization on the V2.3 single public flow.

This deliberately reuses the mature legacy benchmark's measurement helpers,
resource accounting and result schema. Only setup/admission differs: each
client opens exactly one FakeTCP association, performs Reality-like TLS 1.3
bootstrap on that same association, then continues with DTLS/LINK without a
second public connection.
"""

import json
import pathlib
import sys
import time

import bench_mux_two_session_100m as legacy

# The wrapper monkey-patches this symbol to parameterize setup/measurement qdisc
# behavior. main() mirrors it into the legacy helper module so qdisc_stats uses
# the same wrapper too.
run = legacy.run
ns = legacy.ns
wait_text = legacy.wait_text
terminate = legacy.terminate
parse_faketcp_stats = legacy.parse_faketcp_stats
ResourceSampler = legacy.ResourceSampler


def qdisc_stats(ns_name, dev):
    text = run(ns(ns_name, "tc", "-s", "qdisc", "show", "dev", dev), capture=True)
    import re
    m = re.search(r"Sent\s+(\d+)\s+bytes\s+(\d+)\s+pkt\s+\(dropped\s+(\d+)", text)
    return {
        "bytes": int(m.group(1)) if m else None,
        "packets": int(m.group(2)) if m else None,
        "dropped": int(m.group(3)) if m else None,
        "raw": text,
    }


def benchmark_main(args):
    import os
    import subprocess

    if os.geteuid() != 0:
        raise SystemExit("benchmark run must execute as root")
    assets = pathlib.Path(args.assets)
    out = pathlib.Path(args.out_dir)
    out.mkdir(parents=True, exist_ok=True)
    tickets = out / "tickets"
    tickets.mkdir(mode=0o700, exist_ok=True)
    tag = f"wbdsfml{os.getpid()}"
    A, B, R, S = tag + "a", tag + "b", tag + "r", tag + "s"
    procs = []

    def start(label, namespace, command):
        log = out / f"{label}.log"
        fh = open(log, "w")
        p = subprocess.Popen(ns(namespace, *command), stdout=fh, stderr=subprocess.STDOUT, text=True)
        fh.close()
        procs.append(p)
        return p, log

    def cleanup():
        for p in reversed(procs):
            terminate(p, 0.7)
        for n in (A, B, R, S):
            run(["ip", "netns", "del", n], check=False)

    try:
        for n in (A, B, R, S):
            run(["ip", "netns", "add", n])
            run(["ip", "-n", n, "link", "set", "lo", "up"])
        for left, right, nleft, nright in (("a0", "ra", A, R), ("b0", "rb", B, R), ("s0", "rs", S, R)):
            run(["ip", "link", "add", left, "type", "veth", "peer", "name", right])
            run(["ip", "link", "set", left, "netns", nleft])
            run(["ip", "link", "set", right, "netns", nright])
        run(["ip", "-n", A, "addr", "add", "10.92.0.2/30", "dev", "a0"])
        run(["ip", "-n", R, "addr", "add", "10.92.0.1/30", "dev", "ra"])
        run(["ip", "-n", B, "addr", "add", "10.92.0.6/30", "dev", "b0"])
        run(["ip", "-n", R, "addr", "add", "10.92.0.5/30", "dev", "rb"])
        run(["ip", "-n", S, "addr", "add", "10.92.0.10/30", "dev", "s0"])
        run(["ip", "-n", R, "addr", "add", "10.92.0.9/30", "dev", "rs"])
        for n, dev in ((A, "a0"), (B, "b0"), (S, "s0"), (R, "ra"), (R, "rb"), (R, "rs")):
            run(["ip", "-n", n, "link", "set", dev, "up"])
        run(["ip", "-n", A, "route", "add", "10.92.0.8/30", "via", "10.92.0.1"])
        run(["ip", "-n", B, "route", "add", "10.92.0.8/30", "via", "10.92.0.5"])
        run(["ip", "-n", S, "route", "add", "10.92.0.0/29", "via", "10.92.0.9"])
        run(ns(R, "sysctl", "-qw", "net.ipv4.ip_forward=1"))
        for n in (A, B, S):
            run(ns(n, "iptables", "-I", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-j", "DROP"))

        half = args.rtt / 2.0
        for dev in ("rs", "ra", "rb"):
            run(ns(R, "tc", "qdisc", "add", "dev", dev, "root", "netem", "limit", "50000", "delay", f"{half}ms", "loss", "random", f"{args.loss}%", "rate", "100mbit"))

        route_key = "WBD_REALITY_ROUTE_KEY_0123456789abcdef"
        target = "target.example"
        raw, link, service = 443, 47000, 48000
        probe_out = out / "probe.json"
        probe, _ = start("probe-server", S, [str(assets / "udp_multistream_oneway_probe"), "server", str(service), str(args.duration), "6000", "2", str(probe_out)])

        linkp, link_log = start("link-server", S, [str(assets / "wbd-link-server-mux"), "-listen", f"127.0.0.1:{link}", "-service", f"127.0.0.1:{service}", "-ticket-dir", str(tickets), "-ticket-ttl", "60s", "-idle-timeout", "30s", "-max-sessions", "8"])
        wait_text(link_log, "WBD_LINK_SERVER_MUX_READY", 10)

        muxp, mux_log = start("faketcp-mux", S, [
            str(assets / "wbd-faketcp-mux"), "server",
            "--listen", f"10.92.0.10:{raw}",
            "--dtls-shim", str(assets / "wbd_dtls_shim"),
            "--link-target", f"127.0.0.1:{link}",
            "--cert", str(assets / "dtls.pem"), "--key", str(assets / "dtls.key"),
            "--front-cert", str(assets / "front.pem"), "--front-key", str(assets / "front.key"),
            "--server-name", target,
            "--route-key", route_key,
            "--username", "solo", "--password", "shared-password",
            "--ticket-dir", str(tickets),
            "--fallback-target", "127.0.0.1:9",
            "--bootstrap-timeout", "20s",
            "--max-sessions", "8",
        ])
        wait_text(mux_log, "READY role=server-mux", 10)
        wait_text(mux_log, "single_flow_bootstrap=true", 10)

        fca, fca_log = start("faketcp-a", A, [
            str(assets / "wbd-faketcp"), "client",
            "--local-udp", "127.0.0.1:45101",
            "--source", "10.92.0.2:41001", "--remote", f"10.92.0.10:{raw}",
            "--shadow-recovery", "legacy",
            "--reality-server-name", target,
            "--reality-route-key", route_key,
            "--reality-username", "solo", "--reality-password", "shared-password",
            "--reality-ticket-out", str(out / "ticket-a.txt"),
            "--reality-verify-server=false", "--reality-timeout", "20s",
        ])
        fcb, fcb_log = start("faketcp-b", B, [
            str(assets / "wbd-faketcp"), "client",
            "--local-udp", "127.0.0.1:45102",
            "--source", "10.92.0.6:41002", "--remote", f"10.92.0.10:{raw}",
            "--shadow-recovery", "legacy",
            "--reality-server-name", target,
            "--reality-route-key", route_key,
            "--reality-username", "solo", "--reality-password", "shared-password",
            "--reality-ticket-out", str(out / "ticket-b.txt"),
            "--reality-verify-server=false", "--reality-timeout", "20s",
        ])
        wait_text(fca_log, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", 35)
        wait_text(fcb_log, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", 35)
        wait_text(fca_log, "READY role=client", 35)
        wait_text(fcb_log, "READY role=client", 35)
        wait_text(mux_log, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", 35, count=2)

        ta = (out / "ticket-a.txt").read_text().strip()
        tb = (out / "ticket-b.txt").read_text().strip()
        if len(ta) != 64 or len(tb) != 64 or ta == tb:
            raise RuntimeError("invalid or duplicate in-flow Reality tickets")

        da, da_log = start("dtls-a", A, [str(assets / "wbd_dtls_shim"), "client", "46101", "127.0.0.1", "45101", "none", "none"])
        db, db_log = start("dtls-b", B, [str(assets / "wbd_dtls_shim"), "client", "46102", "127.0.0.1", "45102", "none", "none"])
        wait_text(da_log, "READY role=client version=DTLSv1.3", 35)
        wait_text(db_log, "READY role=client version=DTLSv1.3", 35)
        wait_text(mux_log, "BOUND role=server", 35, count=2)

        meter_a_json = out / "meter-a.json"
        meter_b_json = out / "meter-b.json"
        ma, _ = start("meter-a", A, [sys.executable, str(pathlib.Path(legacy.__file__).resolve()), "meter", "--listen", "127.0.0.1:46201", "--target", "127.0.0.1:46101", "--out", str(meter_a_json)])
        mb, _ = start("meter-b", B, [sys.executable, str(pathlib.Path(legacy.__file__).resolve()), "meter", "--listen", "127.0.0.1:46202", "--target", "127.0.0.1:46102", "--out", str(meter_b_json)])
        time.sleep(0.1)

        la, la_log = start("link-a", A, [str(assets / "wbd-link-proxy"), "-mode", "client", "-listen", "127.0.0.1:47101", "-dtls", "127.0.0.1:46201", "-fec", args.fec, "-demo-reality-ticket", ta])
        lb, lb_log = start("link-b", B, [str(assets / "wbd-link-proxy"), "-mode", "client", "-listen", "127.0.0.1:47102", "-dtls", "127.0.0.1:46202", "-fec", args.fec, "-demo-reality-ticket", tb])
        wait_text(la_log, "WBD_LINK_READY role=client", 35)
        wait_text(lb_log, "WBD_LINK_READY role=client", 35)
        wait_text(link_log, "WBD_LINK_MUX_SESSION_READY account=solo", 35, count=2)
        if any(tickets.iterdir()):
            raise RuntimeError("one-time tickets were not both consumed")

        # Only the measured steady-state interval is impaired. Setup remains
        # loss-free in the wrapper so this capacity gate measures post-switch
        # behavior rather than conflating bootstrap survivability with capacity.
        time.sleep(0.2)
        for dev in ("rs", "ra", "rb"):
            run(ns(R, "tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "50000", "delay", f"{half}ms", "loss", "random", f"{args.loss}%", "rate", "100mbit"))

        sampler = ResourceSampler([linkp.pid, muxp.pid, fca.pid, fcb.pid, da.pid, db.pid, la.pid, lb.pid])
        sampler.start()
        ca, _ = start("probe-a", A, [str(assets / "udp_multistream_oneway_probe"), "client", "47101", "1", str(args.offered_mbps_per_stream), str(args.duration), "1200"])
        cb, _ = start("probe-b", B, [str(assets / "udp_multistream_oneway_probe"), "client", "47102", "2", str(args.offered_mbps_per_stream), str(args.duration), "1200"])
        if ca.wait(timeout=args.duration + 4) != 0 or cb.wait(timeout=args.duration + 4) != 0:
            raise RuntimeError("one or both probe clients failed")
        if probe.wait(timeout=args.duration + 9) != 0:
            raise RuntimeError("multistream receiver failed")
        resources = sampler.finish()
        forward_qdisc = qdisc_stats(R, "rs")

        terminate(la); terminate(lb)
        time.sleep(0.15)
        terminate(ma); terminate(mb)
        terminate(fca); terminate(fcb)
        meter_a = json.loads(meter_a_json.read_text())
        meter_b = json.loads(meter_b_json.read_text())
        probe_json = json.loads(probe_out.read_text())
        fstats_a = parse_faketcp_stats(fca_log)
        fstats_b = parse_faketcp_stats(fcb_log)
        retransmit_bytes = 0
        for st in (fstats_a, fstats_b):
            if st and isinstance(st.get("Sender"), dict):
                retransmit_bytes += int(st["Sender"].get("RetransmitBytes", 0))
            elif st and isinstance(st.get("sender"), dict):
                retransmit_bytes += int(st["sender"].get("RetransmitBytes", 0))

        meters = [meter_a, meter_b]
        meter_sum = {k: sum(int(m.get(k, 0)) for m in meters) for k in meter_a.keys()}
        planned_inner_bytes = sum(int(x["planned"]) * int(x["packet_size"]) for x in probe_json.get("streams", []))
        result = {
            "link_mbps": 100,
            "rtt_ms": args.rtt,
            "loss_pct": args.loss,
            "loss_seed_applied": False,
            "fec": args.fec,
            "offered_mbps_per_stream": args.offered_mbps_per_stream,
            "offered_mbps_total": args.offered_mbps_per_stream * 2,
            "duration_s": args.duration,
            "public_flow_model": "single_flow_reality_like_bootstrap_then_dtls",
            "single_flow_bootstrap": True,
            "probe": probe_json,
            "resources": resources,
            "faketcp_client_retransmit_bytes": retransmit_bytes,
            "faketcp_clients": [fstats_a, fstats_b],
            "link_plaintext_meter": {"aggregate": meter_sum, "clients": meters},
            "planned_inner_bytes": planned_inner_bytes,
            "fec_plaintext_expansion": (meter_sum["data_bytes"] / planned_inner_bytes) if planned_inner_bytes else None,
            "forward_qdisc": {k: v for k, v in forward_qdisc.items() if k != "raw"},
        }
        (out / "result.json").write_text(json.dumps(result, sort_keys=True, indent=2) + "\n")
        print("MUX_LOAD_100M_SINGLE_FLOW", json.dumps(result, sort_keys=True))
        if probe_json.get("observed_streams") != 2:
            raise RuntimeError("receiver did not observe both live sessions")
        if any(int(x.get("delivered", 0)) == 0 for x in probe_json.get("streams", [])):
            raise RuntimeError("a live session delivered zero application datagrams")
        return 0
    finally:
        cleanup()


def main():
    # Keep the legacy CLI/meter implementation and only replace the run-mode
    # orchestration. Mirror the wrapper-patched run helper before entering it.
    legacy.run = run
    legacy.benchmark_main = benchmark_main
    return legacy.main()


if __name__ == "__main__":
    raise SystemExit(main())
