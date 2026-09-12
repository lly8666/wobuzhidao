#!/usr/bin/env python3
import pathlib
import sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else ".")

mux = root / "cmd/wbd-faketcp-mux/main_linux.go"
s = mux.read_text()
old = '\t"os"\n\t"os/signal"\n\t"strings"\n'
new = '\t"os"\n\t"os/signal"\n\t"strconv"\n\t"strings"\n'
if s.count(old) != 1:
    raise SystemExit("mux import marker drift")
s = s.replace(old, new, 1)
old = '''\tif err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {\n\t\t_ = syscall.Close(fd)\n\t\treturn nil, err\n\t}\n'''
new = '''\tif err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {\n\t\t_ = syscall.Close(fd)\n\t\treturn nil, err\n\t}\n\tif raw := strings.TrimSpace(os.Getenv("WBD_DIAG_RAW_RCVBUF_BYTES")); raw != "" {\n\t\twant, parseErr := strconv.Atoi(raw)\n\t\tif parseErr != nil || want <= 0 {\n\t\t\t_ = syscall.Close(fd)\n\t\t\treturn nil, fmt.Errorf("invalid WBD_DIAG_RAW_RCVBUF_BYTES %q", raw)\n\t\t}\n\t\tif setErr := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, want); setErr != nil {\n\t\t\t_ = syscall.Close(fd)\n\t\t\treturn nil, fmt.Errorf("set raw SO_RCVBUF=%d: %w", want, setErr)\n\t\t}\n\t\tactual, getErr := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF)\n\t\tif getErr != nil {\n\t\t\t_ = syscall.Close(fd)\n\t\t\treturn nil, fmt.Errorf("get raw SO_RCVBUF: %w", getErr)\n\t\t}\n\t\tfmt.Printf("WBD_RAW_RCVBUF requested=%d actual=%d\\n", want, actual)\n\t}\n'''
if s.count(old) != 1:
    raise SystemExit("mux socket marker drift")
s = s.replace(old, new, 1)
mux.write_text(s)

full = root / "scripts/game_lane_fullstack.sh"
s = full.read_text()
marker = '''sudo ip -n "$C" link set gc0 up\nsudo ip -n "$S" link set gs0 up\n'''
insert = '''sudo ip -n "$C" link set gc0 up\nsudo ip -n "$S" link set gs0 up\nif [[ -n "${WBD_DIAG_RAW_RCVBUF_BYTES:-}" ]]; then\n  sudo ip netns exec "$S" sysctl -qw net.core.rmem_max="$WBD_DIAG_RAW_RCVBUF_BYTES"\n  echo "WBD_RAW_RCVBUF_SYSCTL namespace=server rmem_max=$(sudo ip netns exec \"$S\" sysctl -n net.core.rmem_max)"\nfi\n'''
if s.count(marker) != 1:
    raise SystemExit("fullstack namespace marker drift")
s = s.replace(marker, insert, 1)
full.write_text(s)
