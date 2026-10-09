#!/usr/bin/env python3
"""Reuse the existing strict 0%-loss three-stage receipt at genuine 15ms netem.

No duplicated loss/stage scheduler: replace only the qdisc delay argument in
the existing strict implementation. The enclosing harness configures 15ms
at startup too, so no stale 300ms interval is possible.
"""
import strict_weaknet_stage as strict

_original=strict.tc_args
def tc_args(ns,tc_bin,dev,loss,seed):
    if loss!=0:
        raise ValueError("E1 low-RTT dedicated protector must be lossless")
    cmd=_original(ns,tc_bin,dev,loss,seed)
    if cmd.count("300ms")!=1:
        raise ValueError("strict tc netem template changed unexpectedly")
    return ["15ms" if x=="300ms" else x for x in cmd]

strict.tc_args=tc_args
if __name__=="__main__":strict.main()
