<script lang="ts">
  import type { Alert } from '../api/types';
  import { go } from '../router.svelte';
  import { session } from '../session.svelte';
  import Icon from '../ui/Icon.svelte';

  /** One bar that always spans its cell. Each alert gets an equal share. */
  interface Props { alerts: Alert[] }
  let { alerts }: Props = $props();

  function act(a: Alert) {
    const t = a.target;
    if (t.kind === 'sign-in') session.open({ kind: 'add-account', mode: 'browser', accountId: t.accountId });
    else if (t.kind === 'instance') go({ name: 'instance', id: t.id, tab: t.tab });
    else if (t.kind === 'settings') go({ name: 'settings', tab: t.tab });
    else go({ name: 'instance', id: t.id, tab: 'overview', sub: 'update' });
  }
</script>

<div class="ha" role="region" aria-label="Needs attention" style="font-family: var(--font); color: var(--text)">
  <div class="ha-in">
    {#each alerts as a, i (i)}
      <div class="ha-al">
        <Icon name={a.tone === 'warn' ? 'warn' : 'download'} size={14} sw={2} color={a.tone === 'warn' ? 'var(--warn)' : 'var(--info)'} />
        <span class="small trunc"><span style="font-weight: 500">{a.subject}</span> {a.text}</span>
        <button class="btn btn-sm" onclick={() => act(a)}>{a.action}</button>
      </div>
    {/each}
  </div>
</div>

<style>
  .ha{container-type:inline-size;width:100%}
  .ha-in{display:grid;grid-auto-flow:column;grid-auto-columns:minmax(0,1fr);height:var(--alert-h);border:1px solid color-mix(in srgb,var(--warn) 30%,var(--line));border-radius:var(--radius-lg);background:color-mix(in srgb,var(--warn) 5%,var(--panel));overflow:hidden}
  /* 40px bar, 1px border, 28px button: 5px around the button on every side, radius 10 - 1 - 5 = 4 */
  .ha-al{display:grid;grid-template-columns:16px minmax(0,1fr) auto;align-items:center;column-gap:10px;min-width:0;padding:0 5px 0 var(--inset)}
  .ha-al + .ha-al{border-left:1px solid color-mix(in srgb,var(--warn) 20%,var(--line))}
  .ha-al .btn{border-radius:4px}
  @container (max-width:560px){
    .ha-in{grid-auto-flow:row;height:auto}
    .ha-al{height:38px}
    .ha-al + .ha-al{border-left:0;border-top:1px solid color-mix(in srgb,var(--warn) 20%,var(--line))}
  }
</style>
