#!/usr/bin/env python3
"""Render a raw terminal byte stream to a text grid (best-effort CSI replay).

Usage: ansi-grid.py <file> [rows] [cols]
"""
import re
import sys

path = sys.argv[1]
rows = int(sys.argv[2]) if len(sys.argv) > 2 else 30
cols = int(sys.argv[3]) if len(sys.argv) > 3 else 100

data = open(path, "rb").read()

scr = [[" "] * cols for _ in range(rows)]
r = c = 0

CSI_FINAL = set(b"ABCDEFGHJKSTfmnsuhlprq@`dGXZ")
i = 0
n = len(data)
while i < n:
    b = data[i]
    if b == 0x1B:
        if i + 1 < n and data[i + 1] == ord("]"):
            m = re.match(rb"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)", data[i:])
            i += m.end() if m else 2
            continue
        if i + 1 < n and data[i + 1] == ord("P"):
            m = re.match(rb"\x1bP[\s\S]*?\x1b\\", data[i:])
            i += m.end() if m else 2
            continue
        if i + 1 < n and data[i + 1] == ord("["):
            m = re.match(rb"\x1b\[([0-9;?<>]*)([A-Za-z@`])", data[i:])
            if not m:
                i += 2
                continue
            params = m.group(1).decode("latin1")
            final = m.group(2).decode("latin1")
            i += m.end()
            nums = [int(x) for x in re.findall(r"\d+", params)] or [0]
            if final == "H" or final == "f":
                r = (nums[0] or 1) - 1
                c = (nums[1] if len(nums) > 1 else 1) - 1
            elif final == "A":
                r -= nums[0] or 1
            elif final == "B":
                r += nums[0] or 1
            elif final == "C":
                c += nums[0] or 1
            elif final == "D":
                c -= nums[0] or 1
            elif final == "G":
                c = (nums[0] or 1) - 1
            elif final == "d":
                r = (nums[0] or 1) - 1
            elif final == "J":
                if (nums[0] if nums else 0) == 2:
                    scr = [[" "] * cols for _ in range(rows)]
                    r = c = 0
            elif final == "K":
                if 0 <= r < rows:
                    for x in range(c, cols):
                        scr[r][x] = " "
            continue
        if i + 1 < n and data[i + 1] in (ord("("), ord(")")):
            i += 3
            continue
        i += 1
        continue
    if b == 0x0A:
        r += 1
        i += 1
        continue
    if b == 0x0D:
        c = 0
        i += 1
        continue
    if b == 0x08:
        c = max(0, c - 1)
        i += 1
        continue
    if b == 0x07:
        i += 1
        continue
    # printable: decode one rune
    for ln in (4, 3, 2, 1):
        try:
            s = data[i : i + ln].decode("utf-8")
            break
        except UnicodeDecodeError:
            continue
    else:
        s, ln = "?", 1
    if 0 <= r < rows and 0 <= c < cols:
        scr[r][c] = s
    c += 1
    i += ln

for idx, line in enumerate(scr):
    print("%2d |%s|" % (idx + 1, "".join(line).rstrip()))
