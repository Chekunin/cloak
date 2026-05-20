<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    label: string;
    id: string;
    hint?: string;
    error?: string;
    required?: boolean;
    children: Snippet;
  }

  const { label, id, hint, error, required = false, children }: Props = $props();
</script>

<div class="flex flex-col gap-1.5">
  <label
    for={id}
    class="flex items-center gap-1 text-sm font-medium text-zinc-700 dark:text-zinc-300"
  >
    {label}
    {#if required}
      <span class="text-rose-500" aria-label="required">*</span>
    {/if}
  </label>
  {@render children()}
  {#if error}
    <p class="text-xs text-rose-600 dark:text-rose-400">{error}</p>
  {:else if hint}
    <p class="text-xs text-zinc-500 dark:text-zinc-400">{hint}</p>
  {/if}
</div>
