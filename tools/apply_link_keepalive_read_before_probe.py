from pathlib import Path

p = Path('cmd/wbd-link-proxy/main.go')
s = p.read_text(encoding='utf-8')

old = '''\t\tnow := time.Now()\n\t\tsendPing, pingNonce, dead := liveness.poll(now, keepalive)\n\t\tif dead {\n\t\t\treturn fmt.Errorf("WBD link liveness timeout after %s without valid remote LINK activity", clientRemoteRXTimeout(keepalive))\n\t\t}\n\t\tif sendPing {\n\t\t\tif err := sendLifecycle(conn, dtlsAddr, control.Ping{Nonce: pingNonce}); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n\t\t}\n\t\t_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))\n\t\tn, from, err := conn.ReadFromUDP(buf)\n\t\tnow = time.Now()\n'''
new = '''\t\t// Read first so already-queued authenticated remote traffic gets a chance\n\t\t// to refresh liveness before we decide the path is idle. This avoids\n\t\t// emitting an idle PING solely because the goroutine was descheduled past\n\t\t// the deadline while remote business data was waiting in the socket.\n\t\t_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))\n\t\tn, from, err := conn.ReadFromUDP(buf)\n\t\tnow := time.Now()\n'''
if s.count(old) != 1:
    raise SystemExit(f'initial keepalive poll block count={s.count(old)}')
s = s.replace(old, new, 1)

old = '''\t\t\t\t}\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif isLifecycleControl(buf[:n]) {'''
new = '''\t\t\t\t}\n\t\t\t\tif err := serviceClientKeepalive(conn, dtlsAddr, &liveness, now, keepalive); err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif isLifecycleControl(buf[:n]) {'''
if s.count(old) != 1:
    raise SystemExit(f'startup continue block count={s.count(old)}')
s = s.replace(old, new, 1)

old = '''\t\t\t\tcase control.Pong:\n\t\t\t\t\tliveness.observePong(f.Nonce, now, keepalive)\n\t\t\t\t\tcontinue\n\t\t\t\tcase control.Ping:\n\t\t\t\t\tif err := sendLifecycle(conn, dtlsAddr, control.Pong{Nonce: f.Nonce}); err != nil {\n\t\t\t\t\t\treturn err\n\t\t\t\t\t}\n\t\t\t\t\tcontinue\n'''
new = '''\t\t\t\tcase control.Pong:\n\t\t\t\t\tliveness.observePong(f.Nonce, now, keepalive)\n\t\t\t\t\tif err := serviceClientKeepalive(conn, dtlsAddr, &liveness, now, keepalive); err != nil {\n\t\t\t\t\t\treturn err\n\t\t\t\t\t}\n\t\t\t\t\tcontinue\n\t\t\t\tcase control.Ping:\n\t\t\t\t\tif err := sendLifecycle(conn, dtlsAddr, control.Pong{Nonce: f.Nonce}); err != nil {\n\t\t\t\t\t\treturn err\n\t\t\t\t\t}\n\t\t\t\t\tif err := serviceClientKeepalive(conn, dtlsAddr, &liveness, now, keepalive); err != nil {\n\t\t\t\t\t\treturn err\n\t\t\t\t\t}\n\t\t\t\t\tcontinue\n'''
if s.count(old) != 1:
    raise SystemExit(f'lifecycle continue block count={s.count(old)}')
s = s.replace(old, new, 1)

old = '''\t\t}\n\t\twire, err := path.FlushDue(now)\n'''
new = '''\t\t}\n\t\tif err := serviceClientKeepalive(conn, dtlsAddr, &liveness, now, keepalive); err != nil {\n\t\t\treturn err\n\t\t}\n\t\twire, err := path.FlushDue(now)\n'''
# There are client/server loops with the same tail; only the first occurrence is client.
if s.count(old) < 1:
    raise SystemExit('client loop tail not found')
s = s.replace(old, new, 1)

marker = '''func serverDataLoop(conn *net.UDPConn, serviceAddr, dtlsPeer *net.UDPAddr, path *linkdata.Path, startup serverStartupSession, stop <-chan os.Signal) error {'''
helper = '''func serviceClientKeepalive(conn *net.UDPConn, dtlsAddr *net.UDPAddr, liveness *clientKeepaliveTracker, now time.Time, keepalive time.Duration) error {\n\tsendPing, pingNonce, dead := liveness.poll(now, keepalive)\n\tif dead {\n\t\treturn fmt.Errorf("WBD link liveness timeout after %s without valid remote LINK activity", clientRemoteRXTimeout(keepalive))\n\t}\n\tif sendPing {\n\t\treturn sendLifecycle(conn, dtlsAddr, control.Ping{Nonce: pingNonce})\n\t}\n\treturn nil\n}\n\n'''
if s.count(marker) != 1:
    raise SystemExit(f'server loop marker count={s.count(marker)}')
s = s.replace(marker, helper + marker, 1)

p.write_text(s, encoding='utf-8')
print('client keepalive now reads queued remote traffic before idle probing')
