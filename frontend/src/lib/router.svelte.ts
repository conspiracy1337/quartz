// Hash router. Every screen is a typed Route; links are built with href() so a path
// is never spelled out by hand in a component.

export const INSTANCE_TABS = [
  'overview', 'mods', 'resourcepacks', 'shaders', 'datapacks', 'worlds',
  'servers', 'screenshots', 'logs', 'settings',
] as const;
export type InstanceTab = (typeof INSTANCE_TABS)[number];

export const SETTINGS_TABS = [
  'general', 'appearance', 'java', 'downloads', 'storage', 'syncing', 'integrations', 'updates', 'about',
] as const;
export type SettingsTab = (typeof SETTINGS_TABS)[number];

export type SetupStep = 1 | 2 | 3;

/** Browse's tabs (Go's project types). */
export const PROJECT_KINDS = ['mod', 'modpack', 'resourcepack', 'shader', 'datapack'] as const;
type Kind = (typeof PROJECT_KINDS)[number];
const SOURCES = ['modrinth', 'curseforge'] as const;

export type Route =
  | { name: 'home' }
  | { name: 'library' }
  | { name: 'new-instance' }
  | { name: 'import'; ref?: string; source?: (typeof SOURCES)[number]; version?: string; open?: string }
  | { name: 'browse'; type?: Kind; instance?: string }
  | { name: 'project'; id: string; tab?: 'versions' }
  | { name: 'skins' }
  | { name: 'accounts' }
  | { name: 'syncing' }
  | { name: 'downloads' }
  | { name: 'settings'; tab: SettingsTab }
  | { name: 'settings-search'; query: string }
  | { name: 'instance'; id: string; tab: InstanceTab; sub?: string }
  | { name: 'setup'; step: SetupStep };

const simple = ['home', 'library', 'browse', 'skins', 'accounts', 'syncing', 'downloads'] as const;

export function path(r: Route): string {
  switch (r.name) {
    case 'new-instance': return '/library/new';
    case 'import': return '/library/import' + (r.open ? `/open/${enc(r.open)}` : r.ref && r.source ? `/${enc(r.ref)}/${r.source}${r.version ? '/' + enc(r.version) : ''}` : '');
    case 'browse': return '/browse' + (r.type ? `/t/${r.type}${r.instance ? '/' + enc(r.instance) : ''}` : '');
    case 'project': return `/browse/${enc(r.id)}${r.tab ? '/' + r.tab : ''}`;
    case 'settings': return `/settings/${r.tab}`;
    case 'settings-search': return `/settings/search/${enc(r.query)}`;
    case 'instance': return `/instance/${enc(r.id)}/${r.tab}${r.sub ? '/' + enc(r.sub) : ''}`;
    case 'setup': return `/setup/${r.step}`;
    default: return `/${r.name}`;
  }
}

export const href = (r: Route) => '#' + path(r);

const enc = encodeURIComponent;
const dec = (s: string) => {
  try {
    return decodeURIComponent(s);
  } catch {
    return '';
  }
};

const isOneOf = <T extends string>(list: readonly T[], v: string | undefined): v is T =>
  v !== undefined && (list as readonly string[]).includes(v);

/** Parses a path; anything unknown falls back to Home. */
export function parse(p: string): Route {
  const [a, b, c, d, e] = p.replace(/^#?\/?/, '').split('/');
  if (isOneOf(simple, a) && b === undefined) return { name: a };
  switch (a) {
    case 'library':
      if (b === 'new') return { name: 'new-instance' };
      if (b === 'import') {
        if (c === 'open' && d) return { name: 'import', open: dec(d) };
        return c && isOneOf(SOURCES, d) ? { name: 'import', ref: dec(c), source: d, ...(e ? { version: dec(e) } : {}) } : { name: 'import' };
      }
      break;
    case 'browse':
      if (b === 't' && isOneOf(PROJECT_KINDS, c)) return { name: 'browse', type: c, ...(d ? { instance: dec(d) } : {}) };
      if (b) return { name: 'project', id: dec(b), ...(c === 'versions' ? { tab: 'versions' as const } : {}) };
      break;
    case 'settings':
      if (b === 'search') return { name: 'settings-search', query: dec(c ?? '') };
      if (isOneOf(SETTINGS_TABS, b)) return { name: 'settings', tab: b };
      return { name: 'settings', tab: 'general' };
    case 'instance':
      if (b && isOneOf(INSTANCE_TABS, c)) return { name: 'instance', id: dec(b), tab: c, ...(d ? { sub: dec(d) } : {}) };
      if (b) return { name: 'instance', id: dec(b), tab: 'overview' };
      break;
    case 'setup': {
      const step = Number(b);
      if (step === 1 || step === 2 || step === 3) return { name: 'setup', step };
      return { name: 'setup', step: 1 };
    }
  }
  return { name: 'home' };
}

/** A history entry's distance from the page the launcher opened on, or undefined for a fresh entry. */
const depthOf = (state: unknown): number | undefined => {
  const d = (state as { depth?: unknown } | null)?.depth;
  return typeof d === 'number' ? d : undefined;
};

class Router {
  route = $state<Route>(parse(typeof location === 'undefined' ? '' : location.hash));
  /** How many back steps lead to the page the launcher opened on. */
  depth = $state(0);

  get canBack() {
    return this.depth > 0;
  }

  start(): () => void {
    this.depth = depthOf(history.state) ?? 0;
    this.stamp();
    const sync = () => {
      // A back or forward step lands on a stamped entry; a new one is one deeper.
      const d = depthOf(history.state);
      this.depth = d ?? this.depth + 1;
      if (d === undefined) this.stamp();
      this.route = parse(location.hash);
    };
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }

  /** Navigates to r. `replace` updates the current history entry, for per-keystroke changes;
      `root` also makes it the page Back stops at. */
  go(r: Route, { replace = false, root = false } = {}) {
    if (!replace && !root) {
      location.hash = path(r);
      return;
    }
    if (root) this.depth = 0;
    history.replaceState({ ...history.state, depth: this.depth }, '', '#' + path(r));
    this.route = r;
  }

  /** Goes to the previous page; false on the page the launcher opened on. */
  back(): boolean {
    if (!this.canBack) return false;
    history.back();
    return true;
  }

  private stamp() {
    history.replaceState({ ...history.state, depth: this.depth }, '');
  }
}

export const router = new Router();
export const go = (r: Route, opts?: { replace?: boolean; root?: boolean }) => router.go(r, opts);
export const back = () => router.back();
