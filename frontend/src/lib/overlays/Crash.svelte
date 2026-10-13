<script lang="ts">
  import { api } from '../api';
  import { contentArt } from '../api/content';
  import type { CrashCause } from '../api/types';
  import { plural } from '../format';
  import { query } from '../query.svelte';
  import { go } from '../router.svelte';
  import { session } from '../session.svelte';
  import { tasks } from '../tasks.svelte';
  import Dialog from '../ui/Dialog.svelte';

  /** A run's crash (runId) or a report in crash-reports (file), with its likely cause and fixes. */
  interface Props { instanceId: string; runId?: string | undefined; file?: string | undefined }
  let { instanceId, runId, file }: Props = $props();
  const inst = query(() => api.instances.get(instanceId));
  const info = query(() => (file ? api.logs.crashFile(instanceId, file) : api.logs.crash(instanceId, runId ?? '')));
  let busy = $state(false);

  const sub = $derived.by(() => {
    const i = info.data;
    if (!i) return inst.data?.name ?? '';
    const parts = [inst.data?.name ?? ''];
    if (i.runId) parts.push(`after ${plural(i.lengthMin, 'minute')}`);
    if (i.exitCode >= 0 || i.runId) parts.push(`exit code ${i.exitCode}`);
    return parts.join(', ');
  });

  const relaunch = () => session.play(instanceId);

  /** Applies the newer version of the mod at fault, then launches once its job is done. */
  async function update(c: CrashCause) {
    if (!c.mod || busy) return;
    busy = true;
    try {
      const review = await api.content.review(instanceId, 'mods', true);
      const u = review.updates.find((x) => x.id === c.mod?.item);
      if (!u) {
        session.close();
        go({ name: 'instance', id: instanceId, tab: 'mods', sub: 'updates' });
        return;
      }
      const job = await api.content.apply(review.checkId, [u.id]);
      api.content.forget(instanceId, 'mods');
      session.close();
      if (await tasks.until(job)) relaunch();
    } catch (e) {
      session.fail('Update', e);
    } finally {
      busy = false;
    }
  }

  async function disable(c: CrashCause) {
    if (!c.mod || busy) return;
    busy = true;
    try {
      await api.content.setEnabled(instanceId, 'mods', c.mod.item, false);
      relaunch();
    } catch (e) {
      session.fail('Turn off', e);
    } finally {
      busy = false;
    }
  }

  function report() {
    session.close();
    go({ name: 'instance', id: instanceId, tab: 'logs', sub: file ? 'files' : 'history' });
  }
  const debug = async () => session.copy(await api.logs.debugInfo(instanceId, runId ?? ''), 'Debug info');
  const upload = () => session.upload(() => (file ? api.logs.uploadFile(instanceId, file) : api.logs.uploadRun(instanceId, runId ?? '')));
</script>

<Dialog title="Minecraft crashed" {sub} width={600} alert onclose={() => session.close()}>
  {#if info.data?.cause}
    {@const c = info.data.cause}
    {@const art = contentArt(c.mod?.name ?? c.title, c.mod?.icon)}
    <div class="cause">
      <span class="ico" style="background: {art.background}">{art.initial ?? ''}</span>
      <span style="display: flex; flex-direction: column; gap: 6px"><span style="font-weight: 500">{c.title}</span><span class="small muted">{c.text}</span>
        {#if c.fixes.length}<span style="display: flex; gap: 6px; margin-top: 4px">{#if c.fixes.includes('update')}<button class="btn btn-sm btn-accent" disabled={busy} onclick={() => update(c)}>Update and relaunch</button>{/if}{#if c.fixes.includes('disable')}<button class="btn btn-sm" disabled={busy} onclick={() => disable(c)}>Disable it and relaunch</button>{/if}</span>{/if}
      </span>
    </div>
  {/if}
  {#if info.data}
    {#if info.data.trace}<pre class="trace">{info.data.trace}</pre>{/if}
  {:else}
    <div class="sk" style="height: 132px; border-radius: 8px" aria-hidden="true"></div>
  {/if}
  <div style="display: flex; gap: 6px; flex-wrap: wrap"><button class="btn btn-sm" onclick={report}>Open crash report</button><button class="btn btn-sm" onclick={debug}>Copy debug info</button><button class="btn btn-sm" onclick={upload}>Upload logs to mclo.gs</button></div>
  <span class="small faint">Uploads remove your sign-in token, session ID and username from the logs first.</span>
  {#snippet foot()}<button class="btn" onclick={() => session.close()}>Close</button><button class="btn" onclick={relaunch}>Launch again</button>{/snippet}
</Dialog>

<style>
  .trace{font-family:var(--mono);font-size:12px;line-height:19px;color:var(--muted);background:#08080a;border:1px solid var(--line);border-radius:8px;padding:10px 12px;white-space:pre;overflow:auto;max-height:132px;margin:0}
  .cause{display:grid;grid-template-columns:32px minmax(0,1fr);gap:12px;padding:12px;border-radius:10px;border:1px solid color-mix(in srgb,var(--danger) 30%,var(--line));background:color-mix(in srgb,var(--danger) 5%,var(--panel))}
</style>
