#!/usr/bin/env python3
"""Actions-only real Linux netns TCP/UDP socket MTU/loss fixture.

Independent single scenario per workflow job. This does not run Windows Wintun
or a complete WBD tunnel; WBD LINK/FEC coverage is a separate matrix scenario.
All received application payloads are checked byte-for-byte without logging them.
"""
import argparse
import errno
import hashlib
import json
import os
from pathlib import Path
import socket
import struct
import subprocess
import sys
import tempfile
import time

A, B = "198.18.44.1", "198.18.44.2"
PORT = 28641
PMTUDISC_DONT, PMTUDISC_DO = 0, 2
INNER_BY_FEC = {
    0: {1300: 1229, 1400: 1329},
    20: {1300: 1173, 1400: 1273},
}  # auto record=outer-40; LINK frame = record-31-(56 if FEC else 0)


def checked(cmd, timeout=55):
    return subprocess.run(cmd, check=True, timeout=timeout, text=True, capture_output=True)


def body(seq, size):
    if size < 8:
        raise AssertionError("size must contain sequence header")
    return b"WBDM" + struct.pack("!I", seq) + bytes((seq * 17 + i) % 256 for i in range(size - 8))


def recv_exact(sock, count):
    out = bytearray()
    while len(out) < count:
        chunk = sock.recv(count - len(out))
        if not chunk:
            raise AssertionError("TCP stream ended early")
        out.extend(chunk)
    return bytes(out)


def sizes_for(kind, inner):
    if kind == "tcp":
        return [8, 31, 96, 512, inner - 41, inner - 40, inner - 39,
                inner, inner + 1, 1500, 4096, 16384, 65536]
    if kind == "udp":
        base = [8, 32, 64, 96, 512, inner - 29, inner - 28,
                inner - 27, inner + 1, 1500, 4096]
        return base * 8
    if kind == "udp-jumbo":
        return [8936, 8937, 8972, 8973, 9000, 16000, 32768, 65507]
    raise AssertionError(kind)


def tcp_server(args):
    expected = sizes_for("tcp", args.inner)
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        listener.bind((B, PORT))
        listener.listen(1)
        listener.settimeout(48)
        Path(args.ready).write_text("ready")
        with listener.accept()[0] as conn:
            conn.settimeout(48)
            for seq, size in enumerate(expected):
                announced = struct.unpack("!I", recv_exact(conn, 4))[0]
                if announced != size or recv_exact(conn, size) != body(seq, size):
                    raise AssertionError("TCP missing, corrupt or reordered application message")
                conn.sendall(struct.pack("!I", seq))
    Path(args.result).write_text(json.dumps({"received": len(expected), "bytes": sum(expected)}))


def tcp_client(args):
    sizes = sizes_for("tcp", args.inner)
    with socket.create_connection((B, PORT), timeout=48, source_address=(A, 0)) as conn:
        conn.settimeout(48)
        mss = conn.getsockopt(socket.IPPROTO_TCP, socket.TCP_MAXSEG)
        if not 0 < mss <= args.inner - 40:
            raise AssertionError("TCP MSS exceeds inner MTU budget: " + str(mss))
        for seq, size in enumerate(sizes):
            conn.sendall(struct.pack("!I", size) + body(seq, size))
            if struct.unpack("!I", recv_exact(conn, 4))[0] != seq:
                raise AssertionError("TCP acknowledgement mismatch")
    return {"tcp_messages": len(sizes), "tcp_bytes": sum(sizes),
            "mss": mss, "oversize_socket_writes": sum(s > args.inner for s in sizes)}


