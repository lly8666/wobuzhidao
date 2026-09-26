#!/usr/bin/env python3
import argparse
import json
import os
import pathlib
import re
import signal
import socket
import subprocess
import sys
import threading
import time


def run(cmd, *, check=True, capture=False, timeout=None):
    kwargs = {"text": True, "timeout": timeout}
    if capture:
        kwargs.update(stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    p = subprocess.run(cmd, **kwargs)
    if check and p.returncode != 0:
        raise RuntimeError(f"command failed rc={p.returncode}: {' '.join(cmd)}\n{p.stdout if capture else ''}")
    return p.stdout if capture else ""


def ns(name, *cmd):
    return ["ip", "netns", "exec", name, *map(str, cmd)]


def wait_text(path, needle, timeout=20.0, count=1):
    end = time.time() + timeout
    p = pathlib.Path(path)
    while time.time() < end:
        try:
            text = p.read_text(errors="replace")
        except FileNotFoundError:
            text = ""
        if text.count(needle) >= count:
            return text
        time.sleep(0.05)
    raise RuntimeError(f"timeout waiting for {needle!r} x{count} in {path}")


def terminate(p, timeout=2.0):
    if p is None or p.poll() is not None:
        return
    try:
        p.send_signal(signal.SIGTERM)
        p.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        p.kill()
        p.wait(timeout=1)


def parse_faketcp_stats(path):
    try:
        lines = pathlib.Path(path).read_text(errors="replace").splitlines()
    except FileNotFoundError:
        return None
    for line in reversed(lines):
        if line.startswith("WBD_FAKETCP_STATS "):
            try:
                return json.loads(line.split(" ", 1)[1])
            except json.JSONDecodeError:
                return None
    return None


def read_proc_table():
    out = {}
    page_kib = os.sysconf("SC_PAGE_SIZE") // 1024
    for name in os.listdir("/proc"):
        if not name.isdigit():
            continue
        pid = int(name)
        try:
            raw = pathlib.Path(f"/proc/{pid}/stat").read_text()
            close = raw.rfind(")")
            rest = raw[close + 2 :].split()
            ppid = int(rest[1])
            ticks = int(rest[11]) + int(rest[12])
            statm = pathlib.Path(f"/proc/{pid}/statm").read_text().split()
            rss_kib = int(statm[1]) * page_kib
            out[pid] = (ppid, ticks, rss_kib)
        except (FileNotFoundError, ProcessLookupError, PermissionError, ValueError, IndexError):
            continue
    return out


def tree_members(table, roots):
    members = {int(x) for x in roots if int(x) in table}
    changed = True
    while changed:
        changed = False
        for pid, (ppid, _, _) in table.items():
            if pid not in members and ppid in members:
                members.add(pid)
                changed = True
    return members


class ResourceSampler:
    def __init__(self, roots):
        self.roots = list(roots)
        self.stop_evt = threading.Event()
        self.peak_rss_kib = 0
        self.start_ticks = 0
        self.end_ticks = 0
        self.thread = None
        self.hz = os.sysconf(os.sysconf_names["SC_CLK_TCK"])

    def _snapshot(self):
        table = read_proc_table()
        members = tree_members(table, self.roots)
        ticks = sum(table[p][1] for p in members if p in table)
        rss = sum(table[p][2] for p in members if p in table)
        return ticks, rss, sorted(members)

    def start(self):
        self.start_ticks, rss, _ = self._snapshot()
        self.peak_rss_kib = rss
        self.thread = threading.Thread(target=self._loop, daemon=True)
        self.thread.start()

    def _loop(self):
        while not self.stop_evt.wait(0.05):
            ticks, rss, _ = self._snapshot()
            self.end_ticks = max(self.end_ticks, ticks)
            self.peak_rss_kib = max(self.peak_rss_kib, rss)

    def finish(self):
        self.stop_evt.set()
        if self.thread:
            self.thread.join(timeout=1)
        ticks, rss, members = self._snapshot()
        self.end_ticks = max(self.end_ticks, ticks)
        self.peak_rss_kib = max(self.peak_rss_kib, rss)
        return {
            "cpu_seconds": max(0.0, (self.end_ticks - self.start_ticks) / float(self.hz)),
            "peak_rss_kib": self.peak_rss_kib,
            "sampled_pids": members,
        }


def qdisc_stats(ns_name, dev):
    text = run(ns(ns_name, "tc", "-s", "qdisc", "show", "dev", dev), capture=True)
    m = re.search(r"Sent\s+(\d+)\s+bytes\s+(\d+)\s+pkt\s+\(dropped\s+(\d+)", text)
    return {
        "bytes": int(m.group(1)) if m else None,
        "packets": int(m.group(2)) if m else None,
        "dropped": int(m.group(3)) if m else None,
        "raw": text,
    }


def meter_main(args):
    listen_host, listen_port = args.listen.rsplit(":", 1)
    target_host, target_port = args.target.rsplit(":", 1)
    target = (target_host, int(target_port))
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.bind((listen_host, int(listen_port)))
    s.settimeout(0.2)
    peer = None
    st = {
        "out_packets": 0,
        "out_bytes": 0,
        "control_packets": 0,
        "control_bytes": 0,
        "data_packets": 0,
        "data_bytes": 0,
        "fec_systematic_packets": 0,
        "fec_systematic_bytes": 0,
        "fec_repair_packets": 0,
        "fec_repair_bytes": 0,
        "in_packets": 0,
        "in_bytes": 0,
    }
    stop = False

    def on_signal(_sig, _frame):
        nonlocal stop
        stop = True

    signal.signal(signal.SIGTERM, on_signal)
    signal.signal(signal.SIGINT, on_signal)
    while not stop:
        try:
            data, addr = s.recvfrom(65535)
        except socket.timeout:
            continue
        if addr == target:
            st["in_packets"] += 1
            st["in_bytes"] += len(data)
            if peer is not None:
                s.sendto(data, peer)
            continue
        if peer is None:
            peer = addr
        if addr != peer:
            continue
        st["out_packets"] += 1
        st["out_bytes"] += len(data)
        if data.startswith(b"WBDC"):
            st["control_packets"] += 1
            st["control_bytes"] += len(data)
        else:
            st["data_packets"] += 1
            st["data_bytes"] += len(data)
            if len(data) >= 56 and data[:2] == b"WF":
                if data[8] < 20:
                    st["fec_systematic_packets"] += 1
                    st["fec_systematic_bytes"] += len(data)
                else:
                    st["fec_repair_packets"] += 1
                    st["fec_repair_bytes"] += len(data)
        s.sendto(data, target)
    pathlib.Path(args.out).write_text(json.dumps(st, sort_keys=True, indent=2) + "\n")
    return 0


def benchmark_main(args):
    if os.geteuid() != 0:
        raise SystemExit("benchmark run must execute as root")
    assets = pathlib.Path(args.assets)
    out = pathlib.Path(args.out_dir)
    out.mkdir(parents=True, exist_ok=True)
    tickets = out / "tickets"
    tickets.mkdir(mode=0o700, exist_ok=True)
    tag = f"wbdml{os.getpid()}"
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
        front, raw, link, service = 40443, 40000, 47000, 48000
        probe_out = out / "probe.json"
        probe, probe_log = start("probe-server", S, [str(assets / "udp_multistream_oneway_probe"), "server", str(service), str(args.duration), "6000", "2", str(probe_out)])
        frontp, front_log = start("front-server", S, [str(assets / "wbd-reality-front"), "server", "-listen", f"10.92.0.10:{front}", "-target", "127.0.0.1:9", "-server-name", target, "-cert", str(assets / "front.pem"), "-key", str(assets / "front.key"), "-route-key", route_key, "-username", "solo", "-password", "shared-password", "-ticket-dir", str(tickets)])
        wait_text(front_log, "WBD_REALITY_FRONT_READY", 15)

        for namespace, suffix in ((A, "a"), (B, "b")):
            log = out / f"front-{suffix}.log"
            with open(log, "w") as fh:
                p = subprocess.run(ns(namespace, str(assets / "wbd-reality-front"), "client", "-addr", f"10.92.0.10:{front}", "-server-name", target, "-route-key", route_key, "-username", "solo", "-password", "shared-password", "-verify-server=false", "-ticket-out", str(out / f"ticket-{suffix}.txt")), stdout=fh, stderr=subprocess.STDOUT, text=True, timeout=35)
            if p.returncode != 0:
                raise RuntimeError(f"Reality client {suffix} failed")
            wait_text(log, "WBD_REALITY_FRONT_OK", 1)
        ta = (out / "ticket-a.txt").read_text().strip()
        tb = (out / "ticket-b.txt").read_text().strip()
        if len(ta) != 64 or len(tb) != 64 or ta == tb:
            raise RuntimeError("invalid or duplicate front tickets")

        linkp, link_log = start("link-server", S, [str(assets / "wbd-link-server-mux"), "-listen", f"127.0.0.1:{link}", "-service", f"127.0.0.1:{service}", "-ticket-dir", str(tickets), "-ticket-ttl", "60s", "-idle-timeout", "30s", "-max-sessions", "8"])
        wait_text(link_log, "WBD_LINK_SERVER_MUX_READY", 10)
        muxp, mux_log = start("faketcp-mux", S, [str(assets / "wbd-faketcp-mux"), "server", "--listen", f"10.92.0.10:{raw}", "--dtls-shim", str(assets / "wbd_dtls_shim"), "--link-target", f"127.0.0.1:{link}", "--cert", str(assets / "dtls.pem"), "--key", str(assets / "dtls.key"), "--max-sessions", "8"])
        wait_text(mux_log, "READY role=server-mux", 10)

        fca, fca_log = start("faketcp-a", A, [str(assets / "wbd-faketcp"), "client", "--local-udp", "127.0.0.1:45101", "--source", "10.92.0.2:41001", "--remote", f"10.92.0.10:{raw}"])
        fcb, fcb_log = start("faketcp-b", B, [str(assets / "wbd-faketcp"), "client", "--local-udp", "127.0.0.1:45102", "--source", "10.92.0.6:41002", "--remote", f"10.92.0.10:{raw}"])
        wait_text(fca_log, "READY role=client", 25)
        wait_text(fcb_log, "READY role=client", 25)

        da, da_log = start("dtls-a", A, [str(assets / "wbd_dtls_shim"), "client", "46101", "127.0.0.1", "45101", "none", "none"])
        db, db_log = start("dtls-b", B, [str(assets / "wbd_dtls_shim"), "client", "46102", "127.0.0.1", "45102", "none", "none"])
        wait_text(da_log, "READY role=client version=DTLSv1.3", 35)
        wait_text(db_log, "READY role=client version=DTLSv1.3", 35)
        wait_text(mux_log, "BOUND role=server", 35, count=2)

        meter_a_json = out / "meter-a.json"
        meter_b_json = out / "meter-b.json"
        ma, ma_log = start("meter-a", A, [sys.executable, str(pathlib.Path(__file__).resolve()), "meter", "--listen", "127.0.0.1:46201", "--target", "127.0.0.1:46101", "--out", str(meter_a_json)])
        mb, mb_log = start("meter-b", B, [sys.executable, str(pathlib.Path(__file__).resolve()), "meter", "--listen", "127.0.0.1:46202", "--target", "127.0.0.1:46102", "--out", str(meter_b_json)])
        time.sleep(0.1)

        la, la_log = start("link-a", A, [str(assets / "wbd-link-proxy"), "-mode", "client", "-listen", "127.0.0.1:47101", "-dtls", "127.0.0.1:46201", "-fec", args.fec, "-demo-reality-ticket", ta])
        lb, lb_log = start("link-b", B, [str(assets / "wbd-link-proxy"), "-mode", "client", "-listen", "127.0.0.1:47102", "-dtls", "127.0.0.1:46202", "-fec", args.fec, "-demo-reality-ticket", tb])
        wait_text(la_log, "WBD_LINK_READY role=client", 35)
        wait_text(lb_log, "WBD_LINK_READY role=client", 35)
        wait_text(link_log, "WBD_LINK_MUX_SESSION_READY account=solo", 35, count=2)
        if any(tickets.iterdir()):
            raise RuntimeError("one-time tickets were not both consumed")

        # Reset impairment counters immediately before the offered interval.
        time.sleep(0.2)
        for dev in ("rs", "ra", "rb"):
            run(ns(R, "tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "50000", "delay", f"{half}ms", "loss", "random", f"{args.loss}%", "rate", "100mbit"))

        sampler = ResourceSampler([linkp.pid, muxp.pid, fca.pid, fcb.pid, da.pid, db.pid, la.pid, lb.pid])
        sampler.start()
        ca, ca_log = start("probe-a", A, [str(assets / "udp_multistream_oneway_probe"), "client", "47101", "1", str(args.offered_mbps_per_stream), str(args.duration), "1200"])
        cb, cb_log = start("probe-b", B, [str(assets / "udp_multistream_oneway_probe"), "client", "47102", "2", str(args.offered_mbps_per_stream), str(args.duration), "1200"])
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
        print("MUX_LOAD_100M", json.dumps(result, sort_keys=True))
        if probe_json.get("observed_streams") != 2:
            raise RuntimeError("receiver did not observe both live sessions")
        if any(int(x.get("delivered", 0)) == 0 for x in probe_json.get("streams", [])):
            raise RuntimeError("a live session delivered zero application datagrams")
        return 0
    finally:
        cleanup()


def main():
    p = argparse.ArgumentParser()
    sub = p.add_subparsers(dest="cmd", required=True)
    m = sub.add_parser("meter")
    m.add_argument("--listen", required=True)
    m.add_argument("--target", required=True)
    m.add_argument("--out", required=True)
    b = sub.add_parser("run")
    b.add_argument("--rtt", type=float, required=True)
    b.add_argument("--loss", type=float, default=20.0)
    b.add_argument("--fec", choices=["off", "20:20"], required=True)
    b.add_argument("--offered-mbps-per-stream", type=float, default=20.0)
    b.add_argument("--duration", type=float, default=2.0)
    b.add_argument("--assets", required=True)
    b.add_argument("--out-dir", required=True)
    args = p.parse_args()
    if args.cmd == "meter":
        return meter_main(args)
    return benchmark_main(args)


if __name__ == "__main__":
    raise SystemExit(main())
