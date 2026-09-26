#!/usr/bin/env python3
"""V2-M6B privileged Linux end-to-end TUN qualification harness.

This harness creates two Linux network namespaces connected by a veth underlay,
starts the already-qualified one-lane WBD composition in the namespaces, creates
one TUN per endpoint through cmd/wbd-tun, and verifies real IPv4/IPv6, UDP and
TCP traffic through:

    TUN -> WBDP -> UDPspeeder -> DTLS 1.3 -> udp2raw FakeTCP -> underlay

It is intentionally fail-fast.  It does not emulate /dev/net/tun or replace a
missing raw-packet capability with a weaker test.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import time
from dataclasses import dataclass
from typing import Iterable

EXPECTED_UDP2RAW_SHA = "c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c"
EXPECTED_SPEEDER_SHA = "f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a"
EXPECTED_DTLS_SHIM_SHA = "63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a"

CLIENT_UNDERLAY = "198.18.40.1"
SERVER_UNDERLAY = "198.18.40.2"
CLIENT_TUN4 = "10.77.0.1"
SERVER_TUN4 = "10.77.0.2"
CLIENT_TUN6 = "fd77:7762::1"
SERVER_TUN6 = "fd77:7762::2"


@dataclass
class Proc:
    role: str
    popen: subprocess.Popen
    log_file: object
    log_path: Path


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def run(cmd: Iterable[object], *, check: bool = True, capture: bool = False) -> subprocess.CompletedProcess:
    argv = [str(x) for x in cmd]
    return subprocess.run(
        argv,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.STDOUT if capture else None,
        check=check,
    )


def ns_cmd(ns: str, *cmd: object) -> list[str]:
    return ["ip", "netns", "exec", ns, *[str(x) for x in cmd]]


def start(role: str, cmd: Iterable[object], out: Path) -> Proc:
    log_path = out / f"{role}.log"
    f = log_path.open("wb")
    p = subprocess.Popen(
        [str(x) for x in cmd],
        stdout=f,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    return Proc(role=role, popen=p, log_file=f, log_path=log_path)


def stop_proc(proc: Proc) -> None:
    p = proc.popen
    if p.poll() is None:
        try:
            os.killpg(p.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            p.wait(timeout=2.0)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(p.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            try:
                p.wait(timeout=1.0)
            except subprocess.TimeoutExpired:
                pass
    try:
        proc.log_file.close()
    except Exception:
        pass


def wait_log(proc: Proc, needle: str, timeout: float = 12.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            text = proc.log_path.read_text(errors="replace")
        except OSError:
            text = ""
        if needle in text:
            return
        rc = proc.popen.poll()
        if rc is not None:
            raise RuntimeError(f"{proc.role} exited rc={rc}; expected {needle!r}; log={proc.log_path}")
        time.sleep(0.05)
    raise RuntimeError(f"timeout waiting for {needle!r} in {proc.role}; log={proc.log_path}")


def require_file(path: Path, label: str) -> Path:
    path = path.resolve()
    if not path.is_file():
        raise RuntimeError(f"{label} not found: {path}")
    return path


def require_command(name: str) -> None:
    if shutil.which(name) is None:
        raise RuntimeError(f"required command not found: {name}")


def check_prerequisites(a: argparse.Namespace) -> dict:
    if os.geteuid() != 0:
        raise RuntimeError("M6B requires root/CAP_NET_ADMIN/CAP_NET_RAW")
    if not Path("/dev/net/tun").exists():
        raise RuntimeError("/dev/net/tun is missing; real M6B TUN qualification cannot run")

    for name in ("ip", "ping", "python3"):
        require_command(name)
    if not a.no_auto_rule:
        require_command("iptables")

    a.wbd_tun = require_file(a.wbd_tun, "wbd-tun")
    a.udp2raw = require_file(a.udp2raw, "udp2raw")
    a.udpspeeder = require_file(a.udpspeeder, "UDPspeeder")
    a.dtls_shim = require_file(a.dtls_shim, "DTLS shim")
    a.cert_dir = a.cert_dir.resolve()
    for name in ("ca.pem", "server.pem", "server.key"):
        require_file(a.cert_dir / name, name)

    hashes = {
        "wbd_tun": sha256(a.wbd_tun),
        "udp2raw": sha256(a.udp2raw),
        "udpspeeder": sha256(a.udpspeeder),
        "dtls_shim": sha256(a.dtls_shim),
    }
    expected = {
        "udp2raw": EXPECTED_UDP2RAW_SHA,
        "udpspeeder": EXPECTED_SPEEDER_SHA,
        "dtls_shim": EXPECTED_DTLS_SHIM_SHA,
    }
    for key, want in expected.items():
        if hashes[key] != want:
            raise RuntimeError(f"{key} SHA-256 mismatch: got {hashes[key]}, want {want}")
    return hashes


def delete_ns(name: str) -> None:
    subprocess.run(["ip", "netns", "del", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def make_namespaces(client_ns: str, server_ns: str) -> None:
    delete_ns(client_ns)
    delete_ns(server_ns)
    run(["ip", "netns", "add", client_ns])
    run(["ip", "netns", "add", server_ns])
    run(["ip", "link", "add", "wbdv-c", "type", "veth", "peer", "name", "wbdv-s"])
    run(["ip", "link", "set", "wbdv-c", "netns", client_ns])
    run(["ip", "link", "set", "wbdv-s", "netns", server_ns])
    for ns in (client_ns, server_ns):
        run(ns_cmd(ns, "ip", "link", "set", "lo", "up"))
    run(ns_cmd(client_ns, "ip", "addr", "add", f"{CLIENT_UNDERLAY}/30", "dev", "wbdv-c"))
    run(ns_cmd(server_ns, "ip", "addr", "add", f"{SERVER_UNDERLAY}/30", "dev", "wbdv-s"))
    run(ns_cmd(client_ns, "ip", "link", "set", "wbdv-c", "up"))
    run(ns_cmd(server_ns, "ip", "link", "set", "wbdv-s", "up"))


def add_tun_addresses(client_ns: str, server_ns: str, mtu: int) -> None:
    run(ns_cmd(client_ns, "ip", "link", "set", "dev", "wbd0", "mtu", mtu, "up"))
    run(ns_cmd(server_ns, "ip", "link", "set", "dev", "wbd0", "mtu", mtu, "up"))
    run(ns_cmd(client_ns, "ip", "addr", "add", f"{CLIENT_TUN4}/30", "dev", "wbd0"))
    run(ns_cmd(server_ns, "ip", "addr", "add", f"{SERVER_TUN4}/30", "dev", "wbd0"))
    run(ns_cmd(client_ns, "ip", "-6", "addr", "add", f"{CLIENT_TUN6}/64", "dev", "wbd0"))
    run(ns_cmd(server_ns, "ip", "-6", "addr", "add", f"{SERVER_TUN6}/64", "dev", "wbd0"))


def start_stack(a: argparse.Namespace, client_ns: str, server_ns: str, out: Path) -> list[Proc]:
    b = a.base_port
    key = a.shared_key
    procs: list[Proc] = []

    # Final decoded WBDP datagrams terminate at wbd-tun server.
    procs.append(start(
        "tun-server",
        ns_cmd(server_ns, a.wbd_tun, "-mode", "server", "-ifname", "wbd0", "-mtu", a.mtu, "-listen", f"127.0.0.1:{b}"),
        out,
    ))
    wait_log(procs[-1], "WBD_TUN_READY")

    procs.append(start(
        "speeder-server",
        ns_cmd(server_ns, a.udpspeeder, "-s", f"-l127.0.0.1:{b+1}", f"-r127.0.0.1:{b}", f"-f{a.fec}", "--mode", "0", "--timeout", "8", "-k", key, "--disable-color", "--log-level", "2"),
        out,
    ))
    procs.append(start(
        "dtls-server",
        ns_cmd(server_ns, a.dtls_shim, "server", b+2, "127.0.0.1", b+1, a.cert_dir / "server.pem", a.cert_dir / "server.key"),
        out,
    ))

    server_raw = [
        a.udp2raw, "-s", f"-l{SERVER_UNDERLAY}:{b+3}", f"-r127.0.0.1:{b+2}",
        "-k", key, "--raw-mode", "faketcp", "--disable-color", "--log-level", "2",
    ]
    if not a.no_auto_rule:
        server_raw.append("-a")
    procs.append(start("udp2raw-server", ns_cmd(server_ns, *server_raw), out))
    time.sleep(0.15)

    client_raw = [
        a.udp2raw, "-c", f"-l127.0.0.1:{b+4}", f"-r{SERVER_UNDERLAY}:{b+3}",
        "-k", key, "--raw-mode", "faketcp", "--source-ip", CLIENT_UNDERLAY,
        "--source-port", str(b+10), "--disable-color", "--log-level", "2",
    ]
    if not a.no_auto_rule:
        client_raw.append("-a")
    procs.append(start("udp2raw-client", ns_cmd(client_ns, *client_raw), out))
    time.sleep(0.25)

    procs.append(start(
        "dtls-client",
        ns_cmd(client_ns, a.dtls_shim, "client", b+5, "127.0.0.1", b+4, a.cert_dir / "ca.pem", a.server_name),
        out,
    ))
    wait_log(procs[2], "READY role=server")
    wait_log(procs[-1], "READY role=client")

    procs.append(start(
        "speeder-client",
        ns_cmd(client_ns, a.udpspeeder, "-c", f"-l127.0.0.1:{b+7}", f"-r127.0.0.1:{b+5}", f"-f{a.fec}", "--mode", "0", "--timeout", "8", "-k", key, "--disable-color", "--log-level", "2"),
        out,
    ))
    time.sleep(0.25)

    procs.append(start(
        "tun-client",
        ns_cmd(client_ns, a.wbd_tun, "-mode", "client", "-ifname", "wbd0", "-mtu", a.mtu, "-transport", f"127.0.0.1:{b+7}"),
        out,
    ))
    wait_log(procs[-1], "WBD_TUN_READY")
    return procs


def command_result(cmd: list[str], timeout: float = 15.0) -> dict:
    started = time.monotonic()
    cp = subprocess.run(cmd, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
    return {
        "ok": cp.returncode == 0,
        "returncode": cp.returncode,
        "elapsed_ms": round((time.monotonic() - started) * 1000, 3),
        "output": cp.stdout[-4096:],
    }


def udp_probe(ns: str, host: str, port: int) -> dict:
    code = r'''
import socket,sys,time
host=sys.argv[1]; port=int(sys.argv[2]); payload=b"WBD-M6B-UDP-ROUNDTRIP"
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.settimeout(4)
t0=time.monotonic(); s.sendto(payload,(host,port)); data,_=s.recvfrom(4096); dt=(time.monotonic()-t0)*1000
if data != payload: raise SystemExit("payload mismatch")
print(f"udp_rtt_ms={dt:.3f} bytes={len(data)}")
'''
    return command_result(ns_cmd(ns, "python3", "-c", code, host, port), timeout=8)


def tcp_probe(ns: str, host: str, port: int) -> dict:
    code = r'''
import socket,sys,time
host=sys.argv[1]; port=int(sys.argv[2]); payload=b"WBD-M6B-TCP-INSIDE-VPN"
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM); s.settimeout(5)
t0=time.monotonic(); s.connect((host,port)); s.sendall(payload)
data=b""
while len(data)<len(payload):
    b=s.recv(4096)
    if not b: break
    data+=b
if data != payload: raise SystemExit("payload mismatch")
print(f"tcp_roundtrip_ms={(time.monotonic()-t0)*1000:.3f} bytes={len(data)}")
'''
    return command_result(ns_cmd(ns, "python3", "-c", code, host, port), timeout=10)


def echo_server(ns: str, kind: str, host: str, port: int, out: Path) -> Proc:
    if kind == "udp":
        code = r'''
import socket,sys
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((sys.argv[1],int(sys.argv[2])))
while True:
 d,a=s.recvfrom(65535); s.sendto(d,a)
'''
    else:
        code = r'''
import socket,sys
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind((sys.argv[1],int(sys.argv[2]))); s.listen(8)
while True:
 c,_=s.accept()
 with c:
  while True:
   d=c.recv(65535)
   if not d: break
   c.sendall(d)
'''
    return start(f"inner-{kind}-echo", ns_cmd(ns, "python3", "-u", "-c", code, host, port), out)


def parse_last_json(path: Path) -> dict | None:
    try:
        lines = path.read_text(errors="replace").splitlines()
    except OSError:
        return None
    for line in reversed(lines):
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            return value
    return None


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--wbd-tun", type=Path, required=True)
    ap.add_argument("--udp2raw", type=Path, required=True)
    ap.add_argument("--udpspeeder", type=Path, required=True)
    ap.add_argument("--dtls-shim", type=Path, required=True)
    ap.add_argument("--cert-dir", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--server-name", default="wbd.test")
    ap.add_argument("--fec", choices=("20:10", "20:20"), default="20:20")
    ap.add_argument("--mtu", type=int, default=1400)
    ap.add_argument("--base-port", type=int, default=46000)
    ap.add_argument("--shared-key", default="wbdtest")
    ap.add_argument("--namespace-prefix", default="wbd-m6b")
    ap.add_argument("--no-auto-rule", action="store_true", help="do not pass udp2raw -a; caller must suppress kernel RSTs")
    ap.add_argument("--keep-namespaces", action="store_true")
    a = ap.parse_args()

    a.out = a.out.resolve()
    a.out.mkdir(parents=True, exist_ok=True)
    receipt_path = a.out / "receipt.json"
    client_ns = f"{a.namespace_prefix}-c"
    server_ns = f"{a.namespace_prefix}-s"
    procs: list[Proc] = []
    receipt = {
        "schema": "wbd-v2-m6b-tun-qualification/v1",
        "result": "fail",
        "fec": a.fec,
        "mtu": a.mtu,
        "client_namespace": client_ns,
        "server_namespace": server_ns,
        "underlay": {"client": CLIENT_UNDERLAY, "server": SERVER_UNDERLAY},
        "tunnel_ipv4": {"client": CLIENT_TUN4, "server": SERVER_TUN4},
        "tunnel_ipv6": {"client": CLIENT_TUN6, "server": SERVER_TUN6},
        "tests": {},
    }

    try:
        receipt["binary_sha256"] = check_prerequisites(a)
        make_namespaces(client_ns, server_ns)
        underlay = command_result(ns_cmd(client_ns, "ping", "-c", "2", "-W", "1", SERVER_UNDERLAY), timeout=5)
        receipt["tests"]["underlay_ping"] = underlay
        if not underlay["ok"]:
            raise RuntimeError("namespace underlay ping failed")

        procs = start_stack(a, client_ns, server_ns, a.out)
        add_tun_addresses(client_ns, server_ns, a.mtu)
        time.sleep(0.2)

        ping4 = command_result(ns_cmd(client_ns, "ping", "-c", "5", "-W", "2", SERVER_TUN4), timeout=15)
        receipt["tests"]["ipv4_ping"] = ping4
        if not ping4["ok"]:
            raise RuntimeError("IPv4 TUN ping failed")

        ping6 = command_result(ns_cmd(client_ns, "ping", "-6", "-c", "5", "-W", "2", SERVER_TUN6), timeout=15)
        receipt["tests"]["ipv6_ping"] = ping6
        if not ping6["ok"]:
            raise RuntimeError("IPv6 TUN ping failed")

        udp_echo = echo_server(server_ns, "udp", SERVER_TUN4, a.base_port + 20, a.out)
        procs.append(udp_echo)
        time.sleep(0.1)
        udp = udp_probe(client_ns, SERVER_TUN4, a.base_port + 20)
        receipt["tests"]["udp_roundtrip"] = udp
        if not udp["ok"]:
            raise RuntimeError("UDP-over-WBD round trip failed")

        tcp_echo = echo_server(server_ns, "tcp", SERVER_TUN4, a.base_port + 21, a.out)
        procs.append(tcp_echo)
        time.sleep(0.1)
        tcp = tcp_probe(client_ns, SERVER_TUN4, a.base_port + 21)
        receipt["tests"]["tcp_inside_vpn"] = tcp
        if not tcp["ok"]:
            raise RuntimeError("TCP-inside-WBD round trip failed")

        receipt["result"] = "pass"
    except Exception as exc:
        receipt["error"] = str(exc)
    finally:
        for proc in reversed(procs):
            stop_proc(proc)
        receipt["wbd_tun_stats"] = {
            proc.role: parse_last_json(proc.log_path)
            for proc in procs
            if proc.role in {"tun-client", "tun-server"}
        }
        if not a.keep_namespaces:
            delete_ns(client_ns)
            delete_ns(server_ns)
        receipt_path.write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    print(json.dumps(receipt, indent=2, sort_keys=True))
    return 0 if receipt["result"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
