<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { api } from '$lib/api';
  import HomeAlerts from '$lib/components/HomeAlerts.svelte';
  import NewsBanner from '$lib/components/NewsBanner.svelte';
  import ServerList from '$lib/components/ServerList.svelte';
  import ShotRotator from '$lib/components/ShotRotator.svelte';
  import SkinCard from '$lib/components/SkinCard.svelte';
  import { prefs } from '$lib/prefs.svelte';
  import { lastCount, query, rememberCount } from '$lib/query.svelte';

  // Home 2.6 "Focus column": alerts, the profile row, the news strip, then servers.
  const alerts = query(() => api.home.alerts(), 'home.alerts');
  const news = query(() => api.home.news(), 'home.news');
  // Dismissing hides that article for good; the strip keeps rotating through the rest.
  let dismissed = $state<string[]>([]);
  const shownNews = $derived((news.data ?? []).filter((n) => !dismissed.includes(n.id) && !prefs.value?.general.hiddenNews?.includes(n.id)));
  function hideNews(id: string) {
    dismissed.push(id);
    void prefs.update((s) => (s.general.hiddenNews = [...(s.general.hiddenNews ?? []), id]));
  }
  const wide = new MediaQuery('min-width: 1640px');

  // Rows that have nothing to show leave the grid, so no empty gap is left behind.
  // Until alerts and news answer, the strips shown last time hold their rows as
  // skeletons, so the cards below don't move when the data arrives.
  const STRIP = 'home.news-strip';
  const alertsKnown = $derived(alerts.data !== undefined || !alerts.loading);
  const newsKnown = $derived(news.data !== undefined || !news.loading);
  const showAlerts = $derived(alertsKnown ? (alerts.data?.length ?? 0) > 0 : (lastCount('home.alerts') ?? 0) > 0);
  const showNews = $derived(newsKnown ? shownNews.length > 0 : (lastCount(STRIP) ?? 0) > 0);
  $effect(() => {
    if (newsKnown) rememberCount(STRIP, showNews ? 1 : 0);
  });
  const rows = $derived([showAlerts && 'var(--alert-h)', 'var(--duo-h)', showNews && 'var(--strip-h)', 'minmax(0,1fr)'].filter(Boolean).join(' '));
  const areas = $derived([showAlerts && '"alerts"', '"duo"', showNews && '"news"', '"servers"'].filter(Boolean).join(' '));
</script>

{#snippet strip()}<div class="sk" style="position: absolute; inset: 0; border-radius: var(--radius-lg)" aria-hidden="true"></div>{/snippet}

<div class="home" style="grid-template-rows: {rows}; grid-template-areas: {areas}">
  {#if showAlerts}<div class="cell" style="grid-area: alerts">{#if alertsKnown}<HomeAlerts alerts={alerts.data ?? []} />{:else}{@render strip()}{/if}</div>{/if}
  <div class="duo">
    <div class="cell"><SkinCard scale={wide.current ? 5 : 4} /></div>
    <div class="cell"><ShotRotator /></div>
  </div>
  {#if showNews}<div class="cell" style="grid-area: news">{#if shownNews.length}<NewsBanner items={shownNews} ondismiss={hideNews} />{:else}{@render strip()}{/if}</div>{/if}
  <div class="cell" style="grid-area: servers"><ServerList /></div>
</div>

<style>
  /* The side margin grows slowly with the window (about 104px at 1200 wide, about 130px at 1920),
     so the column fills a maximised window. Past 1760px of content the extra space is split evenly. */
  .home{flex:1 1 auto;min-height:0;display:grid;gap:var(--gap);
    padding-block:var(--gutter);
    padding-inline:max(var(--gutter), calc(80px + (100% - 968px) * .06), calc((100% - 1760px) / 2));
    --duo-h:clamp(176px, calc(176px + (100vw - 1200px) * .12), 264px);
    grid-template-columns:minmax(0,1fr)}
  .cell{position:relative;min-width:0;min-height:0}
  /* The screenshot column stays 16:9 to the row height, so the profile row scales as one unit. */
  .duo{grid-area:duo;display:grid;grid-template-columns:minmax(0,1fr) calc(var(--duo-h) * 16 / 9);gap:var(--gap);min-width:0;min-height:0}
  @media (max-height:680px){.home{--duo-h:148px}}
</style>
