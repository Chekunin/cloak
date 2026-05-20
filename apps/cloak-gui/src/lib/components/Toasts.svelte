<script lang="ts">
  import { toasts } from '$lib/stores/toasts.svelte';
  import type { ToastKind } from '$lib/stores/toasts.svelte';

  const palette: Record<ToastKind, string> = {
    success:
      'border-emerald-300 bg-emerald-50 text-emerald-900 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-100',
    error:
      'border-rose-300 bg-rose-50 text-rose-900 dark:border-rose-900 dark:bg-rose-950 dark:text-rose-100',
    info:
      'border-zinc-300 bg-white text-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100',
  };
</script>

<!-- z-[60]: toasts sit above modals so feedback is always visible. -->
<div
  class="pointer-events-none fixed bottom-4 right-4 z-[60] flex flex-col gap-2"
  role="status"
  aria-live="polite"
>
  {#each toasts.list as toast (toast.id)}
    <div
      class="
        pointer-events-auto flex w-80 items-start gap-3 rounded-md border px-4 py-3 text-sm shadow-lg
        {palette[toast.kind]}
      "
    >
      <div class="flex-1">
        <div class="font-medium">{toast.message}</div>
        {#if toast.hint}
          <div class="mt-0.5 text-xs opacity-80">{toast.hint}</div>
        {/if}
      </div>
      <button
        type="button"
        class="opacity-50 transition hover:opacity-100"
        onclick={() => toasts.dismiss(toast.id)}
        aria-label="Dismiss notification"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4">
          <path d="M6 6l12 12M6 18L18 6" />
        </svg>
      </button>
    </div>
  {/each}
</div>
