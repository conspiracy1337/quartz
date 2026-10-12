// Minimal stand-in for the design tool's runtime: renders <x-dc> templates
// (helmet, {{expr}}, sc-if, sc-for, dc-import, onX handlers, DCLogic).
(() => {
  class DCLogic {
    constructor(props) { this.props = props || {}; this.state = {}; }
    setState(p) {
      const next = typeof p === 'function' ? p(this.state, this.props) : p;
      this.state = { ...this.state, ...next };
      this.__dc && this.__dc.rerender();
    }
    forceUpdate() { this.__dc && this.__dc.rerender(); }
    renderVals() { return {}; }
  }
  window.DCLogic = DCLogic;

  const cache = new Map();
  const styled = new Set();
  async function load(name) {
    if (cache.has(name)) return cache.get(name);
    const p = (async () => {
      const txt = await (await fetch(`./${name}.dc.html`)).text();
      return parseDoc(new DOMParser().parseFromString(txt, 'text/html'), name);
    })();
    cache.set(name, p);
    return p;
  }

  function parseDoc(doc, name) {
    const xdc = doc.querySelector('x-dc');
    const helmet = xdc.querySelector(':scope > helmet');
    if (helmet) helmet.remove();
    const script = doc.querySelector('script[type="text/x-dc"]');
    let Cls = class extends DCLogic {};
    let defaults = {};
    if (script) {
      try {
        const meta = JSON.parse(script.getAttribute('data-props') || '{}');
        for (const [k, v] of Object.entries(meta)) if (k[0] !== '$' && v && 'default' in v) defaults[k] = v.default;
      } catch (e) { console.error(name, 'props', e); }
      // eslint-disable-next-line no-new-func
      Cls = new Function('DCLogic', script.textContent + '\n;return Component;')(DCLogic);
    }
    return { name, xdc, helmet, Cls, defaults };
  }

  function addHelmet(def) {
    if (!def.helmet || styled.has(def.name)) return;
    styled.add(def.name);
    for (const n of [...def.helmet.childNodes]) {
      if (n.nodeType !== 1) continue;
      const c = document.createElement(n.tagName.toLowerCase());
      for (const a of n.attributes) c.setAttribute(a.name, a.value);
      c.textContent = n.textContent;
      document.head.appendChild(c);
    }
  }

  const ONE = /^\s*\{\{\s*([^}]+?)\s*\}\}\s*$/;
  function lookup(expr, scope) {
    expr = expr.trim();
    if (/^-?\d+(\.\d+)?$/.test(expr)) return Number(expr);
    if (expr === 'true') return true;
    if (expr === 'false') return false;
    if (expr === 'null') return null;
    if (/^'.*'$/.test(expr)) return expr.slice(1, -1);
    const parts = expr.split('.');
    let v = scope;
    for (const p of parts) { if (v == null) return undefined; v = v[p]; }
    return v;
  }
  const raw = (s, scope) => { const m = ONE.exec(s); return m ? lookup(m[1], scope) : interp(s, scope); };
  const interp = (s, scope) => s.replace(/\{\{\s*([^}]+?)\s*\}\}/g, (_, e) => { const v = lookup(e, scope); return v == null ? '' : String(v); });
  const BOOL = new Set(['disabled', 'checked', 'hidden', 'selected', 'readonly', 'open', 'required', 'autofocus']);

  class Instance {
    constructor(def, props, parentEl, before) {
      this.def = def;
      this.parentEl = parentEl;
      this.marker = document.createComment(def.name);
      parentEl.insertBefore(this.marker, before);
      this.nodes = [];
      this.children = new Map();
      this.logic = new def.Cls({ ...def.defaults, ...props });
      if (!this.logic.state) this.logic.state = {};
      this.logic.__dc = this;
      addHelmet(def);
    }
    async render() {
      const vals = { ...this.logic.props, ...this.logic.state, ...(this.logic.renderVals() || {}) };
      const frag = document.createDocumentFragment();
      this.seen = new Map();
      await renderChildren(this.def.xdc, frag, vals, this, '0');
      for (const n of this.nodes) n.remove();
      this.nodes = [...frag.childNodes];
      this.parentEl = this.marker.parentNode;
      this.parentEl.insertBefore(frag, this.marker.nextSibling);
      for (const [k, c] of this.children) if (!this.seen.has(k)) { c.unmount(); this.children.delete(k); }
      if (!this.mounted) { this.mounted = true; this.logic.componentDidMount && this.logic.componentDidMount(); }
    }
    rerender() {
      if (this.pending) return;
      this.pending = true;
      queueMicrotask(async () => { this.pending = false; await this.render(); });
    }
    unmount() {
      this.logic.componentWillUnmount && this.logic.componentWillUnmount();
      for (const c of this.children.values()) c.unmount();
      for (const n of this.nodes) n.remove();
      this.marker.remove();
    }
  }

  async function renderChildren(src, out, scope, inst, key) {
    let i = 0;
    for (const n of [...src.childNodes]) await renderNode(n, out, scope, inst, key + '.' + i++);
  }

  async function renderNode(n, out, scope, inst, key) {
    if (n.nodeType === 3) { out.appendChild(document.createTextNode(interp(n.nodeValue, scope))); return; }
    if (n.nodeType !== 1) return;
    const tag = n.tagName.toLowerCase();
    if (tag === 'sc-if') {
      if (raw(n.getAttribute('value') || '', scope)) await renderChildren(n, out, scope, inst, key);
      return;
    }
    if (tag === 'sc-for') {
      const list = raw(n.getAttribute('list') || '', scope) || [];
      const as = n.getAttribute('as') || 'item';
      let j = 0;
      for (const item of list) await renderChildren(n, out, { ...scope, [as]: item }, inst, key + '#' + j++);
      return;
    }
    if (tag === 'dc-import') {
      const props = {};
      for (const a of n.attributes) if (a.name !== 'name' && !a.name.startsWith('hint-')) props[camel(a.name)] = raw(a.value, scope);
      const def = await load(n.getAttribute('name'));
      const holder = document.createElement('div');
      out.appendChild(holder);
      let child = inst.children.get(key);
      if (!child) { child = new Instance(def, props, holder, null); inst.children.set(key, child); }
      else { child.logic.props = { ...def.defaults, ...props }; holder.appendChild(child.marker); }
      inst.seen.set(key, true);
      await child.render();
      // unwrap
      const kids = [child.marker, ...child.nodes];
      for (const k of kids) out.insertBefore(k, holder);
      holder.remove();
      return;
    }
    const svg = n.namespaceURI === 'http://www.w3.org/2000/svg';
    const el = svg ? document.createElementNS(n.namespaceURI, n.tagName) : document.createElement(tag);
    for (const a of n.attributes) {
      if (/^on[A-Z]/.test(a.name) || /^on[a-z]+$/.test(a.name) && ONE.test(a.value)) {
        const fn = raw(a.value, scope);
        if (typeof fn === 'function') el.addEventListener(a.name.slice(2).toLowerCase(), fn);
        continue;
      }
      const v = raw(a.value, scope);
      if (BOOL.has(a.name) && ONE.test(a.value)) { if (v) el.setAttribute(a.name, ''); continue; }
      el.setAttribute(a.name, v == null ? '' : String(v));
    }
    if (tag === 'template') { el.innerHTML = n.innerHTML; out.appendChild(el); return; }
    await renderChildren(tag === 'template' ? n.content : n, el, scope, inst, key);
    out.appendChild(el);
  }
  const camel = (s) => s.replace(/-([a-z])/g, (_, c) => c.toUpperCase());

  window.addEventListener('DOMContentLoaded', async () => {
    const name = location.pathname.split('/').pop().replace(/\.dc\.html$/, '');
    const def = parseDoc(document, name);
    const host = document.body;
    def.xdc.remove();
    const props = Object.fromEntries(new URLSearchParams(location.search));
    for (const k in props) { try { props[k] = JSON.parse(props[k]); } catch { /* string */ } }
    const root = new Instance(def, props, host, host.firstChild);
    await root.render();
    window.__dcReady = true;
  });
})();
