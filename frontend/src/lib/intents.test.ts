import { describe, expect, it, vi } from 'vitest';

const go = vi.fn();
const notify = vi.fn();
vi.mock('./router.svelte', () => ({ go }));
vi.mock('./session.svelte', () => ({ session: { notify } }));
const { show } = await import('./intents');

describe('open targets', () => {
  it('routes each kind', () => {
    show({ kind: 'import', openId: 'ab12' });
    expect(go).toHaveBeenLastCalledWith({ name: 'import', open: 'ab12' });
    show({ kind: 'project', project: 'mr:sodium' });
    expect(go).toHaveBeenLastCalledWith({ name: 'project', id: 'mr:sodium' });
    show({ kind: 'instance', instance: 'i1' });
    expect(go).toHaveBeenLastCalledWith({ name: 'instance', id: 'i1', tab: 'overview' });
  });
  it('turns errors into a toast', () => {
    show({ kind: 'error', subject: "Couldn't open that", text: 'nope' });
    expect(notify).toHaveBeenCalledWith({ tone: 'error', subject: "Couldn't open that", text: 'nope' });
  });
  it('ignores targets missing their value', () => {
    go.mockClear();
    show({ kind: 'import' });
    show({ kind: 'bogus' });
    expect(go).not.toHaveBeenCalled();
  });
});