def udp_server(args):
    sizes = sizes_for(args.kind, args.inner)
    valid = {seq: body(seq, size) for seq, size in enumerate(sizes)}
    seen = set()
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.bind((B, PORT))
        sock.settimeout(0.15)
        Path(args.ready).write_text("ready")
        last = time.monotonic()
        started = last
        while time.monotonic() - started < 18:
            try:
                payload, anc, flags, addr = sock.recvmsg(65535)
            except socket.timeout:
                if Path(args.stop).exists() and time.monotonic() - last > 0.6:
                    break
                continue
            if flags & socket.MSG_TRUNC:
                raise AssertionError("UDP truncated by receiver")
            if addr[0] != A or len(payload) < 8 or payload[:4] != b"WBDM":
                raise AssertionError("unexpected UDP datagram")
            seq = struct.unpack("!I", payload[4:8])[0]
            if seq not in valid or payload != valid[seq] or seq in seen:
                raise AssertionError("UDP corruption, duplicate or wrong length seq=" + str(seq))
            seen.add(seq)
            last = time.monotonic()
        else:
            raise AssertionError("UDP receiver exceeded bounded deadline")
    Path(args.result).write_text(json.dumps({"received": len(seen), "sent": len(sizes),
                                              "received_sequences": sorted(seen)}))


def udp_client(args):
    sizes = sizes_for(args.kind, args.inner)
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.bind((A, 0))
        sock.setsockopt(socket.IPPROTO_IP, 10, PMTUDISC_DONT)
        for seq, size in enumerate(sizes):
            sent = sock.sendto(body(seq, size), (B, PORT))
            if sent != size:
                raise AssertionError("UDP send length mismatch")
            time.sleep(0.003 if args.kind == "udp" else 0.025)
        # DF must reject any application datagram larger than the actual route
        # MTU; 65508 bytes also exceeds IPv4's absolute UDP payload ceiling.
        sock.setsockopt(socket.IPPROTO_IP, 10, PMTUDISC_DO)
        for size in [args.inner - 27, 8936, 65507]:
            try:
                sock.sendto(body(9999, size), (B, PORT))
            except OSError as exc:
                if exc.errno != errno.EMSGSIZE:
                    raise
            else:
                raise AssertionError("DF oversize UDP unexpectedly accepted: " + str(size))
        sock.setsockopt(socket.IPPROTO_IP, 10, PMTUDISC_DONT)
        try:
            sock.sendto(body(9999, 65508), (B, PORT))
        except OSError as exc:
            if exc.errno != errno.EMSGSIZE:
                raise
        else:
            raise AssertionError("UDP IPv4 maximum exceeded without EMSGSIZE")
    return {"udp_sent": len(sizes), "udp_bytes": sum(sizes),
            "df_oversize_emsgsize": 3, "udp_65508_emsgsize": 1}


