"""Audit bounded native capture windows; emit metadata, never payload bytes.

Reuse the existing P2 PCAP/checksum reader. Partial windows are explicitly
inconclusive for whole-stream/TLS semantics and never prove no-HOL.
"""
import argparse, collections, hashlib, json, struct
from pathlib import Path
from check_p2_pcap import read_pcap, l3_payload, parse_ipv4_tcp

def hello(data, server=False):
    out = {}
    try:
        # A plaintext handshake message may span TLS records; caller joins them.
        if len(data) < 4 or data[0] != (2 if server else 1): return out
        end = 4 + int.from_bytes(data[1:4], 'big')
        if end > len(data): return {'incomplete': True}
        b = data[4:end]; version = int.from_bytes(b[:2], 'big'); i = 34
        i += 1+b[i]; ciphers = []
        if server:
            ciphers = [int.from_bytes(b[i:i+2], 'big')]; i += 3
        else:
            n = int.from_bytes(b[i:i+2], 'big'); i += 2
            ciphers = [int.from_bytes(b[j:j+2], 'big') for j in range(i, i+n, 2)]
            i += n; i += 1+b[i]
        extlen = int.from_bytes(b[i:i+2], 'big'); i += 2; limit = i+extlen
        extensions = []; groups = []; points = []; names = []; alpn = []; versions = []
        while i+4 <= limit:
            kind, size = struct.unpack('!HH', b[i:i+4]); i += 4
            x = b[i:i+size]; i += size; extensions.append(kind)
            if kind == 0 and len(x) >= 5:
                n = int.from_bytes(x[3:5], 'big'); names.append(x[5:5+n].decode('ascii', 'replace'))
            elif kind == 10 and len(x) >= 2:
                groups = [int.from_bytes(x[j:j+2], 'big') for j in range(2, len(x), 2)]
            elif kind == 11 and x: points = list(x[1:1+x[0]])
            elif kind == 16 and len(x) >= 2:
                j = 2
                while j < len(x):
                    n = x[j]; j += 1; alpn.append(x[j:j+n].decode('ascii', 'replace')); j += n
            elif kind == 43:
                start = 0 if server else 1
                versions = [int.from_bytes(x[j:j+2], 'big') for j in range(start, len(x), 2)]
        nongrease = lambda ns: [n for n in ns if (n & 0x0f0f) != 0x0a0a]
        parts = [str(version), '-'.join(map(str, nongrease(ciphers))), '-'.join(map(str, nongrease(extensions)))]
        if not server: parts += ['-'.join(map(str, nongrease(groups))), '-'.join(map(str, points))]
        fingerprint = ','.join(parts)
        out.update(legacy_version=version, ciphers=ciphers, extensions=extensions,
                   supported_versions=versions, sni=names, alpn_offered=alpn,
                   ja3s_md5=hashlib.md5(fingerprint.encode()).hexdigest() if server else None,
                   ja3_md5=None if server else hashlib.md5(fingerprint.encode()).hexdigest())
    except (IndexError, struct.error, ValueError): out['parse_error'] = True
    return out

def percentile(values, fraction):
    return sorted(values)[min(len(values)-1, int((len(values)-1)*fraction))] if values else None

