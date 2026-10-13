import type { BrowseResults, ProjectHit } from './api/types';

/** Applies a further page of results: rows a twin merged into change in place
    (nothing moves), new rows go at the end. */
export function mergePage(rows: ProjectHit[], page: Pick<BrowseResults, 'rows' | 'changed'>): ProjectHit[] {
  const changed = new Map(page.changed.map((c) => [c.id, c]));
  return [...rows.map((r) => changed.get(r.id) ?? r), ...page.rows];
}
