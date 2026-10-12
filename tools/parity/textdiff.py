# For each board: texts only in the design, only in the app, and common texts that moved or restyled.
import sys, os
from collections import Counter, defaultdict
d, a = sys.argv[1:3]
only = sys.argv[3:]
IGN = {'Launcher', 'Quartz Client'}
def load(p):
    rows = []
    for ln in open(p).read().splitlines():
        try: pos, font, txt = ln.split('|', 2)
        except ValueError: continue
        x, y, w = map(int, pos.split(','))
        rows.append((txt, x, y, w, font))
    return rows
for f in sorted(os.listdir(a)):
    if not f.endswith('.txt') or not os.path.exists(os.path.join(d, f)): continue
    b = f[:-4]
    if only and b not in only: continue
    D, A = load(os.path.join(d, f)), load(os.path.join(a, f))
    cd, ca = Counter(t for t, *_ in D if t not in IGN), Counter(t for t, *_ in A if t not in IGN)
    od, oa = cd - ca, ca - cd
    pd, pa = defaultdict(list), defaultdict(list)
    for t, x, y, w, fn in D: pd[t].append((x, y, w, fn))
    for t, x, y, w, fn in A: pa[t].append((x, y, w, fn))
    moved = []
    for t in cd & ca:
        for (x1, y1, w1, f1), (x2, y2, w2, f2) in zip(pd[t], pa[t]):
            if abs(x1 - x2) > 2 or abs(y1 - y2) > 2 or abs(w1 - w2) > 2 or f1 != f2:
                moved.append(f'{t[:40]!r} d({x1},{y1},{w1},{f1}) a({x2},{y2},{w2},{f2})')
    if not (od or oa or moved): continue
    print(f'=== {b}: design-only {sum(od.values())}, app-only {sum(oa.values())}, moved {len(moved)}')
    if od: print('  D:', ' ¦ '.join(list(od.elements())[:25]))
    if oa: print('  A:', ' ¦ '.join(list(oa.elements())[:25]))
    for m in moved[:8]: print('  M:', m)
