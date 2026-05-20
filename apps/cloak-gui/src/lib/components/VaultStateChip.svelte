<script lang="ts">
  import type { VaultState } from '$lib/api';

  interface Props {
    state: VaultState;
  }

  const { state }: Props = $props();

  const palette: Record<VaultState, { label: string; cls: string; dot: string }> = {
    uninitialized: {
      label: 'Uninitialized',
      cls: 'bg-zinc-100 text-zinc-700 dark:bg-zinc-900 dark:text-zinc-300',
      dot: 'bg-zinc-400 dark:bg-zinc-600',
    },
    locked: {
      label: 'Locked',
      cls: 'bg-rose-100 text-rose-900 dark:bg-rose-950 dark:text-rose-200',
      dot: 'bg-rose-500',
    },
    unlocked: {
      label: 'Unlocked',
      cls: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-950 dark:text-emerald-200',
      dot: 'bg-emerald-500',
    },
  };

  const tone = $derived(palette[state]);
</script>

<span
  class="inline-flex items-center gap-2 rounded-full px-3 py-1 text-sm font-medium {tone.cls}"
  aria-label="Vault state: {tone.label}"
>
  <span class="size-2 rounded-full {tone.dot}" aria-hidden="true"></span>
  {tone.label}
</span>
