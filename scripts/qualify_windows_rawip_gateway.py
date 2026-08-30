#!/usr/bin/env python3
"""Privileged qualification for Windows-style Wintun raw-IP server egress.

Two client namespaces intentionally reuse inner 10.66.0.2/30. Each real wbd-tun
client sends M6A raw-IP frames to wbd-ip-gateway-server. The gateway must create
isolated VRF/conntrack-zone TUN sessions and route/NAT traffic to an Internet
namespace. DNS-style UDP, generic UDP and simultaneous identical-tuple TCP are
required to round-trip.
"""
from __future__ import annotations

import argparse
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time

CLIENTS = [
    ("wbdg-c1", "wg1h", "wg1c", "192.0.2.1", "192.0.2.2", 50001),
    ("wbdg-c2", "wg2h", "wg2c", "192.0.2.5", "192.0.2.6", 50002),
]
INTERNET_NS = "wbdg-int"
INTERNET_HOST_IF = "wgih"
INTERNET_NS_IF = "wgii"
INTERNET_HOST = "203.0.113.1"
INTERNET_IP = "203.0.113.2"
INNER_CLIENT = "10.66.0.2/30"


def run(cmd: list[str], *, check: bool = True, capture: bool = False) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, text=True, check=check,
                          stdout=subprocess.PIPE if capture else None,
                          stderr=subprocess.STDOUT if capture else None)


def ns(nsname: str, *cmd: str) -> list[str]:
    return ["ip", "netns", "exec", nsname, *cmd]


def quiet(cmd: list[str]) -> None:
    subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def require(name: str) -> None:
    if shutil.which(name) is None:
        raise RuntimeError(f"required command missing: {name}")


def cleanup() -> None:
    for name, host_if, _, _, _, _ in CLIENTS:
        quiet(["ip", "netns", "del", name])
        quiet(["ip", "link", "del", host_if])
    quiet(["ip", "netns", "del", INTERNET_NS])
    quiet(["ip", "link", "del", INTERNET_HOST_IF])
    for i in range(64):
        quiet(["ip", "link", "del", f"wh{i:02d}"])
        quiet(["ip", "link", "del", f"wv{i:02d}"])
        quiet(["ip", "link", "del", f"wt{i:02d}"])


def setup_network() -> None:
    cleanup()
    for name, host_if, ns_if, host_ip, client_ip, _ in CLIENTS:
        run(["ip", "netns", "add", name])
        run(["ip", "link", "add", host_if, "type", "veth", "peer", "name", ns_if])
        run(["ip", "link", "set", ns_if, "netns", name])
        run(["ip", "addr", "add", host_ip + "/30", "dev", host_if])
        run(["ip", "link", "set", host_if, "up"])
        run(ns(name, "ip", "link", "set", "lo", "up"))
        run(ns(name, "ip", "addr", "add", client_ip + "/30", "dev", ns_if))
        run(ns(name, "ip", "link", "set", ns_if, "up"))

    run(["ip", "netns", "add", INTERNET_NS])
    run(["ip", "link", "add", INTERNET_HOST_IF, "type", "veth", "peer", "name", INTERNET_NS_IF])
    run(["ip", "link", "set", INTERNET_NS_IF, "netns", INTERNET_NS])
    run(["ip", "addr", "add", INTERNET_HOST + "/30", "dev", INTERNET_HOST_IF])
    run(["ip", "link", "set", INTERNET_HOST_IF, "up"])
    run(ns(INTERNET_NS, "ip", "link", "set", "lo", "up"))
    run(ns(INTERNET_NS, "ip", "addr", "add", INTERNET_IP + "/30", "dev", INTERNET_NS_IF))
    run(ns(INTERNET_NS, "ip", "link", "set", INTERNET_NS_IF, "up"))
    run(ns(INTERNET_NS, "ip", "route", "add", "default", "via", INTERNET_HOST))


