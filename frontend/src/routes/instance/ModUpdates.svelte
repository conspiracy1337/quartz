<script lang="ts">
  import { listKeys } from '$lib/listkeys';
  import { api } from '$lib/api';
  import type { ContentKind, Instance, ModUpdate, UpdateReview } from '$lib/api/types';
  import { hasBridge } from '$lib/bridge';
  import { bytes } from '$lib/format';
  import { prefs } from '$lib/prefs.svelte';
  import { go, href } from '$lib/router.svelte';
  import { CHANNELS } from '$lib/settings/registry';
  import { session } from '$lib/session.svelte';
  import { tasks } from '$lib/tasks.svelte';
  import ChangelogDialog from '$lib/components/ChangelogDialog.svelte';
  import Check from '$lib/ui/Check.svelte';
  import Empty from '$lib/ui/Empty.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Seg from '$lib/ui/Seg.svelte';
  import Tile from '$lib/ui/Tile.svelte';

  interface Props { instance: Instance; kind?: ContentKind }
  let { instance: i, kind = 'mods' }: Props = $props();

  let review = $state<UpdateReview | null>(null);
  let picked = $state<Record<string, boolean>>({});
  let channel = $state<string>(prefs.value?.updates.modChannel ?? 'release');
  let open = $state(-1);
  let busy = $state(false);
  let loading = $state(true);
  const list = $derived(review?.updates ?? []);
  // Blocked files can be picked too: the job waits for their manual download.
  const chosen = $derived(list.filter((u) => picked[u.id]));
  const count = $derived(chosen.length);
  const size = $derived(chosen.reduce((n, u) => n + u.size, 0));
  const cur = $derived<ModUpdate | undefined>(list[open]);
  const back = $derived(href({ name: 'instance', id: i.id, tab: kind }));
  const locked = $derived(review?.locked ?? []);
  const failed = $derived(review?.failed ?? []);
  const lockedText = $derived(locked.length === 1 ? `${locked[0]!.name} is locked to ${locked[0]!.version} and was skipped.`
    : `${locked[0]?.name} and ${locked.length - 1} more are locked and were skipped.`);

  async function load(fresh = false) {
    loading = true;
    try {
      review = await api.content.review(i.id, kind, fresh);
      // Everything that can be downloaded starts picked; the preview keeps the design's sample choice.
      // Updates picked on the content tab come ticked; otherwise every update that needs no manual download.
      const want = session.updatePick;
      session.updatePick = null;
      picked = Object.fromEntries(review.updates.map((u, k) => [u.id, want ? want.has(u.id) : !u.blocked && (hasBridge() || k < 4)]));
      open = -1;
    } catch (e) {
      session.fail('Check updates', e);
    } finally {
      loading = false;
    }
  }
  $effect(() => {
    void load();
  });

  async function pickChannel(v: string) {
    await prefs.update((s) => (s.updates.modChannel = v as 'release' | 'beta' | 'alpha'));
    await load(true);
  }

  // A changelog loads when its dialog opens.
  async function show(k: number) {
    open = k;
    const u = list[k];
    if (!u || u.versions.length || !review) return;
    try {
      u.versions = await api.content.changelog(review.checkId, u.id);
    } catch (e) {
      session.fail(u.name, e);
    }
  }

  async function unlock() {
    try {
      for (const l of locked) await api.content.setLocked(i.id, l.id, false);
      await load(true);
    } catch (e) {
      session.fail('Unlock', e);
    }
  }

  async function apply() {
    if (!review || busy) return;
    busy = true;
    try {
      const job = await api.content.apply(review.checkId, chosen.map((u) => u.id));
      api.content.forget(i.id, kind);
      if (job) await tasks.load();
      go({ name: 'instance', id: i.id, tab: kind });
    } catch (e) {
      session.fail('Update', e);
    } finally {
      busy = false;
    }
  }

  /** The project page's Versions tab; the platform's site for an item without a reference. */
  function allVersions(u: ModUpdate) {
    if (u.ref) go({ name: 'project', id: u.ref, tab: 'versions' });
    else if (u.versionsUrl) void api.app.openUrl(u.versionsUrl);
  }
