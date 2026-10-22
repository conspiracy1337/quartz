<script lang="ts" module>
  import { below } from './Menu.svelte';

  interface Question { x: number; y: number; text: string; action: string; danger: boolean; run: () => void }

  /** An action that asks once more in a menu under its button (ConfirmMenu shows it). */
  export class Confirm {
    asked = $state<Question | null>(null);

    /** Asks under the pressed button; run happens only on the action. danger is false for an action that sends something out. */
    ask(e: MouseEvent, text: string, action: string, run: () => void, danger = true) {
      this.asked = { ...below(e), text, action, danger, run };
    }

    close() {
      this.asked = null;
    }

    confirmed() {
      const run = this.asked?.run;
      this.asked = null;
      run?.();
    }
  }
</script>

<script lang="ts">
  import Menu from './Menu.svelte';
  import MenuConfirm from './MenuConfirm.svelte';

  interface Props { confirm: Confirm }
  let { confirm }: Props = $props();
</script>

{#if confirm.asked}
  {@const a = confirm.asked}
  <Menu x={a.x} y={a.y} label={a.action} onclose={() => confirm.close()}>
    <MenuConfirm text={a.text} action={a.action} danger={a.danger} oncancel={() => confirm.close()} onconfirm={() => confirm.confirmed()} />
  </Menu>
{/if}
