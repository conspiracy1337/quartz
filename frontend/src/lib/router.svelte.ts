// Hash router. Every screen is a typed Route; links are built with href() so a path
// is never spelled out by hand in a component.

export const INSTANCE_TABS = [
  'overview', 'mods', 'resourcepacks', 'shaders', 'datapacks', 'worlds',
  'servers', 'screenshots', 'logs', 'version', 'settings',
] as const;
export type InstanceTab = (typeof INSTANCE_TABS)[number];

export const SETTINGS_TABS = [
  'general', 'appearance', 'java', 'downloads', 'storage', 'syncing', 'integrations', 'updates', 'about',
] as const;
export type SettingsTab = (typeof SETTINGS_TABS)[number];

export type SetupStep = 1 | 2 | 4;

/** Browse's tabs (Go's project types). */
export const PROJECT_KINDS = ['mod', 'modpack', 'resourcepack', 'shader', 'datapack'] as const;
type Kind = (typeof PROJECT_KINDS)[number];
const SOURCES = ['modrinth', 'curseforge'] as const;

export type Route =
  | { name: 'home' }
  | { name: 'library' }
  | { name: 'new-instance' }
  | { name: 'import'; ref?: string; source?: (typeof SOURCES)[number]; open?: string }
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
    case 'import': return '/library/import' + (r.open ? `/open/${enc(r.open)}` : r.ref && r.source ? `/${enc(r.ref)}/${r.source}` : '');
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
  const [a, b, c, d] = p.replace(/^#?\/?/, '').split('/');
  if (isOneOf(simple, a) && b === undefined) return { name: a };
  switch (a) {
    case 'library':
      if (b === 'new') return { name: 'new-instance' };
      if (b === 'import') {
        if (c === 'open' && d) return { name: 'import', open: dec(d) };
        return c && isOneOf(SOURCES, d) ? { name: 'import', ref: dec(c), source: d } : { name: 'import' };
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
      if (step === 1 || step === 2 || step === 4) return { name: 'setup', step };
      return { name: 'setup', step: 1 };
    }
  }
  return { name: 'home' };
}

class Router {
  route = $state<Route>(parse(typeof location === 'undefined' ? '' : location.hash));

  start(): () => void {
    const sync = () => {
      this.route = parse(location.hash);
    };
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }

  /** Navigates to r. `replace` updates the current history entry, for per-keystroke changes. */
  go(r: Route, { replace = false } = {}) {
    if (!replace) {
      location.hash = path(r);
      return;
    }
    history.replaceState(history.state, '', '#' + path(r));
    this.route = r;
  }
}

export const router = new Router();
export const go = (r: Route, opts?: { replace?: boolean }) => router.go(r, opts);
