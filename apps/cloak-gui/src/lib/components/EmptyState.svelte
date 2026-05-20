<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    description: string;
    action?: Snippet;
  }

  const { title, description, action }: Props = $props();
</script>

<div
  class="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-zinc-300 px-8 py-12 text-center dark:border-zinc-700"
>
  <div
    class="flex size-12 items-center justify-center rounded-full bg-zinc-100 text-zinc-400 dark:bg-zinc-800 dark:text-zinc-500"
    aria-hidden="true"
  >
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="size-6">
      <rect x="3" y="11" width="18" height="11" rx="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  </div>
  <h3 class="text-base font-semibold text-zinc-900 dark:text-zinc-100">{title}</h3>
  <p class="max-w-sm text-sm text-zinc-500 dark:text-zinc-400">{description}</p>
  {#if action}
    <div class="mt-2">{@render action()}</div>
  {/if}
</div>
