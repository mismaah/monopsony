"""Regenerates the reference symbols in src/invite/qr.fixtures.json.

Our encoder is checked against two independent implementations, because each
one has a quirk of its own:

  * python-qrcode places the data exactly as ISO/IEC 18004 describes, but its
    mask-penalty scoring is approximate, so it sometimes settles on a mask the
    spec would not choose.
  * segno scores masks to the letter of the spec, but appends a stray zero pad
    codeword when the bit stream already ends on a codeword boundary, which
    byte-mode streams always do.

So: take python-qrcode's modules, and let segno's scoring pick the mask. The
fixture is then the symbol the standard describes, with no input from us.

    pip install qrcode segno && python scripts/qr-fixtures.py
"""

import io
import json
import os

import qrcode
from qrcode.constants import ERROR_CORRECT_M
from qrcode.util import MODE_8BIT_BYTE, QRData
from segno import encoder

PAYLOADS = [
    "https://monopsony.game/join/ABCD-2345",
    "a",
    "http://localhost:5173/join/7Q4X-M2LP",
    "https://a-rather-long-hostname.example.com/join/ZZZZ-7777?ref=lobby",
    "Mono the octopus — ünïcøde ✓",
    "x" * 200,
    "".join(chr(33 + (i * 7) % 90) for i in range(660)),  # fills version 20
]

OUT = os.path.join(os.path.dirname(__file__), "..", "src", "invite", "qr.fixtures.json")


def scored(modules):
    """segno's penalty for a symbol, with the info areas blanked as the spec asks."""
    size = len(modules)
    matrix = tuple(bytearray(1 if m else 0 for m in row) for row in modules)
    for i in range(9):
        if i != 6:  # the timing patterns cross the format strip and stay put
            matrix[i][8] = 0
            matrix[8][i] = 0
    for i in range(1, 9):
        matrix[size - i][8] = 0
        matrix[8][size - i] = 0
    if size >= 45:  # version 7 and up carry version information
        for i in range(6):
            for d in range(9, 12):
                matrix[i][size - d] = 0
                matrix[size - d][i] = 0
    return encoder.evaluate_mask(matrix, size, size)


def symbol(text):
    q = qrcode.QRCode(error_correction=ERROR_CORRECT_M, border=0, box_size=1)
    q.add_data(QRData(text.encode("utf-8"), mode=MODE_8BIT_BYTE, check_data=False))
    q.best_fit()
    best = None
    for mask in range(8):
        q.makeImpl(False, mask)
        modules = [list(row) for row in q.modules]
        score = scored(modules)
        if best is None or score < best[0]:
            best = (score, mask, modules)
    _, mask, modules = best
    rows = ["".join("#" if m else "." for m in row) for row in modules]
    return {"version": q.version, "mask": mask, "text": text, "rows": rows}


fixtures = [symbol(p) for p in PAYLOADS]
for f in fixtures:
    print("version", f["version"], "mask", f["mask"], "size", len(f["rows"]))
with io.open(OUT, "w", encoding="utf-8", newline="\n") as fh:
    fh.write(json.dumps(fixtures, indent=1, ensure_ascii=False) + "\n")
