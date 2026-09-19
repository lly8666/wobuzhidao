#!/usr/bin/env python3
"""Small controlled UDP/TCP echo target for WBD physical qualification."""

import argparse
import socket
import threading


def udp_loop(host: str, port: int) -> None:
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind((host, port))
        print(f"WBD_QUAL_ECHO_UDP_READY listen={host}:{port}", flush=True)
        while True:
            data, peer = sock.recvfrom(65535)
            if data:
                sock.sendto(data, peer)


def tcp_client(conn: socket.socket, peer: tuple[str, int]) -> None:
    try:
        with conn:
            while True:
                data = conn.recv(65535)
                if not data:
                    return
                conn.sendall(data)
    except OSError as exc:
        print(f"tcp client {peer[0]}:{peer[1]} error: {exc}", flush=True)


def tcp_loop(host: str, port: int) -> None:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        sock.bind((host, port))
        sock.listen(128)
        print(f"WBD_QUAL_ECHO_TCP_READY listen={host}:{port}", flush=True)
        while True:
            conn, peer = sock.accept()
            threading.Thread(target=tcp_client, args=(conn, peer), daemon=True).start()


def main() -> None:
    parser = argparse.ArgumentParser(description="Controlled WBD physical qualification echo target")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--udp-port", type=int, default=37001)
    parser.add_argument("--tcp-port", type=int, default=37002)
    args = parser.parse_args()
    for value, label in ((args.udp_port, "udp-port"), (args.tcp_port, "tcp-port")):
        if value < 1 or value > 65535:
            parser.error(f"{label} must be 1..65535")

    udp = threading.Thread(target=udp_loop, args=(args.host, args.udp_port), daemon=True)
    udp.start()
    tcp_loop(args.host, args.tcp_port)


if __name__ == "__main__":
    main()
