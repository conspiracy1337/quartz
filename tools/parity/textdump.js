// Injected into a page: dumps visible leaf text with rounded boxes, as "x,y,w|font|text" lines.
(() => {
  const out = [];
  const walk = (n) => {
    for (const c of n.childNodes) {
      if (c.nodeType === 3 && c.nodeValue.trim()) {
        const r = document.createRange(); r.selectNodeContents(c);
        const b = r.getBoundingClientRect();
        if (b.width < 1 || b.bottom < 0 || b.top > innerHeight || b.right < 0 || b.left > innerWidth) continue;
        const el = c.parentElement, cs = getComputedStyle(el);
        if (cs.visibility === 'hidden' || +cs.opacity === 0) continue;
        out.push(`${Math.round(b.left)},${Math.round(b.top)},${Math.round(b.width)}|${cs.fontSize} ${cs.fontWeight} ${cs.color}|${c.nodeValue.trim().replace(/\s+/g, ' ')}`);
      } else if (c.nodeType === 1) {
        const cs = getComputedStyle(c);
        if (cs.display === 'none') continue;
        walk(c);
      }
    }
  };
  walk(document.body);
  return out;
})()