def audit(path, local, port):
    if path.stat().st_size > 16*1024*1024: raise ValueError('capture exceeds 16MiB per-window bound')
    network, frames = read_pcap(path); flows = {}; bad_parse = 0; truncated = 0
    for ts, frame, orig in frames:
        if len(frame) != orig: truncated += 1
        ip = l3_payload(network, frame)
        if ip is None: continue
        p = parse_ipv4_tcp(ts, ip)
        if p is None: bad_parse += 1; continue
        p['ipv4_length']=int.from_bytes(ip[2:4],'big')
        if p['src'] == local and p['sport'] == port: direction='S2C'; peer_port=p['dport']
        elif p['dst'] == local and p['dport'] == port: direction='C2S'; peer_port=p['sport']
        else: continue
        # Separate the peer IP internally; summaries intentionally omit addresses.
        peer = p['dst'] if direction == 'S2C' else p['src']
        f = flows.setdefault((peer,peer_port,direction), dict(packets=[],segments=[]))
        f['packets'].append(p)
        if p['payload']: f['segments'].append((p['seq'], p['payload']))
    summaries = []
    for (_, peer_port, direction), f in flows.items():
        ps = f['packets']; chunks=[]; mismatch=0; exact_repeats=0; seen={}; high=None; out_of_order=0
        for seq, payload in f['segments']:
            key=(seq,len(payload)); prior=seen.get(key)
            if prior is not None:
                exact_repeats+=1
                if prior != payload: mismatch+=1
            else: seen[key]=payload
            if high is not None and seq < high: out_of_order+=1
            high=max(high or seq, seq+len(payload))
        # These short windows are <2GiB per flow; sort relative to first observed seq for wrap.
        anchor=f['segments'][0][0] if f['segments'] else 0
        segments=sorted((((seq-anchor)&0xffffffff, payload) for seq,payload in f['segments']),key=lambda x:x[0])
        base=None; buf=bytearray()
        for seq,payload in segments:
            if base is None or seq > base+len(buf):
                if buf: chunks.append(bytes(buf))
                base=seq; buf=bytearray(payload); continue
            at=seq-base; common=min(len(payload),len(buf)-at)
            if bytes(buf[at:at+common]) != payload[:common]: mismatch+=1
            if common < len(payload): buf.extend(payload[common:])
        if buf: chunks.append(bytes(buf))
        types=collections.Counter(); versions=collections.Counter(); lengths=[]; plain=bytearray()
        unaligned=0; partial=0; invalid=0
        for chunk in chunks:
            if len(chunk)<5 or chunk[0] not in (20,21,22,23) or chunk[1]!=3:
                unaligned+=1; continue
            i=0
            while i+5<=len(chunk):
                ct,ver,n=struct.unpack('!BHH',chunk[i:i+5])
                if ct not in (20,21,22,23) or ver not in (0x0301,0x0302,0x0303) or not 0<n<=18432:
                    invalid+=1; break
                if i+5+n>len(chunk): partial+=1; break
                types[str(ct)]+=1; versions[hex(ver)]+=1; lengths.append(n)
                if ct==22: plain.extend(chunk[i+5:i+5+n])
                i+=5+n
            if 0<len(chunk)-i<5: partial+=1
        flags={label:sum(bool(p['flags']&mask) for p in ps) for label,mask in [('SYN',2),('ACK',16),('FIN',1),('RST',4)]}
        summaries.append(dict(direction=direction, peer_port=peer_port, packets=len(ps), flags=flags,
            max_ipv4_length=max(p['ipv4_length'] for p in ps),
            header_timeline=[dict(timestamp=p['ts'],seq=p['seq'],ack=p['ack'],flags=p['flags'],
                payload_length=len(p['payload']),window=p['window'])
                for i,p in enumerate(ps) if i<12 or p['flags']&7][:128],
            payload_packets=len(f['segments']), payload_bytes=sum(len(x[1]) for x in f['segments']),
            ipv4_checksum_bad=sum(not p['ip_checksum_ok'] for p in ps),
            tcp_checksum_bad=sum(not p['tcp_checksum_ok'] for p in ps),
            fragmented=sum(bool(p['mf'] or p['frag_offset']) for p in ps),
            syn_options=[p['options'] for p in ps if p['flags']&2][:4],
            exact_seq_length_repeats=exact_repeats, retransmission_or_overlap_bytes_conflict=mismatch,
            out_of_order_or_repeat_segments=out_of_order,
            tls_record_types=dict(types), tls_wire_versions=dict(versions),
            tls_complete_records=len(lengths), tls_length_p50=percentile(lengths,.5),tls_length_p95=percentile(lengths,.95),
            tls_length_max=max(lengths) if lengths else None, tls_unaligned_runs=unaligned,
            tls_partial_tail_runs=partial, tls_invalid_headers_in_aligned_runs=invalid,
            plaintext_hello=hello(bytes(plain),server=direction=='S2C')))
    return dict(pcap_bytes=path.stat().st_size, pcap_sha256=hashlib.sha256(path.read_bytes()).hexdigest(),
        captured_frames=len(frames), truncated_frames=truncated, parse_rejected_frames=bad_parse, flows=summaries,
        payload_exported=False, bounds='Window-only;mid-stream gaps/tails inconclusive;no-HOL needs business evidence',
        security_note='TLS1.3 tickets/encrypted extensions not observable without session secrets;not a website fingerprint equivalence proof')

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--pcap',required=True);ap.add_argument('--local-ip',required=True)
    ap.add_argument('--port',type=int,default=443);ap.add_argument('--output',required=True)
    args=ap.parse_args();result=audit(Path(args.pcap),args.local_ip,args.port)
    Path(args.output).write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))

if __name__=='__main__':main()
