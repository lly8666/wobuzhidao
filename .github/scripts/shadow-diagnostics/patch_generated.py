"""Patch only the final generated harness, after legacy wrapper validation."""
from pathlib import Path
import sys

p = Path(sys.argv[1])
assets = Path(__file__).resolve().parent
s = p.read_text()
def replace(old, new):
    global s
    assert s.count(old) == 1, repr(old)
    s = s.replace(old, new, 1)

start = s.index("cat >\"$LOG_DIR/load.py\" <<'PY_LOAD'\n")
end = s.index('\nPY_LOAD\n', start) + len('\nPY_LOAD\n')
s = s[:start] + 'cp "'+str(assets/'load.py')+'" "$LOG_DIR/load.py"\n' + s[end:]
replace("    with count.open('ab') as f: f.write(b.hex().encode()+b'\\n')", '    # No per-packet file I/O in the echo hot path.')
replace(': >"$LOG_DIR/rotation.log"', '''rm -f "$LOG_DIR/monitor.stop" "$LOG_DIR/monitor.ready"
python3 "'''+str(assets/'monitor.py')+'''" "$LOG_DIR" "$C" "$S" &
MONITOR_PID=$!; PIDS+=("$MONITOR_PID")
for _ in $(seq 1 100); do
  [[ -f "$LOG_DIR/monitor.ready" ]] && break
  kill -0 "$MONITOR_PID" || exit 1
  sleep .1
done
[[ -f "$LOG_DIR/monitor.ready" ]]
: >"$LOG_DIR/rotation.log"''')
replace('wait "$LOAD_PID"\ndrop_pid "$LOAD_PID"', '''load_rc=0
wait "$LOAD_PID" || load_rc=$?
touch "$LOG_DIR/monitor.stop"
wait "$MONITOR_PID"
drop_pid "$MONITOR_PID"
drop_pid "$LOAD_PID"
(( load_rc == 0 )) || exit "$load_rc"''')
p.write_text(s)
