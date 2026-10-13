<script lang="ts">
  import { captureKey } from '../keycapture';
  import { prefs } from '../prefs.svelte';
  import { session } from '../session.svelte';
  import { SHORTCUTS, chords, clash, type ShortcutId } from '../shortcuts';
  import Icon from '../ui/Icon.svelte';
  import Setting from '../ui/Setting.svelte';

  /** Settings, General: the editable keyboard shortcuts (composed: setting rows with the
      Syncing page's key button, which turns into "Press a key…"). */
  const editable = SHORTCUTS.filter((s) => s.editable);
  const current = $derived(chords(prefs.value?.general.keys));
  let capturing = $state<ShortcutId | null>(null);
  let cancel: (() => void) | undefined;
  $effect(() => () => cancel?.());

  async function capture(id: ShortcutId) {
    cancel?.();
    const c = captureKey(true);
    cancel = c.cancel;
    capturing = id;
    const chord = await c.done;
    if (capturing !== id) return;
    capturing = null;
    if (!chord || chord === current[id]) return;
    const why = clash(id, chord, current);
    if (why) return session.notify({ tone: 'error', subject: "Couldn't use that shortcut", text: why });
    await set(id, chord);
  }

  function set(id: ShortcutId, chord: string | null) {
    return prefs.update((s) => {
      const keys = { ...(s.general.keys ?? {}) };
      const def = SHORTCUTS.find((x) => x.id === id)!.chord;
      if (chord && chord !== def) keys[id] = chord;
      else delete keys[id];
      s.general.keys = keys;
    });
  }
</script>

<section class="grp" aria-label="Keyboard shortcuts">
  <h2 class="h3">Keyboard shortcuts</h2>
  <div class="card">
    {#each editable as s, i (s.id)}
      {#if i > 0}<div class="divider"></div>{/if}
      {@const custom = current[s.id] !== s.chord}
      <Setting title={s.label} desc={custom ? `Default ${s.chord}` : undefined}>
        <span style="display: flex; gap: 6px; justify-content: flex-end; align-items: center">
          {#if custom}<button class="btn btn-icon btn-sm btn-ghost" aria-label="Reset {s.label} to {s.chord}" onclick={() => set(s.id, null)}><Icon name="x" size={10} sw={2.2} /></button>{/if}
          <button class="btn btn-sm" style="justify-content: space-between; font-family: var(--mono)" aria-label="Shortcut for {s.label}" aria-pressed={capturing === s.id} onclick={() => capture(s.id)}><span class="trunc">{capturing === s.id ? 'Press a key…' : current[s.id]}</span></button>
        </span>
      </Setting>
    {/each}
  </div>
</section>
