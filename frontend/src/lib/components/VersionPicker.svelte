<script lang="ts">
  import { untrack } from 'svelte';
  import { api } from '../api';
  import { shortMonthDay } from '../format';
  import { query } from '../query.svelte';
  import Toggle from '../ui/Toggle.svelte';

  /** Version families on the left, that family's releases on the right. Both lists share one
      fixed height and scroll, so their edges line up and nothing moves when you pick. */
  interface Props { current?: string; onpick?: (id: string) => void }
  let { current = '', onpick }: Props = $props();

  const families = query(() => api.versions.minecraft());
  let m = $state(-1);
  let v = $state(0);
  let snapshots = $state(false);

  // Once the list arrives, start on the family that holds `current` (or the newest one)
  // and report that selection, so the form never shows one version and submits another.
  $effect.pre(() => {
    const d = families.data;
    if (!d?.length || m >= 0) return;
    const fi = Math.max(0, d.findIndex((f) => f.releases.some((r) => r.id === current)));
    untrack(() => choose(fi, Math.max(0, d[fi]!.releases.findIndex((r) => r.id === current)), false));
  });

  const fam = $derived(families.data?.[m]);
  const list = $derived(fam ? [...fam.releases, ...(snapshots ? fam.snapshots : [])] : []);
  const vi = $derived(Math.min(v, list.length - 1));

  function choose(nm: number, nv: number, snap: boolean) {
    m = nm;
    v = nv;
    snapshots = snap;
    const f = families.data?.[nm];
    if (!f) return;
    const l = [...f.releases, ...(snap ? f.snapshots : [])];
    const sel = l[Math.min(nv, l.length - 1)];
    if (sel) onpick?.(sel.id);
  }
  const date = (t: number) => `${shortMonthDay(t)}, ${new Date(t).getFullYear()}`;
</script>

<div class="vp picker">
  <section aria-label="Version family">
    <div class="plabel"><span>Version</span><span class="tnum">{families.data?.length ?? ''}</span></div>
    <div class="list plist vp-box" role="radiogroup" aria-label="Version family">
      {#each families.data ?? [] as f, i (f.id)}
        <button class="prow" class:on={m === i} onclick={() => choose(i, 0, snapshots)} role="radio" aria-checked={m === i}>
          <span style="min-width: 0"><span class="mono" style="display: block; font-weight: 500">{f.id}</span><span class="trunc small faint" style="display: block">{f.note}, latest {f.releases[0]?.id}</span></span>
          {#if current !== '' && f.releases.some((r) => r.id === current)}<span class="badge">Current</span>{/if}
        </button>
      {/each}
    </div>
  </section>
  <section aria-label="Exact release">
    <div class="plabel"><span>{fam?.id ?? ''} releases</span><span style="display: flex; gap: 12px; align-items: center"><label class="vp-snap"><Toggle on={snapshots} label="Show snapshots" onchange={(s) => choose(m, 0, s)} />Snapshots</label><span class="tnum">{list.length}</span></span></div>
    <div class="list plist vp-box" role="radiogroup" aria-label="Exact release">
      {#each list as r, i (r.id)}
        <button class="prow min" class:on={vi === i} onclick={() => choose(m, i, snapshots)} role="radio" aria-checked={vi === i}>
          <span class="radio" class:on={vi === i} aria-hidden="true"></span>
          <span style="display: flex; gap: 8px; align-items: center; min-width: 0"><span class="mono">{r.id}</span>{#if current !== '' && r.id === current}<span class="badge">Current</span>{/if}{#if r.snapshot}<span class="badge badge-warn">Snapshot</span>{/if}</span>
          <span class="small faint">Java {r.java}</span>
          <span class="small faint tnum vp-date">{date(r.released)}</span>
        </button>
      {/each}
    </div>
  </section>
</div>

<style>
  .vp{width:100%;font-family:var(--font);color:var(--text)}
  .vp-box{height:352px}
  .vp-snap{display:flex;gap:8px;align-items:center;color:var(--muted)}
  .vp .prow.min{grid-template-columns:16px minmax(0,1fr) 64px 104px}
  @media (max-width:1100px){.vp .prow.min{grid-template-columns:16px minmax(0,1fr) 64px}.vp-date{display:none}}
</style>
