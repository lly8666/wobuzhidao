"""Stream closed, non-overwritten pcap chunks; preserve compressed raw evidence."""
import argparse
import gzip
import hashlib
import json
import struct
from pathlib import Path


def read_chunk(path,start_unix,seconds):
    digest=hashlib.sha256(); counts=[0]*seconds; wire=[0]*seconds
    bad=packets=0
    with open(path,'rb') as f:
        head=f.read(24); digest.update(head)
        if len(head)!=24 or head[:4] not in [b'\xd4\xc3\xb2\xa1',b'\xa1\xb2\xc3\xd4']:
            raise ValueError('unsupported/truncated pcap header')
        e='<' if head[:4]==b'\xd4\xc3\xb2\xa1' else '>'
        _,_,_,_,_,snap,link=struct.unpack(e+'IHHIIII',head)
        if link!=1 or snap!=128: raise ValueError('wrong capture format/snaplen')
        while True:
            h=f.read(16)
            if not h: break
            if len(h)!=16: raise ValueError('truncated packet header')
            sec,usec,n,original=struct.unpack(e+'IIII',h)
            if n>128: raise ValueError('unbounded captured frame')
            frame=f.read(n); digest.update(h);digest.update(frame)
            if len(frame)!=n: raise ValueError('truncated captured frame')
            packets+=1
            if n<54 or frame[12:14]!=b'\x08\x00': bad+=1;continue
            ip=frame[14:]; ih=(ip[0]&15)*4
            if ip[0]>>4!=4 or ip[9]!=6 or len(ip)<ih+20: bad+=1;continue
            total,frag=struct.unpack('!HH',ip[2:4]+ip[6:8]); tcp=ip[ih:]; th=(tcp[12]>>4)*4
            if total>1400 or frag&0x3fff or th<20 or total<ih+th or len(tcp)<th: bad+=1;continue
            rel=int((sec*1_000_000_000+usec*1000-start_unix)//1_000_000_000)
            if 0<=rel<seconds:
                counts[rel]+=1; wire[rel]+=total
    return dict(path=path.name,sha256=digest.hexdigest(),raw_bytes=path.stat().st_size,
                packets=packets,bad_headers=bad,packets_by_second=counts,ip_bytes_by_second=wire)


def main():
    a=argparse.ArgumentParser();a.add_argument('--artifact-dir',required=True);a.add_argument('--duration',type=int,required=True)
    x=a.parse_args();root=Path(x.artifact_dir);start=int((root/'start-unix-ns.txt').read_text())
    seconds=x.duration+61; result={};total=0
    for point in ['c2s-pre','c2s-post','s2c-pre','s2c-post']:
        chunks=sorted((root/'capture').glob(point+'-*.pcap'))
        if not chunks or len(chunks)>((x.duration+120)//15+2): raise ValueError('missing/excess capture chunks')
        rows=[];counts=[0]*seconds;wire=[0]*seconds
        for p in chunks:
            if p.stat().st_size>100*1024*1024: raise ValueError('capture chunk exceeded declared100MiB bound')
            r=read_chunk(p,start,seconds);total+=r['raw_bytes']
            if total>16*1024**3: raise ValueError('capture exceeded declared16GiB disk budget')
            for i,v in enumerate(r.pop('packets_by_second')):counts[i]+=v
            for i,v in enumerate(r.pop('ip_bytes_by_second')):wire[i]+=v
            gz=p.with_suffix(p.suffix+'.gz')
            if gz.exists(): raise ValueError('capture archive overwrite forbidden')
            with p.open('rb') as src,gzip.open(gz,'wb',compresslevel=1) as dst:
                while data:=src.read(1024*1024):dst.write(data)
            h=hashlib.sha256()
            with gz.open('rb') as f:
                while data:=f.read(1024*1024):h.update(data)
            r.update(archive=gz.name,archive_sha256=h.hexdigest(),archive_bytes=gz.stat().st_size)
            p.unlink();rows.append(r)
        result[point]=dict(chunks=rows,packets_by_second=counts,ip_bytes_by_second=wire)
    (root/'capture-receipt.json').write_text(json.dumps(dict(snaplen=128,chunk_seconds=15,raw_bytes=total,points=result),indent=2)+'\n')


if __name__=='__main__':main()
