#!/bin/env python3

from pwn import *
import sys

BINARY = "chall_patched"
context.binary = exe = ELF(BINARY, checksec=False)
context.terminal = "konsole -e".split()
context.log_level = "INFO"
context.bits = 64
context.arch = "amd64"

def init():
    if args.RMT:
        p = remote(sys.argv[1], sys.argv[2])
    else:
        p = process()
    return Exploit(p), p


class Exploit:
    def __init__(self, p: process):
        self.p = p

    def debug(self, script=None):
        if not args.RMT and args.DBG:
            if script:
                attach(self.p, "\n".join(script))
            else:
                attach(self.p)


x, p = init()
x.debug((
    "source /usr/share/pwngdb/.gdbinit",
    "source /usr/share/peda/peda.py",
))

p.interactive()
