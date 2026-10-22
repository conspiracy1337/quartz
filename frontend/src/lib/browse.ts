import type { BrowseResults, BrowseSort, ProjectHit, ProjectKind } from './api/types';

/** Browse's search and filters, as a page left them. */
export interface BrowseState {
  type: ProjectKind; search: string; sort: BrowseSort; cats: Record<string, boolean>;
  useVersion: boolean; useLoader: boolean; side: '' | 'client' | 'server'; game: string;
}

/** What a newly opened Browse starts from: the search last left, on the type asked for
    (another type keeps the search and drops the categories, as switching tabs does). */
export function resumeBrowse(last: BrowseState | null, type?: ProjectKind): BrowseState | undefined {
  if (!last) return undefined;
  return !type || type === last.type ? last : { ...last, type, cats: {} };
}

/** Applies a further page of results: rows a twin merged into change in place
    (nothing moves), new rows go at the end. */
export function mergePage(rows: ProjectHit[], page: Pick<BrowseResults, 'rows' | 'changed'>): ProjectHit[] {
  const changed = new Map(page.changed.map((c) => [c.id, c]));
  return [...rows.map((r) => changed.get(r.id) ?? r), ...page.rows];
}
