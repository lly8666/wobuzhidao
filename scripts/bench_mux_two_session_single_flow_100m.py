#!/usr/bin/env python3
"""100 Mbit/s characterization over one public WBD FakeTCP association.

The product contract under ADR-0014 allows exactly one active public WBD
transport for a connected Logical Tunnel. Capacity is therefore measured with
one FakeTCP -> Reality-like TLS bootstrap -> DTLS -> LINK association carrying
two independent inner application streams. The two streams preserve the
historical aggregate 40/60/80 Mbit/s offered points without creating a second
public SYN lineage.

This reuses the mature legacy benchmark's measurement helpers, resource
accounting and result schema. Only setup/admission/topology differs.
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
    A, R, S = tag + "a", tag + "r", tag + "s"
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
        for n in (A, R, S):
            run(["ip", "netns", "del", n], check=False)

    try:
        for n in (A, R, S):
            run(["ip", "netns", "add", n])
            run(["ip", "-n", n, "link", "set", "lo", "up"])
        for left, right, nleft, nright in (("a0", "ra", A, R), ("s0", "rs", S, R)):
            run(["ip", "link", "add", left, "type", "veth", "peer", "name", right])
            run(["ip", "link", "set", left, "netns", nleft])
            run(["ip", "link", "set", right, "netns", nright])
        run(["ip", "-n", A, "addr", "add", "10.92.0.2/30", "dev", "a0"])
        run(["ip", "-n", R, "addr", "add", "10.92.0.1/30", "dev", "ra"])
        run(["ip", "-n", S, "addr", "add", "10.92.0.6/30", "dev", "s0"])
        run(["ip", "-n", R, "addr", "add", "10.92.0.5/30", "dev", "rs"])
        for n, dev in ((A, "a0"), (S, "s0"), (R, "ra"), (R, "rs")):
            run(["ip", "-n", n, "link", "set", dev, "up"])
        run(["ip", "-n", A, "route", "add", "10.92.0.4/30", "via", "10.92.0.1"])
        run(["ip", "-n", S, "route", "add", "10.92.0.0/30", "via", "10.92.0.5"])
        run(ns(R, "sysctl", "-qw", "net.ipv4.ip_forward=1"))
        for n in (A, S):
            run(ns(n, "iptables", "-I", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-j", "DROP"))

        half = args.rtt / 2.0
        for dev in ("rs", "ra"):
            run(ns(R, "tc", "qdisc", "add", "dev", dev, "root", "netem", "limit", "50000", "delay", f"{half}ms", "loss", "random", f"{args.loss}%", "rate", "100mbit"))

        route_key = "WBD_REALITY_ROUTE_KEY_0123456789abcdef"
        target = "target.example"
        raw, link, service = 443, 47000, 48000
        probe_out = out / "probe.json"
        probe, _ = start("probe-server", S, [str(assets / "udp_multistream_oneway_probe"), "server", str(service), str(args.duration), "6000", "2", str(probe_out)])

        linkp, link_log = start("link-server", S, [
            str(assets / "wbd-link-server-mux"),
            "-listen", f"127.0.0.1:{link}",
            "-service", f"127.0.0.1:{service}",
            "-ticket-dir", str(tickets),
            "-ticket-ttl", "60s",
            "-idle-timeout", "30s",
            "-max-sessions", "4",
        ])
        wait_text(link_log, "WBD_LINK_SERVER_MUX_READY", 10)

        muxp, mux_log = start("faketcp-mux", S, [
            str(assets / "wbd-faketcp-mux"), "server",
            "--listen", f"10.92.0.6:{raw}",
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
            "--max-sessions", "4",
        ])
        wait_text(mux_log, "READY role=server-mux", 10)
        wait_text(mux_log, "single_flow_bootstrap=true", 10)

        fc, fc_log = start("faketcp", A, [
            str(assets / "wbd-faketcp"), "client",
            "--local-udp", "127.0.0.1:45101",
            "--source", "10.92.0.2:41001", "--remote", f"10.92.0.6:{raw}",
            "--shadow-recovery", "legacy",
            "--reality-server-name", target,
            "--reality-route-key", route_key,
            "--reality-username", "solo", "--reality-password", "shared-password",
            "--reality-ticket-out", str(out / "ticket.txt"),
            "--reality-installation-id", "00112233445566778899aabbccddeeff",
            "--reality-tunnel-config-out", str(out / "tunnel.json"),
            "--reality-verify-server=false", "--reality-timeout", "20s",
        ])
        wait_text(fc_log, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", 35)
        wait_text(fc_log, "READY role=client", 35)
        wait_text(mux_log, "WBD_SINGLE_FLOW_BOOTSTRAP_READY", 35)

        ticket = (out / "ticket.txt").read_text().strip()
        if len(ticket) != 64:
            raise RuntimeError("invalid in-flow Reality ticket")
        tunnel = json.loads((out / "tunnel.json").read_text())
        if len(str(tunnel.get("tunnel_id", ""))) != 32 or not tunnel.get("address4"):
            raise RuntimeError("invalid in-flow Logical Tunnel config")

        dtls, dtls_log = start("dtls", A, [
            str(assets / "wbd_dtls_shim"), "client", "46101", "127.0.0.1", "45101", "none", "none",
        ])
        wait_text(dtls_log, "READY role=client version=DTLSv1.3", 35)
        wait_text(mux_log, "BOUND role=server", 35)

        meter_json = out / "meter.json"
        meter, _ = start("meter", A, [
            sys.executable, str(pathlib.Path(legacy.__file__).resolve()), "meter",
            "--listen", "127.0.0.1:46201",
            "--target", "127.0.0.1:46101",
            "--out", str(meter_json),
        ])
        time.sleep(0.1)

        linkc, linkc_log = start("link-client", A, [
            str(assets / "wbd-link-proxy"),
            "-mode", "client",
            "-listen", "127.0.0.1:47101",
            "-dtls", "127.0.0.1:46201",
            "-fec", args.fec,
            "-demo-reality-ticket", ticket,
        ])
        wait_text(linkc_log, "WBD_LINK_READY role=client", 35)
        wait_text(link_log, "WBD_LINK_MUX_SESSION_READY account=solo", 35)
        if any(tickets.iterdir()):
            raise RuntimeError("one-time ticket was not consumed")

        # Setup is loss-free in the wrapper. Activate requested impairment only
        # after the single public association has crossed the bootstrap barrier
        # and LINK is ready, so this gate characterizes no-HOL steady transport.
        time.sleep(0.2)
        for dev in ("rs", "ra"):
            run(ns(R, "tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "50000", "delay", f"{half}ms", "loss", "random", f"{args.loss}%", "rate", "100mbit"))

        sampler = ResourceSampler([linkp.pid, muxp.pid, fc.pid, dtls.pid, linkc.pid])
        sampler.start()
        c1, _ = start("probe-stream-1", A, [
            str(assets / "udp_multistream_oneway_probe"), "client", "47101", "1",
            str(args.offered_mbps_per_stream), str(args.duration), "1200",
        ])
        c2, _ = start("probe-stream-2", A, [
            str(assets / "udp_multistream_oneway_probe"), "client", "47101", "2",
            str(args.offered_mbps_per_stream), str(args.duration), "1200",
        ])
        if c1.wait(timeout=args.duration + 4) != 0 or c2.wait(timeout=args.duration + 4) != 0:
            raise RuntimeError("one or both inner probe streams failed")
        if probe.wait(timeout=args.duration + 9) != 0:
            raise RuntimeError("multistream receiver failed")
        resources = sampler.finish()
        forward_qdisc = qdisc_stats(R, "rs")

        terminate(linkc)
        time.sleep(0.15)
        terminate(meter)
        terminate(fc)
        meter_result = json.loads(meter_json.read_text())
        probe_json = json.loads(probe_out.read_text())
        fstats = parse_faketcp_stats(fc_log)
        retransmit_bytes = 0
        if fstats and isinstance(fstats.get("Sender"), dict):
            retransmit_bytes = int(fstats["Sender"].get("RetransmitBytes", 0))
        elif fstats and isinstance(fstats.get("sender"), dict):
            retransmit_bytes = int(fstats["sender"].get("RetransmitBytes", 0))

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
            "public_flow_model": "one_public_faketcp_flow_two_inner_streams",
            "public_flow_count": 1,
            "inner_stream_count": 2,
            "single_flow_bootstrap": True,
            "logical_tunnel": True,
            "probe": probe_json,
            "resources": resources,
            "faketcp_client_retransmit_bytes": retransmit_bytes,
            "faketcp_clients": [fstats],
            "link_plaintext_meter": {"aggregate": meter_result, "clients": [meter_result]},
            "planned_inner_bytes": planned_inner_bytes,
            "fec_plaintext_expansion": (int(meter_result.get("data_bytes", 0)) / planned_inner_bytes) if planned_inner_bytes else None,
            "forward_qdisc": {k: v for k, v in forward_qdisc.items() if k != "raw"},
        }
        (out / "result.json").write_text(json.dumps(result, sort_keys=True, indent=2) + "\n")
        print("MUX_LOAD_100M_SINGLE_PUBLIC_FLOW", json.dumps(result, sort_keys=True))
        if probe_json.get("observed_streams") != 2:
            raise RuntimeError("receiver did not observe both inner streams")
        if any(int(x.get("delivered", 0)) == 0 for x in probe_json.get("streams", [])):
            raise RuntimeError("an inner stream delivered zero application datagrams")
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
