<script lang="ts">
  /** The one empty state: monolith's .empty block with a short sentence and an optional action. */
  interface Props {
    text: string;
    action?: string;
    href?: string;
    onclick?: () => void;
    class?: string;
    style?: string;
  }
  let { text, action, href, onclick, class: cls = '', style }: Props = $props();
</script>

<div class="empty {cls}" {style}>
  <span>{text}</span>
  {#if action && href}<a class="btn btn-sm" {href}>{action}</a>{:else if action}<button class="btn btn-sm" {onclick}>{action}</button>{/if}
</div>
