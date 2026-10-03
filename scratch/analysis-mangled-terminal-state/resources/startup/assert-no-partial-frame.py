#!/usr/bin/env python3
"""Assert that a raw terminal byte stream never paints a partial frame.

Usage: assert-no-partial-frame.py <file> [--cols N] [--rows N]

Reads the stream from process start and answers one question: does any
printable character reach the terminal BEFORE the first complete paint?

A "complete paint" is a clear-screen (ESC[2J) followed by content. Anything
printed before that is, by definition, a partial frame: the TUI has not yet
taken ownership and whatever lands there is either visible garbage or something
the later full paint has to overwrite.

Exit status is 0 when the stream is clean and 1 when it is not, so the check
can gate a release.
"""
import re
import sys

path = sys.argv[1]
COLS = 120
ROWS = 30
for i, a in enumerate(sys.argv):
    if a == "--cols" and i + 1 < len(sys.argv):
        COLS = int(sys.argv[i + 1])
    if a == "--rows" and i + 1 < len(sys.argv):
        ROWS = int(sys.argv[i + 1])

data = open(path, "rb").read()

OSC = re.compile(rb"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)")
DCS = re.compile(rb"\x1bP[\s\S]*?\x1b\\")
# CSI = ESC [ params (0x30-0x3F) intermediates (0x20-0x2F) final (0x40-0x7E).
# The intermediate byte matters: bubbletea emits DECRQM probes such as
# ESC[?2026$p, whose '$' is an intermediate, and a regex that only allows a
# letter final would leave "[?2026$p" behind and report it as printable text.
CSI = re.compile(rb"\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]")

# Where the first complete paint starts: a clear-screen that is followed by
# actual content.
first_paint = None
for m in re.finditer(rb"\x1b\[2J", data):
    rest = data[m.end():]
    stripped = OSC.sub(b"", rest[:4096])
    stripped = DCS.sub(b"", stripped)
    stripped = CSI.sub(b"", stripped)
    if any(0x21 <= b < 0x7F for b in stripped):
        first_paint = m.start()
        break

head = data if first_paint is None else data[:first_paint]

# Everything in the head that is not an escape sequence.
text = OSC.sub(b"", head)
text = DCS.sub(b"", text)
text = CSI.sub(b"", text)
text = re.sub(rb"\x1b[()][A-Za-z0-9]", b"", text)
printable = [b for b in text if 0x20 <= b < 0x7F]

print("stream          : %s" % path)
print("bytes           : %d" % len(data))
print("first full paint: %s" % ("byte %d" % first_paint if first_paint is not None else "NONE"))
print("head bytes      : %d" % len(head))
print("printable in head: %r" % bytes(printable))
print("head escapes    : %r" % head)

if first_paint is None:
    print("\nFAIL: the stream never paints a complete frame")
    sys.exit(1)
if printable:
    print("\nFAIL: %d printable byte(s) precede the first full paint" % len(printable))
    sys.exit(1)

print("\nPASS: no printable byte precedes the first full paint")
sys.exit(0)