class Proc:
    def __init__(self, name: str, argv: list[str], out: Path):
        self.name = name
        self.path = out / f"{name}.log"
        self.file = self.path.open("wb")
        self.p = subprocess.Popen(argv, stdout=self.file, stderr=subprocess.STDOUT, start_new_session=True)

    def stop(self) -> None:
        if self.p.poll() is None:
            try:
                os.killpg(self.p.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                self.p.wait(timeout=2)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(self.p.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                self.p.wait(timeout=2)
        self.file.close()


def wait_log(p: Proc, needle: str, timeout: float = 12.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        text = p.path.read_text(errors="replace") if p.path.exists() else ""
        if needle in text:
            return
        if p.p.poll() is not None:
            raise RuntimeError(f"{p.name} exited rc={p.p.returncode}; wanted {needle}; log:\n{text[-4000:]}")
        time.sleep(0.05)
    text = p.path.read_text(errors="replace") if p.path.exists() else ""
    raise RuntimeError(f"timeout waiting {needle} in {p.name}; log:\n{text[-4000:]}")


def start_internet(out: Path) -> Proc:
    code = r'''
import socket,threading
IP='203.0.113.2'
def udp(port):
 s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((IP,port))
 while True:
  d,a=s.recvfrom(65535); s.sendto(d,a)
def tcp():
 s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind((IP,8443)); s.listen(16)
 while True:
  c,_=s.accept()
  def one(c):
   with c:
    while True:
     d=c.recv(65535)
     if not d: return
     c.sendall(d)
  threading.Thread(target=one,args=(c,),daemon=True).start()
threading.Thread(target=udp,args=(53,),daemon=True).start()
threading.Thread(target=udp,args=(5300,),daemon=True).start()
print('INTERNET_READY',flush=True)
tcp()
'''
    return Proc("internet", ns(INTERNET_NS, "python3", "-u", "-c", code), out)


def udp_probe(client_ns: str, port: int, tag: str) -> str:
    code = r'''
import socket,sys,time
ip=sys.argv[1]; port=int(sys.argv[2]); tag=sys.argv[3].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.settimeout(5)
t=time.monotonic(); s.sendto(tag,(ip,port)); d,_=s.recvfrom(4096)
assert d==tag,(d,tag)
print(f'UDP_PASS port={port} rtt_ms={(time.monotonic()-t)*1000:.3f}')
'''
    cp = run(ns(client_ns, "python3", "-c", code, INTERNET_IP, str(port), tag), capture=True)
    return cp.stdout.strip()


def simultaneous_tcp() -> list[str]:
    code = r'''
import socket,sys,time
payload=sys.argv[1].encode(); s=socket.socket(); s.settimeout(7)
s.bind(('10.66.0.2',40000)); t=time.monotonic(); s.connect(('203.0.113.2',8443)); s.sendall(payload)
d=b''
while len(d)<len(payload):
 b=s.recv(4096)
 if not b: break
 d+=b
assert d==payload,(d,payload)
print(f'TCP_PASS local={s.getsockname()} rtt_ms={(time.monotonic()-t)*1000:.3f}')
'''
    ps = []
    for i, (name, *_rest) in enumerate(CLIENTS, 1):
        ps.append(subprocess.Popen(ns(name, "python3", "-c", code, f"same-tuple-client-{i}"),
                                   text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT))
    out = []
    for p in ps:
        text, _ = p.communicate(timeout=12)
        if p.returncode != 0:
            raise RuntimeError(f"simultaneous TCP failed rc={p.returncode}: {text}")
        out.append(text.strip())
    return out


def qualify(a: argparse.Namespace, backend: str, out: Path) -> None:
    setup_network()
    procs: list[Proc] = []
    try:
        internet = start_internet(out)
        procs.append(internet); wait_log(internet, "INTERNET_READY")
        gateway = Proc("gateway-" + backend, [str(a.gateway),
            "-listen", "0.0.0.0:49100",
            "-firewall-helper", str(a.firewall),
            "-backend", backend,
            "-firewall-state", f"/tmp/wbd-ipg-{backend}.state",
            "-max-sessions", "8",
            "-idle-timeout", "30s"], out)
        procs.append(gateway); wait_log(gateway, "WBD_IP_GATEWAY_READY")

        for idx, (name, _host_if, _ns_if, host_ip, client_ip, local_port) in enumerate(CLIENTS):
            tun = Proc(f"tun-{backend}-{idx+1}", ns(name, str(a.wbd_tun),
                "-mode", "client", "-ifname", "wbd0", "-mtu", "1400",
                "-local", f"{client_ip}:{local_port}", "-transport", f"{host_ip}:49100"), out)
            procs.append(tun); wait_log(tun, "WBD_TUN_READY")
            run(ns(name, "ip", "link", "set", "wbd0", "mtu", "1400", "up"))
            run(ns(name, "ip", "addr", "add", INNER_CLIENT, "dev", "wbd0"))
            run(ns(name, "ip", "route", "add", INTERNET_IP + "/32", "dev", "wbd0"))

        print(f"[{backend}]", udp_probe(CLIENTS[0][0], 53, "dns-one"))
        print(f"[{backend}]", udp_probe(CLIENTS[1][0], 53, "dns-two"))
        print(f"[{backend}]", udp_probe(CLIENTS[0][0], 5300, "udp-one"))
        print(f"[{backend}]", udp_probe(CLIENTS[1][0], 5300, "udp-two"))
        for line in simultaneous_tcp(): print(f"[{backend}] {line}")

        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            text = gateway.path.read_text(errors="replace")
            if text.count("WBD_IP_GATEWAY_SESSION_READY") >= 2:
                break
            time.sleep(0.05)
        text = gateway.path.read_text(errors="replace")
        if text.count("WBD_IP_GATEWAY_SESSION_READY") < 2:
            raise RuntimeError("gateway did not create two isolated sessions\n" + text[-6000:])
        if "zone=2000" not in text or "zone=2001" not in text:
            raise RuntimeError("conntrack zones 2000/2001 not observed\n" + text[-6000:])
        print(f"WBD_RAWIP_GATEWAY_QUALIFY_PASS backend={backend} sessions=2 same_inner=10.66.0.2 same_tcp_source=40000 dns_udp_tcp=1")
    finally:
        for p in reversed(procs):
            p.stop()
        cleanup()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--gateway", type=Path, required=True)
    ap.add_argument("--wbd-tun", type=Path, required=True)
    ap.add_argument("--firewall", type=Path, required=True)
    ap.add_argument("--backend", choices=["iptables", "nft", "both"], default="both")
    ap.add_argument("--out", type=Path, default=Path("/tmp/wbd-rawip-gateway"))
    a = ap.parse_args()
    if os.geteuid() != 0:
        raise RuntimeError("qualification requires root")
    if not Path("/dev/net/tun").exists():
        raise RuntimeError("/dev/net/tun missing")
    for name in ("ip", "python3", "sysctl"):
        require(name)
    a.gateway = a.gateway.resolve(); a.wbd_tun = a.wbd_tun.resolve(); a.firewall = a.firewall.resolve()
    for p in (a.gateway, a.wbd_tun, a.firewall):
        if not p.exists(): raise RuntimeError(f"missing {p}")
    a.out.mkdir(parents=True, exist_ok=True)
    backends = [a.backend] if a.backend != "both" else ["iptables", "nft"]
    for backend in backends:
        require(backend)
        qualify(a, backend, a.out)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    finally:
        cleanup()
