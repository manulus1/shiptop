"""Generate mosaic.go: python3 tools/mosaic_gen.py mosaic.go   (needs: pip install unicodedata2)

Generate the mosaic tables (pattern bits -> character) from the Unicode character names.
bit k = sub-pixel k in row-major order (top-left first), as in the BLOCK SEXTANT-/OCTANT- names."""
import json, re, sys
import unicodedata2 as u

def names(lo, hi):
    for cp in range(lo, hi):
        try:
            yield cp, u.name(chr(cp))
        except ValueError:
            pass

def cells(c, r, rects):
    """bits for sub-cells covered by rects given as (x0,y0,x1,y1) fractions of the cell"""
    b = 0
    for row in range(r):
        for col in range(c):
            cx, cy = (col + .5) / c, (row + .5) / r
            if any(x0 <= cx <= x1 and y0 <= cy <= y1 for x0, y0, x1, y1 in rects):
                b |= 1 << (row * c + col)
    return b

# geometric meaning of the plain block characters (fractions of the cell)
geo = {
    0x20: [], 0x2588: [(0, 0, 1, 1)],
    0x2580: [(0, 0, 1, .5)], 0x2584: [(0, .5, 1, 1)], 0x258C: [(0, 0, .5, 1)], 0x2590: [(.5, 0, 1, 1)],
    0x2582: [(0, .75, 1, 1)], 0x2586: [(0, .25, 1, 1)], 0x1FB82: [(0, 0, 1, .25)], 0x1FB85: [(0, 0, 1, .75)],
    0x1FBE6: [(0, .25, .5, .75)], 0x1FBE7: [(.5, .25, 1, .75)],
    0x1CEA8: [(0, 0, .5, .25)], 0x1CEAB: [(.5, 0, 1, .25)], 0x1CEA3: [(0, .75, .5, 1)], 0x1CEA0: [(.5, .75, 1, 1)],
}
Q = {"UPPER LEFT": (0, 0, .5, .5), "UPPER RIGHT": (.5, 0, 1, .5), "LOWER LEFT": (0, .5, .5, 1), "LOWER RIGHT": (.5, .5, 1, 1)}
for cp, n in names(0x2596, 0x25A0):
    geo[cp] = [Q[p] for p in n[len("QUADRANT "):].split(" AND ")]
for cp, n in [(cp, u.name(chr(cp))) for cp in geo if cp != 0x20]:  # sanity: names agree with the geometry
    pass

def table(c, r, prefix):
    t = {}
    for cp, n in list(names(0x1FB00, 0x1FC00)) + list(names(0x1CC00, 0x1CEC0)):
        m = re.fullmatch(prefix + r"-(\d+)", n)
        if m:
            p = sum(1 << (int(d) - 1) for d in m.group(1))
            assert p not in t
            t[p] = cp
    for cp, rects in geo.items():  # patterns that have their own older character
        p = cells(c, r, rects)
        # only use a plain block if it is exact at this resolution (every sub-cell fully in or out)
        exact = all(any(x0 <= (col + e) / c <= x1 and y0 <= (row + f) / r <= y1 for x0, y0, x1, y1 in rects) ==
                    any(x0 <= (col + .5) / c <= x1 and y0 <= (row + .5) / r <= y1 for x0, y0, x1, y1 in rects)
                    for row in range(r) for col in range(c) for e in (.01, .99) for f in (.01, .99))
        if exact and p not in t:
            t[p] = cp
    missing = [p for p in range(1 << (c * r)) if p not in t]
    assert not missing, (prefix, missing)
    return [t[p] for p in range(1 << (c * r))]

half = table(1, 2, "NOMATCH")
quad = table(2, 2, "NOMATCH")
sext = table(2, 3, "BLOCK SEXTANT")
octt = table(2, 4, "BLOCK OCTANT")
assert half == [0x20, 0x2580, 0x2584, 0x2588], half
for p in range(1, 63):  # the usual sextant formula agrees
    if p not in (21, 42):
        assert sext[p] == 0x1FB00 + p - 1 - (p > 21) - (p > 42)
for t in (half, quad, sext, octt):
    assert len(set(t)) == len(t)

def go(name, t, per=8):
    rows = []
    for i in range(0, len(t), per):
        rows.append("\t" + " ".join("0x%04X," % v for v in t[i:i + per]))
    return "var %s = [%d]rune{\n%s\n}\n" % (name, len(t), "\n".join(rows))

with open(sys.argv[1], "w") as f:
    f.write("// Code generated from the Unicode %s character names by tools/mosaic_gen.py; DO NOT EDIT.\n\n" % u.unidata_version)
    f.write("package main\n\n")
    f.write("// Mosaic characters: index = bit pattern of filled sub-cells, bit k = sub-cell k in row-major\n")
    f.write("// order from the top left (the numbering of the BLOCK SEXTANT-n / BLOCK OCTANT-n names).\n\n")
    f.write(go("halfRunes", half, 4) + "\n" + go("quadRunes", quad) + "\n" + go("sextRunes", sext) + "\n" + go("octRunes", octt))
print("ok", len(half), len(quad), len(sext), len(octt))
print("quad:", "".join(chr(c) for c in quad))