</script>

<div class="topbar">
  <a href={back} class="btn btn-sm btn-ghost btn-icon" aria-label="Back"><Icon name="chevron-left" size={14} sw={2} /></a>
  <h1 class="h2">Updates for {i.name}</h1>
  <div style="flex: 1 1 0"></div>
  <span class="small faint">Channel</span><Seg options={CHANNELS} bind:value={channel} label="Update channel" onchange={pickChannel} />
</div>
<div class="page page-fill" style="gap: 12px; padding-top: 16px">
  <div class="list list-fill" use:listKeys>
    {#if loading}
      {#each { length: 6 } as _, k (k)}<div class="row ur" aria-hidden="true"><div class="sk" style="width: 16px; height: 16px; border-radius: 4px"></div><div class="sk" style="width: 32px; height: 32px; border-radius: 7px"></div><div style="display: flex; flex-direction: column; gap: 6px"><div class="sk" style="height: 12px; width: {[40, 30, 46, 34, 52, 38][k]}%"></div><div class="sk" style="height: 10px; width: 24%"></div></div><div class="sk opt" style="height: 10px; width: 70%"></div><div class="sk" style="height: 28px; width: 84px; justify-self: end"></div></div>{/each}
    {:else}
    {#each list as u, k (u.id)}
      <div class="row ur">
        <Check on={!!picked[u.id]} label="Update {u.name}" onchange={(v) => (picked[u.id] = v)} />
        <Tile art={u.art} />
        <span style="min-width: 0"><span class="trunc" style="display: block; font-weight: 500">{u.name}</span><span class="trunc small faint" style="display: block">{u.note}</span></span>
        <span class="mono muted opt" style="gap: 8px; align-items: center"><span>{u.from}</span><Icon name="arrow-right" size={12} sw={2} /><span style="color: var(--text)">{u.to}</span></span>
        <span style="display: flex; justify-content: flex-end"><button class="btn btn-sm" onclick={() => show(k)}>Changelog</button></span>
      </div>
    {:else}
      <Empty text={failed.length ? 'No updates found.' : 'Everything is up to date.'} action="Back" href={back} />
    {/each}
    {/if}
  </div>
  {#each failed as f (f)}
    <!-- Composed from the locked card: a platform that couldn't be asked. -->
    <div class="card" style="padding: 12px 16px"><span class="small muted">{f}</span></div>
  {/each}
  {#if locked.length}
    <div class="card" style="display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 14px; align-items: center; padding: 12px 16px">
      <span class="small muted">{lockedText}</span>
      <button class="btn btn-sm btn-ghost" onclick={unlock}>Unlock</button>
    </div>
  {/if}
  <div style="display: flex; justify-content: flex-end; gap: 8px; align-items: center">
    <span class="small faint tnum" style="margin-right: auto">{count} selected, about {bytes(size)}</span>
    <a href={back} class="btn">Cancel</a>
    <button class="btn btn-accent" disabled={count === 0 || busy} onclick={apply}>Update {count}</button>
  </div>
</div>
{#if cur}
  <ChangelogDialog title={cur.name} meta="{cur.from} to {cur.to}" versions={cur.versions} onclose={() => (open = -1)}>
    {#snippet foot()}<button class="btn" style="margin-right: auto" disabled={!cur.ref && !cur.versionsUrl} onclick={() => allVersions(cur)}>All versions</button><button class="btn" onclick={() => (open = -1)}>Close</button><button class="btn btn-accent" onclick={() => ((picked[cur.id] = true), (open = -1))}>Keep selected</button>{/snippet}
  </ChangelogDialog>
{/if}

<style>
  .ur{grid-template-columns:16px 32px minmax(0,1fr) 200px 96px;height:56px}
  .ur .opt{display:flex}
  @media (max-width:1100px){.ur{grid-template-columns:16px 32px minmax(0,1fr) 96px}.ur .opt{display:none}}
</style>
