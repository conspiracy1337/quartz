<script lang="ts">
  import { api } from '../api';
  import { query, skeletonRows } from '../query.svelte';
  import { prefs } from '../prefs.svelte';
  import { homeRow, homeScope, inHomeOrder } from '../servers.svelte';
  import ServerTable from './ServerTable.svelte';

  /** Home's servers: every instance's, de-duplicated, in the instance tab's table. Join uses the instance last used on it. */
  const servers = query(() => api.home.servers(), 'home.servers'); // opening Home again shows the last answers while it pings
  const rows = $derived(servers.data && inHomeOrder(servers.data.map(homeRow), prefs.value?.general.serverOrder));
</script>

<div class="sl">
  <ServerTable {rows} scope={homeScope} skeleton={skeletonRows('home.servers', 6)} empty="Servers from all your instances show up here." />
</div>

<style>
  .sl{position:absolute;inset:0;display:flex;flex-direction:column;gap:12px}
</style>
