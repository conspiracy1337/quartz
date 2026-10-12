<script lang="ts">
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';

  const SHORTCUTS = [
    { name: 'Anywhere', items: [['Command palette', ['Ctrl', 'K']], ['Switch instance', ['Ctrl', 'P']], ['Settings', ['Ctrl', ',']], ['Show shortcuts', ['?']]] },
    { name: 'Go to', items: [['Home', ['G', 'H']], ['Library', ['G', 'L']], ['Browse', ['G', 'B']], ['Downloads', ['G', 'D']]] },
    { name: 'Instance', items: [['Play', ['Ctrl', 'Enter']], ['Stop game', ['Ctrl', '.']], ['Open folder', ['Ctrl', 'O']], ['Next tab', ['Ctrl', 'Tab']]] },
    { name: 'Lists', items: [['Move', ['J', 'K']], ['Toggle selected', ['Space']], ['Search', ['/']], ['Select all', ['Ctrl', 'A']]] },
  ] as const;
</script>

<Dialog title="Keyboard shortcuts" width={680} onclose={() => session.close()}>
  <div class="cols">
    {#each SHORTCUTS as g (g.name)}
      <section aria-label={g.name}>
        <h3 class="h3" style="margin-bottom: 4px">{g.name}</h3>
        {#each g.items as [label, keys] (label)}<div class="sc" style="border-top: 1px solid var(--line)"><span class="muted trunc">{label}</span><span class="k">{#each keys as k (k)}<span class="kbd">{k}</span>{/each}</span></div>{/each}
      </section>
    {/each}
  </div>
  {#snippet foot()}<span class="small faint" style="margin-right: auto">Change these in Settings, General.</span><button class="btn" onclick={() => session.close()}>Close</button>{/snippet}
</Dialog>

<style>
  .cols{display:grid;grid-template-columns:1fr 1fr;gap:20px 28px}
  @media (max-width:760px){.cols{grid-template-columns:1fr}}
  .sc{display:flex;justify-content:space-between;align-items:center;height:32px;gap:12px}
  .sc .k{display:flex;gap:4px;flex:0 0 auto}
</style>
