#!/usr/bin/env bash
set -u
OUT=${1:?usage: collect_link_hotspot_profile.sh OUT_DIR}
mkdir -p "$OUT/pprof"
P="$OUT/pprof"

find_link_pid() {
  python3 - <<'PY'
import os,pathlib
for x in pathlib.Path('/proc').iterdir():
    if not x.name.isdigit(): continue
    try:
        if os.path.basename(os.readlink(x/'exe')) == 'wbd-link-server-mux':
            print(x.name); break
    except OSError: pass
PY
}

pid=""
for _ in $(seq 1 900); do
  pid=$(find_link_pid)
  [[ -n "$pid" ]] && break
  sleep 1
 done
if [[ -z "$pid" ]]; then echo 'WBD_LINK_PROFILE_FAIL reason=pid_not_found' | tee "$P/status.txt"; exit 31; fi
echo "WBD_LINK_PROFILE_PID pid=$pid" | tee "$P/status.txt"
cp "/proc/$pid/exe" "$P/wbd-link-server-mux" || exit 32
chmod a+r "$P/wbd-link-server-mux"

{
  echo "pid=$pid"
  echo "nproc=$(nproc)"
  echo "cpu_max=$(cat /sys/fs/cgroup/cpu.max 2>/dev/null || echo unavailable)"
  echo "cpuset_effective=$(cat /sys/fs/cgroup/cpuset.cpus.effective 2>/dev/null || echo unavailable)"
  echo "server_cgroup=$(tr '\n' ';' </proc/$pid/cgroup 2>/dev/null || true)"
  echo "server_cpus_allowed=$(awk '/Cpus_allowed_list/{print $2}' /proc/$pid/status 2>/dev/null)"
  echo "model_name=$(awk -F: '/model name/{gsub(/^ +/,"",$2); print $2; exit}' /proc/cpuinfo)"
  echo '--- lscpu ---'
  lscpu
} >"$P/runner-cpu.txt"

# Wait for the fixed 20% spike rather than guessing from process startup time.
spike=0
for _ in $(seq 1 180); do
  q=$(sudo nsenter -t "$pid" -n tc qdisc show 2>/dev/null || true)
  if printf '%s\n' "$q" | grep -Eq 'loss( random)? 20([.]0+)?%'; then
    spike=1; printf '%s\n' "$q" >"$P/qdisc-profile-start.txt"; break
  fi
  sleep 1
 done
if [[ "$spike" != 1 ]]; then echo 'WBD_LINK_PROFILE_FAIL reason=spike20_not_seen' | tee -a "$P/status.txt"; exit 33; fi

for _ in $(seq 1 30); do
  if sudo nsenter -t "$pid" -n curl -fsS --max-time 2 http://127.0.0.1:6060/debug/pprof/ >/dev/null 2>&1; then break; fi
  sleep .2
 done
if ! sudo nsenter -t "$pid" -n curl -fsS --max-time 2 http://127.0.0.1:6060/debug/pprof/ >/dev/null 2>&1; then
  echo 'WBD_LINK_PROFILE_FAIL reason=pprof_not_ready' | tee -a "$P/status.txt"; exit 34
fi

proc_ticks() { awk '{p=index($0,") "); s=substr($0,p+2); split(s,a," "); print a[12]+a[13]}' "/proc/$pid/stat" 2>/dev/null || echo 0; }
echo "$(proc_ticks)" >"$P/link-cpu-ticks-start.txt"
profile_start=$(date +%s%N)
sudo nsenter -t "$pid" -n curl -fsS --max-time 30 'http://127.0.0.1:6060/debug/pprof/profile?seconds=20' >"$P/cpu.pprof" || exit 35
profile_end=$(date +%s%N)
echo "$(proc_ticks)" >"$P/link-cpu-ticks-end.txt"

sudo nsenter -t "$pid" -n curl -fsS --max-time 5 http://127.0.0.1:6060/debug/pprof/allocs >"$P/allocs.pprof" || true
sudo nsenter -t "$pid" -n curl -fsS --max-time 5 http://127.0.0.1:6060/debug/pprof/mutex >"$P/mutex.pprof" || true
sudo nsenter -t "$pid" -n curl -fsS --max-time 5 http://127.0.0.1:6060/debug/pprof/block >"$P/block.pprof" || true
sudo nsenter -t "$pid" -n curl -fsS --max-time 5 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=2' >"$P/goroutines.txt" || true
sudo nsenter -t "$pid" -n tc qdisc show >"$P/qdisc-profile-end.txt" 2>&1 || true

set +e
go tool pprof -top -cum -nodecount=40 "$P/wbd-link-server-mux" "$P/cpu.pprof" >"$P/cpu-top.txt" 2>&1
[[ -s "$P/allocs.pprof" ]] && go tool pprof -top -cum -nodecount=40 -sample_index=alloc_space "$P/wbd-link-server-mux" "$P/allocs.pprof" >"$P/allocs-top.txt" 2>&1
[[ -s "$P/mutex.pprof" ]] && go tool pprof -top -cum -nodecount=40 -sample_index=delay "$P/wbd-link-server-mux" "$P/mutex.pprof" >"$P/mutex-top.txt" 2>&1
[[ -s "$P/block.pprof" ]] && go tool pprof -top -cum -nodecount=40 -sample_index=delay "$P/wbd-link-server-mux" "$P/block.pprof" >"$P/block-top.txt" 2>&1
set -e

python3 - "$P/profile-meta.json" "$profile_start" "$profile_end" <<'PY'
import json,sys
p,a,b=sys.argv[1],int(sys.argv[2]),int(sys.argv[3])
obj={
 'cpu_profile_requested_sec':20,
 'cpu_profile_actual_sec':(b-a)/1e9,
 'load_window_sec':120,
 'cpu_profile_fraction_of_load_window':20/120,
 'mutex_profile_fraction':5,
 'block_profile_rate_ns':1000000,
 'socket_thread_sampler_interval_sec':1.0,
 'notes':'pprof is test-only; CPU profile active only during 20pct spike; alloc/mutex/block are snapshots after CPU sample'
}
open(p,'w').write(json.dumps(obj,indent=2,sort_keys=True)+'\n')
PY
echo 'WBD_LINK_PROFILE_PASS spike=20 cpu_seconds=20' | tee -a "$P/status.txt"
