// Picking rows in a list for bulk actions, as file managers do: a click ticks or
// unticks one row, Shift-click applies the same to every row from the last one
// clicked (the anchor) to this one.

/** Updates picked in place and returns the new anchor. */
export function pickRows(picked: Record<string, boolean>, order: string[], id: string, on: boolean, anchor: string | null, shift: boolean): string {
  const at = order.indexOf(id);
  const from = anchor ? order.indexOf(anchor) : -1;
  if (shift && from >= 0 && at >= 0) {
    for (let k = Math.min(from, at); k <= Math.max(from, at); k++) picked[order[k]!] = on;
  } else {
    picked[id] = on;
  }
  return id;
}
