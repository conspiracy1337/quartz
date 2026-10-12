<script lang="ts">
  import type { Snippet } from 'svelte';

  /** The centred first-run column with its four-step progress bar. */
  interface Props { step: number; width?: number; children: Snippet }
  let { step, width = 460, children }: Props = $props();
  const STEPS = 4;
</script>

<main class="wrap">
  <div class="box" style="width: min({width}px, 100%)">
    <div class="steps" aria-label="Step {step} of {STEPS}">{#each { length: STEPS } as _, i (i)}<span class:on={i < step}></span>{/each}</div>
    {@render children()}
  </div>
</main>

<style>
  .wrap{min-height:100vh;width:100%;overflow:auto;display:grid;place-items:center;padding:24px;background:radial-gradient(70% 60% at 50% 0%,#121214 0%,var(--bg) 70%)}
  .box{display:flex;flex-direction:column;gap:22px}
  .steps{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:6px}
  .steps span{height:3px;border-radius:2px;background:var(--line-2)}
  .steps span.on{background:var(--text)}
</style>
