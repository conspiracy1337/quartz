<script lang="ts">
  import { api } from '../api';
  import type { NewsItem } from '../api/types';
  import { monthDay } from '../format';
  import Icon from '../ui/Icon.svelte';

  /** The latest minecraft.net article. A strip under 120px tall, a picture tile above that. */
  interface Props { item: NewsItem; ondismiss: () => void }
  let { item, ondismiss }: Props = $props();

  function open(e: MouseEvent) {
    e.preventDefault();
    void api.app.openUrl(item.url);
  }
</script>

<div class="nb" style="font-family: var(--font)">
  <div class="nb-grid">
    <span class="nb-pic" aria-hidden="true">{#if item.image}<span class="nb-art" style="background: url({item.image}) center/cover no-repeat"></span>{:else}<span class="nb-art"><span class="nb-sky"></span><span class="nb-sun"></span><span class="nb-m1"></span><span class="nb-tree"></span><span class="nb-m2"></span></span>{/if}<span class="nb-shade"></span></span>
    <span class="nb-txt" aria-hidden="true">
      <span class="nb-ttl">{item.title}</span>
      <span class="nb-meta"><span class="trunc">{item.source}, {monthDay(item.published)}</span><Icon name="external" size={12} sw={2} style="flex: 0 0 12px" /></span>
    </span>
    <button class="btn btn-sm btn-icon btn-ghost nb-x" aria-label="Hide this article" onclick={ondismiss}><Icon name="x" size={10} sw={2.2} /></button>
  </div>
  <a class="nb-hit" href={item.url} onclick={open} aria-label="Minecraft news: {item.title}. Opens {item.source} in your browser."></a>
</div>

<style>
  /* Strip under 120px tall (picture flush left, text, dismiss), picture tile above that. */
  .nb{position:absolute;inset:0;border-radius:var(--radius-lg);overflow:hidden;border:1px solid var(--line);background:var(--panel);container-type:size;color:var(--text)}
  .nb-grid{position:absolute;inset:0;display:grid;grid-template-columns:128px minmax(0,1fr) 28px;align-items:center;column-gap:var(--inset);padding-right:var(--inset)}
  .nb-pic{position:relative;align-self:stretch;overflow:hidden;background:#1a2346}
  .nb-art{position:absolute;inset:0;transition:transform 600ms var(--ease)}
  @media (hover:hover){.nb:has(.nb-hit:hover) .nb-art{transform:scale(1.04)}}
  .nb-sky{position:absolute;inset:0;background:linear-gradient(180deg,#2a2f6e 0%,#7a4f9a 38%,#ff9a5a 70%,#ffd08a 100%)}
  .nb-sun{position:absolute;left:62%;top:38%;width:22%;aspect-ratio:1;border-radius:50%;background:radial-gradient(circle,#fff3c4 0%,#ffc670 45%,transparent 70%)}
  .nb-m1{position:absolute;left:0;right:0;bottom:0;height:62%;background:#3b2f5c;clip-path:polygon(0 40%,8% 40%,8% 30%,16% 30%,16% 20%,24% 20%,24% 34%,34% 34%,34% 14%,42% 14%,42% 26%,52% 26%,52% 38%,62% 38%,62% 22%,70% 22%,70% 10%,78% 10%,78% 30%,88% 30%,88% 42%,100% 42%,100% 100%,0 100%)}
  .nb-m2{position:absolute;left:0;right:0;bottom:0;height:42%;background:#1e2a24;clip-path:polygon(0 30%,10% 30%,10% 44%,22% 44%,22% 24%,30% 24%,30% 36%,44% 36%,44% 50%,58% 50%,58% 32%,66% 32%,66% 20%,74% 20%,74% 40%,86% 40%,86% 28%,100% 28%,100% 100%,0 100%)}
  .nb-tree{position:absolute;left:15%;bottom:30%;width:9%;aspect-ratio:1.4;background:#f2a3c4}
  .nb-shade{display:none}
  .nb-txt{display:flex;flex-direction:column;gap:2px;min-width:0}
  .nb-ttl{font-size:13px;line-height:20px;font-weight:500;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .nb-meta{display:flex;gap:6px;align-items:center;min-width:0;font-size:12px;line-height:16px;color:var(--faint)}
  .nb-hit{position:absolute;inset:0;z-index:1;border-radius:var(--radius-lg)}
  .nb-hit:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
  .nb-x{position:relative;z-index:2}
  @container (min-height:120px){
    .nb-grid{display:block;padding:0}
    .nb-pic{position:absolute;inset:0}
    .nb-shade{display:block;position:absolute;inset:0;background:linear-gradient(0deg,rgba(6,6,8,.86) 0%,rgba(6,6,8,.35) 55%,rgba(6,6,8,0) 100%)}
    .nb-txt{position:absolute;left:var(--inset);right:var(--inset);bottom:var(--inset);gap:6px;color:#fff}
    .nb-ttl{font-size:18px;line-height:24px;font-weight:600;letter-spacing:-.015em;white-space:normal;display:-webkit-box;-webkit-line-clamp:2;line-clamp:2;-webkit-box-orient:vertical}
    .nb-meta{color:rgba(255,255,255,.72)}
    .nb-x{position:absolute;top:12px;right:12px;background:rgba(0,0,0,.4);color:#fff;border-color:transparent}
    .nb-x:hover{background:rgba(0,0,0,.6)}
  }
</style>
