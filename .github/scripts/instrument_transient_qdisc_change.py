#!/usr/bin/env python3
import pathlib
import sys

product=pathlib.Path(sys.argv[1])
rotation=product/'scripts/game_lane_rotation_soak.sh'
s=rotation.read_text()
changes={
'''  sudo ip netns exec "$C" tc qdisc replace dev gc0 root netem limit 24000 delay "${NETEM_DELAY_MS}ms" loss random "${loss}%"''':
'''  sudo ip netns exec "$C" tc qdisc change dev gc0 root netem limit 24000 delay "${NETEM_DELAY_MS}ms" loss random "${loss}%"''',
'''  sudo ip netns exec "$S" tc qdisc replace dev gs0 root netem limit 24000 delay "${NETEM_DELAY_MS}ms" loss random "${loss}%"''':
'''  sudo ip netns exec "$S" tc qdisc change dev gs0 root netem limit 24000 delay "${NETEM_DELAY_MS}ms" loss random "${loss}%"''',
}
for old,new in changes.items():
    if s.count(old)!=1:
        raise SystemExit(f'transient qdisc change marker drift: {old}: {s.count(old)}')
    s=s.replace(old,new,1)
rotation.write_text(s)
print('WBD_TRANSIENT_QDISC_CHANGE_PATCHED preserve_existing_queue=1')
