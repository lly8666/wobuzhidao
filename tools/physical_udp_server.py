"""Controlled physical UDP target: bounded summaries, no packet captures."""
import argparse, json, os, socket, struct, threading, time
from pathlib import Path

SIZES = (96, 256, 512, 1000, 1372)
PATTERN = bytes(range(256)) * 6

def cpu(pid):
    fields = Path('/proc/%s/stat' % pid).read_text().split(') ', 1)[1].split()
    return (int(fields[11]) + int(fields[12])) / os.sysconf('SC_CLK_TCK')

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--bind', default='198.18.0.1')
    ap.add_argument('--port', type=int, default=18445)
    ap.add_argument('--output', required=True)
    ap.add_argument('--wbd-pid', type=int, required=True)
    args = ap.parse_args()
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 2 * 1024 * 1024)
        sock.bind((args.bind, args.port)); sock.settimeout(30)
        hello, peer = sock.recvfrom(4096)
        if not hello.startswith(b'P7H '): raise ValueError('Expected qualification hello')
        config = json.loads(hello[4:]); duration = int(config['Seconds']); seed = int(config['Seed'])
        rate = float(config['Mbps']) * 1e6 / 8
        if not 1 <= duration <= 180 or not 0 < rate <= 5e6: raise ValueError('Out of bounded test range')
        sock.sendto(b'P7A', peer)
        while True:
            data, address = sock.recvfrom(4096)
            if address != peer: continue
            if data == b'P7G': break
            if data.startswith(b'P7H '): sock.sendto(b'P7A', peer)
        seen = bytearray(1000000)
        stats = dict(RxPackets=0, RxBytes=0, DuplicatePackets=0, BadPayload=0, ReorderedPackets=0,
                     TxPackets=0, TxBytes=0, MaxSendLagMs=0.0)
        start = time.monotonic(); cpu_before = cpu(args.wbd_pid); helper_cpu_before = time.process_time()
        maximum = -1; finished = threading.Event()
        def send():
            sequence = 0; buffers = {n: bytearray(n) for n in SIZES}
            for n, b in buffers.items(): b[16:] = PATTERN[:n-16]
            while time.monotonic()-start < duration:
                elapsed = time.monotonic()-start; budget = elapsed * rate; batch = 0
                while stats['TxBytes'] + SIZES[sequence % len(SIZES)] <= budget and batch < 32:
                    n = SIZES[sequence % len(SIZES)]; b = buffers[n]
                    struct.pack_into('!4sIIHBB', b, 0, b'P7D1', seed, sequence, n, 1, 0)
                    sock.sendto(b, peer); stats['TxBytes'] += n; stats['TxPackets'] += 1
                    stats['MaxSendLagMs'] = max(stats['MaxSendLagMs'], (elapsed-stats['TxBytes']/rate)*1000)
                    sequence += 1; batch += 1
                time.sleep(.001)
            finished.set()
        sender = threading.Thread(target=send); sender.start(); sock.settimeout(.2)
        while time.monotonic()-start < duration+3:
            try: data, address = sock.recvfrom(4096)
            except socket.timeout: continue
            if address != peer: continue
            if data.startswith(b'P7P'):
                sock.sendto(data, peer); continue
            if not data.startswith(b'P7D1'): continue
            if len(data) < 16: stats['BadPayload'] += 1; continue
            magic, stream, seq, n, direction, reserved = struct.unpack('!4sIIHBB', data[:16])
            if stream != seed or seq >= len(seen) or n != len(data) or n not in SIZES or direction != 0 or data[16:] != PATTERN[:n-16]:
                stats['BadPayload'] += 1; continue
            if seen[seq]: stats['DuplicatePackets'] += 1; continue
            seen[seq] = 1
            if seq < maximum: stats['ReorderedPackets'] += 1
            maximum = max(maximum, seq); stats['RxPackets'] += 1; stats['RxBytes'] += n
        sender.join()
        stats.update(Result='MEASURED', Seed=seed, Seconds=duration, RequestedMbps=config['Mbps'],
                     PeerIPv4=peer[0], WBDCPUSeconds=cpu(args.wbd_pid)-cpu_before,
                     HelperCPUSeconds=time.process_time()-helper_cpu_before,
                     EffectiveTargetReceiveBuffer=sock.getsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF),
                     PayloadSizes=list(SIZES), NoPcap=True)
        Path(args.output).write_text(json.dumps(stats, indent=2))
        reply=b'P7R '+json.dumps(stats).encode()
        # Reliable retrieval is bounded and kept out of business counters.
        deadline=time.monotonic()+8
        while time.monotonic()<deadline:
            sock.sendto(reply, peer)
            try: data, address=sock.recvfrom(4096)
            except socket.timeout: continue
            if address==peer and data==b'P7DONE': break

if __name__ == '__main__': main()
