<script lang="ts">
  import { copy } from '$lib/clipboard';

  interface Props {
    value: string;
    sensitive?: boolean;
    label?: string;
    /** Optional inline display variant: an icon-only button next to a value. */
    iconOnly?: boolean;
  }

  const { value, sensitive = false, label = 'Copied', iconOnly = false }: Props = $props();

  async function onClick() {
    await copy(value, { sensitive, label });
  }
</script>

{#if iconOnly}
  <button
    type="button"
    onclick={onClick}
    aria-label="Copy"
    class="rounded p-1 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
  >
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4">
      <rect x="9" y="9" width="13" height="13" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  </button>
{:else}
  <button
    type="button"
    onclick={onClick}
    class="inline-flex items-center gap-1.5 rounded-md border border-zinc-300 bg-white px-2.5 py-1 text-xs font-medium text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800"
  >
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-3.5">
      <rect x="9" y="9" width="13" height="13" rx="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
    Copy
  </button>
{/if}
