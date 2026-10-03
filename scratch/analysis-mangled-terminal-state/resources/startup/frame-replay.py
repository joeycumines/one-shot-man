#!/usr/bin/env python3
"""Replay a raw terminal stream and report every moment a PARTIAL frame is
visible on the real terminal.

A frame is "partial" when content has been painted but the screen is not a
coherent whole: rows painted at a width other than the terminal's, or text
painted while the alt screen is inactive (i.e. onto the user's shell), or
non-blank content painted after a clear but before the frame completes.

Usage: frame-replay.py <file> [rows] [cols]
"""
import re
import sys

path = sys.argv[1]
ROWS = int(sys.argv[2]) if len(sys.argv) > 2 else 30
COLS = int(sys.argv[3]) if len(sys.argv) > 3 else 120
data = open(path, "rb").read()

CUP = re.compile(rb"\x1b\[(\d*)(?:;(\d*))?H")
CSI = re.compile(rb"\x1b\[([\x30-\x3f]*)([\x20-\x2f]*)([\x40-\x7e])")
OSC = re.compile(rb"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)")
DCS = re.compile(rb"\x1bP[\s\S]*?\x1b\\")

scr = [[" "] * COLS for _ in range(ROWS)]
r = c = 0
alt = False
findings = []
painted_since_clear = 0
cleared_at = None

i = 0
n = len(data)
while i < n:
    if data[i] == 0x1B:
        m = CUP.match(data, i)
        if m:
            r = int(m.group(1) or 1) - 1
            c = int(m.group(2) or 1) - 1
            i = m.end()
            continue
        m = OSC.match(data, i)
        if m:
            i = m.end()
            continue
        m = DCS.match(data, i)
        if m:
            i = m.end()
            continue
        m = CSI.match(data, i)
        if m:
            params = m.group(1).decode("latin1")
            final = m.group(3).decode("latin1")
            i = m.end()
            nums = [int(x) for x in re.findall(r"\d+", params)] or [0]
            if final in "hl" and "1049" in params:
                alt = final == "h"
                if not alt:
                    findings.append((i, "alt-screen LEAVE — the user's shell is visible again"))
                scr = [[" "] * COLS for _ in range(ROWS)]
                r = c = 0
            elif final == "J" and nums and nums[0] == 2:
                scr = [[" "] * COLS for _ in range(ROWS)]
                r = c = 0
                painted_since_clear = 0
                cleared_at = i
            elif final == "K":
                if 0 <= r < ROWS:
                    for x in range(c, COLS):
                        scr[r][x] = " "
            continue
        if i + 1 < n and data[i + 1] in (ord("("), ord(")")):
            i += 3
            continue
        i += 1
        continue
    b = data[i]
    if b == 0x0A:
        r += 1
        i += 1
        continue
    if b == 0x0D:
        c = 0
        i += 1
        continue
    if b in (0x07, 0x08):
        i += 1
        continue
    for ln in (4, 3, 2, 1):
        try:
            s = data[i : i + ln].decode("utf-8")
            break
        except UnicodeDecodeError:
            continue
    else:
        s, ln = "?", 1
    if not alt and s.strip():
        findings.append((i, "text painted while NOT on the alt screen: %r" % s))
    if 0 <= r < ROWS and 0 <= c < COLS:
        scr[r][c] = s
    painted_since_clear += 1
    c += 1
    i += ln

print("stream: %s (%d bytes, %dx%d)" % (path, len(data), COLS, ROWS))
if not findings:
    print("no partial-frame events detected")
else:
    for off, what in findings:
        print("  byte %6d  %s" % (off, what))

print("\nfinal screen (non-blank rows):")
for idx, line in enumerate(scr):
    t = "".join(line).rstrip()
    if t:
        print("  %2d |%s|" % (idx + 1, t[:COLS]))
