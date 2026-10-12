<script lang="ts">
  import { api } from '../api';
  import { query } from '../query.svelte';
  import { go } from '../router.svelte';
  import { session } from '../session.svelte';
  import Dialog from '../ui/Dialog.svelte';

  interface Props { instanceId: string }
  let { instanceId }: Props = $props();
  const inst = query(() => api.instances.get(instanceId));
  const TRACE = `java.lang.RuntimeException: Mixin transformation of net.minecraft.client.render.entity.EntityRenderer failed
  at org.spongepowered.asm.mixin.transformer.MixinProcessor.applyMixins
  Caused by: InvalidInjectionException: @Inject(method = "shouldRender") on entityculling.mixins.json:EntityRendererMixin
  Mod file: mods/entityculling-fabric-1.7.2.jar`;

  const relaunch = () => session.play(instanceId);
  function update() {
    session.close();
    go({ name: 'instance', id: instanceId, tab: 'mods', sub: 'updates' });
  }
</script>

<Dialog title="Minecraft crashed" sub="{inst.data?.name ?? ''}, after 3 minutes, exit code 1" width={600} alert onclose={() => session.close()}>
  <div class="cause">
    <span class="ico" style="background: #7cc7e8">E</span>
    <span style="display: flex; flex-direction: column; gap: 6px"><span style="font-weight: 500">Entity Culling 1.7.2 failed to load</span><span class="small muted">Its mixin into the entity renderer did not apply. This usually means it does not support Sodium 0.6.5 yet. Entity Culling 1.7.4 fixes this.</span><span style="display: flex; gap: 6px; margin-top: 4px"><button class="btn btn-sm btn-accent" onclick={update}>Update and relaunch</button><button class="btn btn-sm" onclick={relaunch}>Disable it and relaunch</button></span></span>
  </div>
  <pre class="trace">{TRACE}</pre>
  <div style="display: flex; gap: 6px; flex-wrap: wrap"><button class="btn btn-sm" onclick={() => { session.close(); go({ name: 'instance', id: instanceId, tab: 'logs', sub: 'history' }); }}>Open crash report</button><button class="btn btn-sm">Copy debug info</button><button class="btn btn-sm">Upload logs to mclo.gs</button></div>
  <span class="small faint">Uploads remove your sign-in token, session ID and username from the logs first.</span>
  {#snippet foot()}<button class="btn" onclick={() => session.close()}>Close</button><button class="btn" onclick={relaunch}>Launch again</button>{/snippet}
</Dialog>

<style>
  .trace{font-family:var(--mono);font-size:12px;line-height:19px;color:var(--muted);background:#08080a;border:1px solid var(--line);border-radius:8px;padding:10px 12px;white-space:pre;overflow:auto;max-height:132px;margin:0}
  .cause{display:grid;grid-template-columns:32px minmax(0,1fr);gap:12px;padding:12px;border-radius:10px;border:1px solid color-mix(in srgb,var(--danger) 30%,var(--line));background:color-mix(in srgb,var(--danger) 5%,var(--panel))}
</style>
