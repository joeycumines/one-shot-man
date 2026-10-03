#!/usr/bin/env python3
"""Replay a raw terminal byte stream and report the first moment a PARTIAL
frame is visible on the real terminal.

A "partial frame" here is a non-blank cell painted while the alt screen is
active and before the first full-width row has been painted, or any write that
lands on the PRIMARY screen after the alt screen has been entered. The point is
to catch mangled output that the TUI's later full paint merely overwrites.

Usage: partial-frame.py <file> [rows] [cols]
"""
import re, sys

path = sys.argv[1]
ROWS = int(sys.argv[2]) if len(sys.argv) > 2 else 30
COLS = int(sys.argv[3]) if len(sys.argv) > 3 else 120
data = open(path, "rb").read()

scr = [[" "] * COLS for _ in range(ROWS)]
r = c = 0
alt = False
painted_any = False
first_paint_at = None
violations = []
i = 0
n = len(data)
while i < n:
    b = data[i]
    if b == 0x1B:
        m = re.match(rb"\x1b\[(\d+)?(?:;(\d+))?H", data[i:])
        if m:
            r = int(m.group(1) or 1) - 1
            c = int(m.group(2) or 1) - 1
            i += m.end(); continue
        m = re.match(rb"\x1b\[([0-9;?<>]*)([A-Za-z@`])", data[i:])
        if m:
            params, final = m.group(1).decode("latin1"), m.group(2).decode("latin1")
            nums = [int(x) for x in re.findall(r"\d+", params)] or [0]
            i += m.end()
            if final in "hl" and params.startswith("?1049"):
                alt = final == "h"
                scr = [[" "] * COLS for _ in range(ROWS)]
                r = c = 0
                violations.append((i, "alt-screen %s" % ("enter" if alt else "leave")))
            elif final == "J" and nums and nums[0] == 2:
                scr = [[" "] * COLS for _ in range(ROWS)]
                r = c = 0
            elif final == "K":
                if 0 <= r < ROWS:
                    for x in range(c, COLS):
                        scr[r][x] = " "
            continue
        m = re.match(rb"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)", data[i:])
        if m:
            i += m.end(); continue
        if i + 1 < n and data[i + 1] in (ord("("), ord(")")):
            i += 3; continue
        i += 1; continue
    if b == 0x0A:
        r += 1; i += 1; continue
    if b == 0x0D:
        c = 0; i += 1; continue
    if b in (0x07, 0x08):
        i += 1; continue
    for ln in (4, 3, 2, 1):
        try:
            s = data[i : i + ln].decode("utf-8"); break
        except UnicodeDecodeError:
            continue
    else:
        s, ln = "?", 1
    if 0 <= r < ROWS and 0 <= c < COLS:
        scr[r][c] = s
    if not painted_any and s.strip():
        painted_any = True
        first_paint_at = i
    c += 1
    i += ln

print("first non-blank paint at byte %s (of %d)" % (first_paint_at, n))
print("events:")
for off, what in violations:
    print("  byte %5d  %s" % (off, what))
# Screen at the first paint, and at the moment just before the first full row.
def show(tag):
    print(tag)
    for idx, line in enumerate(scr):
        t = "".join(line).rstrip()
        if t:
            print("   %2d |%s|" % (idx + 1, t[:110]))
show("final screen:")
