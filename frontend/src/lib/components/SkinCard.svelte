<script lang="ts">
  import { api } from '../api';
  import { hours } from '../format';
  import { query } from '../query.svelte';
  import { STEVE, accountLook, capeLook } from '../look';
  import PlayerModel from './PlayerModel.svelte';

  /** The current skin in 3D next to this week's play activity. */
  interface Props { scale?: number }
  let { scale = 6 }: Props = $props();

  const DAYS = [['M', 'Monday'], ['T', 'Tuesday'], ['W', 'Wednesday'], ['T', 'Thursday'], ['F', 'Friday'], ['S', 'Saturday'], ['S', 'Sunday']] as const;

  const week = query(() => api.home.week());
  const skins = query(() => api.accounts.skins());
  const accounts = query(() => api.accounts.list());
  const me = $derived(accounts.data?.find((a) => a.isDefault));
  const capes = query(() => (me ? api.accounts.capes(me.id) : Promise.resolve({ capes: [], worn: '' })));
  // With no account yet, the default skin stands in (as on the Skins page).
  const look = $derived(me && skins.data ? accountLook(me, skins.data) : accounts.data && !me ? STEVE : undefined);
  const cape = $derived(capeLook(capes.data?.capes.find((c) => c.id === capes.data?.worn)));

  const total = $derived((week.data ?? []).reduce((n, d) => n + d.minutes, 0));
  const max = $derived(Math.max(1, ...(week.data ?? []).map((d) => d.minutes)));
  const days = $derived((week.data ?? []).map((d, i) => ({
    letter: DAYS[i]![0], label: `${DAYS[i]![1]}, ${hours(d.minutes)}`, h: Math.round((d.minutes / max) * 100) + '%', today: d.today ?? i === 6,
  })));
  const summary = $derived(`Played ${hours(total)} this week. ` + days.map((d) => d.label.replace(',', '')).join(', '));
</script>

<section class="card skc" aria-label="Your skin and play activity" style="font-family: var(--font); color: var(--text)">
  <div class="skc-in">
    <div class="skc-mdl">
      {#if look}<PlayerModel {...look} {...cape} controls={false} fill {scale} />{/if}
    </div>
    <div class="skc-pt">
      <div class="skc-head"><span class="small faint">Played this week</span><span class="mono small tnum">{week.data ? hours(total) : ''}</span></div>
      <div class="skc-chart">
        <div class="skc-bars" role="img" aria-label={summary}>
          {#each days as d, i (i)}<span class:today={d.today} style="height: {d.h}" title={d.label}></span>{/each}
        </div>
        <div class="skc-days" aria-hidden="true">
          {#each days as d, i (i)}<span class:today={d.today}>{d.letter}</span>{/each}
        </div>
      </div>
    </div>
  </div>
</section>

<style>
  /* Model and this week's play activity. Landscape: side by side. Portrait: stacked. The chart takes all the height it gets. */
  .skc{position:absolute;inset:0;container-type:size;overflow:hidden}
  .skc-in{position:absolute;inset:0;display:grid;grid-template-rows:minmax(0,1.4fr) minmax(140px,1fr)}
  .skc-mdl{position:relative;min-height:0;min-width:0;background:radial-gradient(60% 55% at 50% 55%,#19191d 0%,transparent 75%)}
  .skc-pt{display:flex;flex-direction:column;gap:12px;padding:var(--inset);border-top:1px solid var(--line);min-width:0;min-height:0}
  .skc-head{display:flex;justify-content:space-between;align-items:baseline;gap:8px;height:16px}
  .skc-chart{flex:1 1 auto;min-height:0;display:grid;grid-template-rows:minmax(0,1fr) 16px;row-gap:6px}
  .skc-bars,.skc-days{display:grid;grid-template-columns:repeat(7,minmax(0,1fr));column-gap:6px}
  .skc-bars{align-items:end;min-height:0}
  .skc-bars span{border-radius:3px;background:var(--line-2);min-height:2px}
  .skc-bars span.today{background:var(--accent)}
  .skc-days span{font-size:11px;line-height:16px;color:var(--faint);text-align:center}
  .skc-days span.today{color:var(--text)}
  @container (orientation:landscape){
    .skc-in{grid-template-rows:minmax(0,1fr);grid-template-columns:minmax(0,1fr) minmax(0,1.3fr)}
    .skc-pt{border-top:0;border-left:1px solid var(--line)}
  }
</style>
