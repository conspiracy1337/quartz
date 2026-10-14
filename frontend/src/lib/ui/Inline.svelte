<script lang="ts" module>
  import { api } from '../api';
  import { go } from '../router.svelte';

  /** Opens a description link: a project on its page in the launcher, anything else in the browser. */
  export function follow(url: string, project?: string) {
    if (!project) return void api.app.openUrl(url);
    api.browse.resolve(project).then((id) => go({ name: 'project', id }), () => void api.app.openUrl(url));
  }
</script>

<script lang="ts">
  import type { Span } from '../api/types';

  /** Description text: plain runs, <code>, and links (projects open in the launcher, the rest in the browser). Renders text nodes only, never HTML. */
  interface Props { spans: Span[] }
  let { spans }: Props = $props();
</script>

{#each spans as s, i (i)}{#if s.code}<code>{s.text}</code>{:else if s.href}{@const url = s.href}<button class="tlink" onclick={() => follow(url, s.project)}>{s.text}</button>{:else}{s.text}{/if}{/each}
