<script lang="ts">
  interface Props {
    label: string;
    value: string | number;
    hint?: string;
  }

  const { label, value, hint }: Props = $props();
</script>

<div
  class="rounded-lg border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
>
  <div class="text-xs uppercase tracking-wider text-zinc-500 dark:text-zinc-400">
    {label}
  </div>
  <div class="mt-1 select-text font-mono text-2xl text-zinc-900 dark:text-zinc-100">
    {value}
  </div>
  {#if hint}
    <div class="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{hint}</div>
  {/if}
</div>
