#!/usr/bin/env python3
from pathlib import Path
import sys

product = Path(sys.argv[1])
p = product / "cmd/wbd-link-server-mux/main.go"
s = p.read_text()

old = '''\tmtu          int\n}'''
new = '''\tmtu          int\n\tdiagRCVBufEffective int\n}'''
if s.count(old) != 1:
    raise SystemExit(f"config marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\tflag.IntVar(&c.mtu, "mtu", defaultInnerMTU, "expected inner IP MTU; full inner IPv4/IPv6 packet, excluding WBD and outer overhead")\n\tflag.Parse()'''
new = '''\tflag.IntVar(&c.mtu, "mtu", defaultInnerMTU, "expected inner IP MTU; full inner IPv4/IPv6 packet, excluding WBD and outer overhead")\n\tflag.IntVar(&c.diagRCVBufEffective, "diag-rcvbuf-effective-bytes", 0, "diagnostic-only desired effective SO_RCVBUF for shared LINK UDP socket; 0 preserves product default")\n\tflag.Parse()'''
if s.count(old) != 1:
    raise SystemExit(f"flag marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t_ = conn.SetReadBuffer(4 << 20)\n\t_ = conn.SetWriteBuffer(4 << 20)'''
new = '''\tif c.diagRCVBufEffective > 0 {\n\t\tif c.diagRCVBufEffective < 64<<10 || c.diagRCVBufEffective%2 != 0 {\n\t\t\t_ = conn.Close()\n\t\t\treturn nil, errors.New("diagnostic effective receive buffer must be even and >=64KiB")\n\t\t}\n\t\traw, rawErr := conn.SyscallConn()\n\t\tif rawErr != nil {\n\t\t\t_ = conn.Close()\n\t\t\treturn nil, rawErr\n\t\t}\n\t\tconst soRcvbufForce = 33 // Linux SO_RCVBUFFORCE; diagnostic workflow runs privileged.\n\t\tvar setErr error\n\t\tvar effective int\n\t\tif err := raw.Control(func(fd uintptr) {\n\t\t\tsetErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soRcvbufForce, c.diagRCVBufEffective/2)\n\t\t\tif setErr == nil {\n\t\t\t\teffective, setErr = syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)\n\t\t\t}\n\t\t}); err != nil {\n\t\t\t_ = conn.Close()\n\t\t\treturn nil, err\n\t\t}\n\t\tif setErr != nil {\n\t\t\t_ = conn.Close()\n\t\t\treturn nil, fmt.Errorf("set diagnostic LINK SO_RCVBUF: %w", setErr)\n\t\t}\n\t\tif effective != c.diagRCVBufEffective {\n\t\t\t_ = conn.Close()\n\t\t\treturn nil, fmt.Errorf("diagnostic LINK SO_RCVBUF mismatch requested_effective=%d observed=%d", c.diagRCVBufEffective, effective)\n\t\t}\n\t\tfmt.Printf("WBD_LINK_RCVBUF_DIAG requested_effective_bytes=%d effective_bytes=%d force=1\\n", c.diagRCVBufEffective, effective)\n\t} else {\n\t\t_ = conn.SetReadBuffer(4 << 20)\n\t}\n\t_ = conn.SetWriteBuffer(4 << 20)'''
if s.count(old) != 1:
    raise SystemExit(f"buffer marker drift: {s.count(old)}")
s = s.replace(old, new, 1)
p.write_text(s)
print("WBD_SERVER_LINK_RCVBUF_DIAG_PATCHED isolated_socket=1")
