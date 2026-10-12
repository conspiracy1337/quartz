# Stacks design (top) over app (bottom) and reports a pixel-difference score.
import sys, os
from PIL import Image, ImageChops, ImageStat
d, a, o = sys.argv[1:4]
os.makedirs(o, exist_ok=True)
rows = []
for f in sorted(x for x in os.listdir(a) if x.endswith('.png')):
    if not os.path.exists(os.path.join(d, f)): continue
    A = Image.open(os.path.join(d, f)).convert('RGB'); B = Image.open(os.path.join(a, f)).convert('RGB')
    if A.size != B.size: B2 = B.resize(A.size)
    else: B2 = B
    diff = ImageChops.difference(A, B2)
    score = sum(ImageStat.Stat(diff).mean) / 3
    w = max(A.width, B.width)
    c = Image.new('RGB', (w, A.height + B.height + 6), (255, 0, 255))
    c.paste(A, (0, 0)); c.paste(B, (0, A.height + 6))
    c.save(os.path.join(o, f))
    diff.point(lambda p: 255 if p > 24 else 0).save(os.path.join(o, f.replace('.png', '.diff.png')))
    rows.append((score, f))
for s, f in sorted(rows, reverse=True): print(f'{s:6.2f} {f}')
