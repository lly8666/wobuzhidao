#!/usr/bin/env python3
"""Resolve one AF_PACKET SOCK_RAW receiver FD by kernel inode, fail closed.

A read-only helper for possible future diagnostics; does NOT trace processes,
send packets, change socket buffers or serialize fd/inode/address/payload.
The receive FD must be selected from the target process's network namespace.
"""
import os
from pathlib import Path
import re
from afpacket_schedstat import parse_starttime

MAX_FDS = 4096
SOCKET_LINK = re.compile(r"^socket:\[(\d+)\]$")


def packet_raw_inodes(text):
    lines=[line.split() for line in text.splitlines() if line.strip()]
    if not lines or not {"Inode","Type"}.issubset(lines[0]):
        raise ValueError("packet table absent or not recognized")
    inode_index=lines[0].index("Inode")
    type_index=lines[0].index("Type")
    found=set()
    for line in lines[1:]:
        if len(line)<=max(inode_index,type_index):
            raise ValueError("malformed packet table row")
        if line[type_index]=="3":   # Linux SOCK_RAW
            if not line[inode_index].isdigit():
                raise ValueError("non-numeric packet socket inode")
            found.add(int(line[inode_index]))
    return found


def select_receiver_fd(inodes, fd_targets):
    if len(fd_targets)>MAX_FDS:
        raise ValueError("too many open process descriptors")
    matches=[]
    for fd, link in fd_targets.items():
        if type(fd) is not int or fd<0:
            raise ValueError("invalid process descriptor")
        m=SOCKET_LINK.fullmatch(link)
        if m and int(m.group(1)) in inodes:
            matches.append(fd)
    if len(matches)!=1:
        raise ValueError("ambiguous or unavailable product packet receiver: count=%d" % len(matches))
    return matches[0]


def resolve(pid, expected_executable):
    if type(pid) is not int or pid<=0:
        raise ValueError("positive target pid required")
    if not expected_executable or "/" in expected_executable:
        raise ValueError("exact executable basename required")
    root=Path("/proc")/str(pid)
    try:
        exe=os.path.basename(os.readlink(root/"exe"))
        if exe!=expected_executable:
            raise ValueError("target executable mismatch")
        first=parse_starttime((root/"stat").read_text())
        packet=packet_raw_inodes((root/"net/packet").read_text())
        descriptors=[p for p in (root/"fd").iterdir() if p.name.isdigit()]
        if len(descriptors)>MAX_FDS:
            raise ValueError("FD scan cap exceeded")
        links={}
        for f in descriptors:
            try:
                links[int(f.name)]=os.readlink(f)
            except FileNotFoundError:
                # Churning process FD table cannot be mistaken for a unique socket.
                raise ValueError("FD churn during packet socket selection")
        candidate=select_receiver_fd(packet,links)
        second=parse_starttime((root/"stat").read_text())
        if first!=second:
            raise ValueError("process identity changed while selecting packet socket")
        if os.readlink(root/"fd"/str(candidate))!=links[candidate]:
            raise ValueError("packet socket FD reused while selecting")
        return candidate
    except OSError as exc:
        raise ValueError("packet receiver metadata unavailable") from exc
