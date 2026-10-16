<script lang="ts">
  import type { ServerStats } from '../api/types';

  /** A server's version badge, coloured by how it suits the instance: green takes this version,
      yellow takes another of the same major one, red only another major one. */
  interface Props { server: ServerStats }
  let { server: s }: Props = $props();
  const TONE = { ok: 'badge-accent', minor: 'badge-warn', major: 'badge-danger', '': '' } as const;
  const tip = $derived(s.fit === 'ok' ? 'Takes this version' : s.fit === 'minor' ? `Newest version you can use: ${s.version}` : s.fit === 'major' ? `Takes ${s.version}, not this major version` : undefined);
</script>

{#if s.online}<span class="badge trunc {TONE[s.fit]}" title={tip}>{s.version}</span>{:else}<span class="badge">Offline</span>{/if}