def inside(args):
    if args.role == "server":
        if args.kind == "tcp":
            tcp_server(args)
        else:
            udp_server(args)
        return
    if args.role == "client":
        result = tcp_client(args) if args.kind == "tcp" else udp_client(args)
        print(json.dumps(result))
        return
    raise AssertionError("unknown role")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--kind", choices=("tcp", "udp", "udp-jumbo"), required=True)
    parser.add_argument("--outer", type=int, choices=(1300, 1400), required=True)
    parser.add_argument("--loss", type=int, choices=(0, 5, 10), required=True)
    parser.add_argument("--fec-parity", type=int, choices=(0, 20), default=0)
    parser.add_argument("--role", choices=("host", "server", "client"), default="host")
    parser.add_argument("--inner", type=int)
    parser.add_argument("--ready")
    parser.add_argument("--result")
    parser.add_argument("--stop")
    args = parser.parse_args()
    if os.environ.get("GITHUB_ACTIONS") != "true" or os.geteuid() != 0:
        raise RuntimeError("privileged GitHub Actions only")
    if args.role != "host":
        inside(args)
        return

    inner = INNER_BY_FEC[args.fec_parity][args.outer]
    prefix = "wbdm" + str(os.getpid())
    ns_a, ns_b = prefix + "a", prefix + "b"
    # unique namespace names; interface names inside namespace stay short
    with tempfile.TemporaryDirectory(prefix="wbd-mtu-matrix-") as directory:
        ready = str(Path(directory) / "ready")
        result = str(Path(directory) / "result.json")
        stop = str(Path(directory) / "stop")
        setup = False
        server = None
        try:
            checked(["ip", "netns", "add", ns_a])
            checked(["ip", "netns", "add", ns_b])
            setup = True
            checked(["ip", "link", "add", "wda", "type", "veth", "peer", "name", "wdb"])
            checked(["ip", "link", "set", "wda", "netns", ns_a])
            checked(["ip", "link", "set", "wdb", "netns", ns_b])
            for ns, dev, address in [(ns_a, "wda", A), (ns_b, "wdb", B)]:
                checked(["ip", "-n", ns, "addr", "add", address + "/30", "dev", dev])
                checked(["ip", "-n", ns, "link", "set", "lo", "up"])
                checked(["ip", "-n", ns, "link", "set", dev, "mtu", str(inner), "up"])
                if args.loss:
                    checked(["ip", "netns", "exec", ns, "tc", "qdisc", "add",
                             "dev", dev, "root", "netem", "loss", str(args.loss) + "%"])
            command = [sys.executable, str(Path(__file__).resolve()),
                       "--kind", args.kind, "--outer", str(args.outer),
                       "--loss", str(args.loss), "--fec-parity", str(args.fec_parity),
                       "--inner", str(inner),
                       "--ready", ready, "--result", result, "--stop", stop]
            server = subprocess.Popen(["ip", "netns", "exec", ns_b] + command +
                                      ["--role", "server"], stdout=subprocess.PIPE,
                                      stderr=subprocess.PIPE, text=True)
            deadline = time.monotonic() + 8
            while not Path(ready).exists():
                if server.poll() is not None or time.monotonic() > deadline:
                    raise AssertionError("receiver did not start")
                time.sleep(.02)
            sent = checked(["ip", "netns", "exec", ns_a] + command + ["--role", "client"])
            Path(stop).write_text("done")
            stdout, stderr = server.communicate(timeout=50)
            if server.returncode != 0:
                raise AssertionError("receiver failed: " + stderr[-2000:])
            received = json.loads(Path(result).read_text())
            sent_metrics = json.loads(sent.stdout)
            print(json.dumps({"observation":"PRE_ASSERT", "scope":"LINUX_KERNEL_VETH_SOCKETS_ONLY", "outer":args.outer, "loss":args.loss, "kind":args.kind, "sent":sent_metrics, "received":received["received"]}), flush=True)
            if args.kind == "tcp":
                if received["received"] != sent_metrics["tcp_messages"]:
                    raise AssertionError("TCP missing bytes/messages")
            elif args.loss == 0 and received["received"] != sent_metrics["udp_sent"]:
                raise AssertionError("0% UDP loss: incomplete application delivery")
            elif args.kind == "udp" and received["received"] < 10:
                stat_a = checked(["ip", "netns", "exec", ns_a, "tc", "-s", "qdisc", "show", "dev", "wda"]).stdout
                stat_b = checked(["ip", "netns", "exec", ns_b, "tc", "-s", "qdisc", "show", "dev", "wdb"]).stdout
                raise AssertionError("unexpected near-total small UDP loss: received=%d sent=%d; sender=%s receiver=%s" % (received["received"], sent_metrics["udp_sent"], stat_a.strip(), stat_b.strip()))
            report = {"result": "PASS", "scope": "LINUX_KERNEL_VETH_SOCKETS_ONLY",
                      "outer_config": args.outer, "inner_mtu": inner,
                      "fec_parity_budget": args.fec_parity,
                      "fec_coding_in_kernel_scope": False,
                      "loss_percent": args.loss, "kind": args.kind,
                      "sent": sent_metrics, "received": received["received"],
                      "dropped_application_datagrams": (sent_metrics.get("udp_sent", 0) -
                                                        received["received"]) if args.kind != "tcp" else 0,
                      "all_sent_udp_received": received["received"] == sent_metrics.get("udp_sent", -1)
                                              if args.kind != "tcp" else None,
                      "jumbo_under_loss_compatibility": "NOT_PROVEN" if args.kind == "udp-jumbo" and args.loss else "TESTED"}
            print(json.dumps(report, separators=(",", ":")))
        finally:
            if server and server.poll() is None:
                server.terminate()
                server.communicate(timeout=5)
            if setup:
                for ns in (ns_a, ns_b):
                    subprocess.run(["ip", "netns", "delete", ns], capture_output=True)
            else:
                for ns in (ns_a, ns_b):
                    subprocess.run(["ip", "netns", "delete", ns], capture_output=True)


if __name__ == "__main__":
    main()
